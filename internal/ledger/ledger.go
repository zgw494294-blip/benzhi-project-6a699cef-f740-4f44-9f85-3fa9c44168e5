package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/benzhi/benchslot/internal/benchslot"
)

func Load(path string) (benchslot.Ledger, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return benchslot.NewLedger(), nil
	}
	if err != nil {
		return benchslot.Ledger{}, fmt.Errorf("read ledger %q: %w", path, err)
	}
	var value benchslot.Ledger
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return benchslot.Ledger{}, fmt.Errorf("decode ledger %q: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return benchslot.Ledger{}, fmt.Errorf("decode ledger %q: multiple JSON values", path)
		}
		return benchslot.Ledger{}, fmt.Errorf("decode ledger %q: %w", path, err)
	}
	if err := value.Validate(); err != nil {
		return benchslot.Ledger{}, fmt.Errorf("validate ledger %q: %w", path, err)
	}
	return value, nil
}

func Save(path string, value benchslot.Ledger) (retErr error) {
	if err := value.Validate(); err != nil {
		return fmt.Errorf("validate ledger before save: %w", err)
	}
	directory := filepath.Dir(path)
	base := filepath.Base(path)
	lock, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open ledger directory: %w", err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock ledger directory: %w", closeWith(err, lock))
	}
	defer func() {
		unlockErr := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		if err := errors.Join(unlockErr, lock.Close()); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release ledger lock: %w", err))
		}
	}()

	current, err := Load(path)
	if err != nil {
		return fmt.Errorf("load current ledger before save: %w", err)
	}
	value, err = merge(current, value)
	if err != nil {
		return fmt.Errorf("merge ledger before save: %w", err)
	}
	temporary, err := os.CreateTemp(directory, "."+base+".tmp-")
	if err != nil {
		return fmt.Errorf("create temporary ledger: %w", err)
	}
	temporaryName := temporary.Name()
	keepTemporary := true
	defer func() {
		if !keepTemporary {
			return
		}
		if cleanupErr := os.Remove(temporaryName); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("remove temporary ledger: %w", cleanupErr))
		}
	}()

	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode ledger: %w", closeWith(err, temporary))
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary ledger: %w", closeWith(err, temporary))
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary ledger: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("rename temporary ledger: %w", err)
	}
	keepTemporary = false
	return nil
}

func merge(current, proposed benchslot.Ledger) (benchslot.Ledger, error) {
	merged := current
	reservations := make(map[string]int, len(merged.Reservations))
	for index, reservation := range merged.Reservations {
		reservations[reservation.ID] = index
	}
	for _, reservation := range proposed.Reservations {
		index, exists := reservations[reservation.ID]
		if !exists {
			reservations[reservation.ID] = len(merged.Reservations)
			merged.Reservations = append(merged.Reservations, reservation)
			continue
		}
		resolved, err := mergeReservation(merged.Reservations[index], reservation)
		if err != nil {
			return benchslot.Ledger{}, err
		}
		merged.Reservations[index] = resolved
	}

	receipts := make(map[string]benchslot.Receipt, len(merged.Receipts))
	for _, receipt := range merged.Receipts {
		receipts[receipt.ID] = receipt
	}
	for _, receipt := range proposed.Receipts {
		currentReceipt, exists := receipts[receipt.ID]
		if !exists {
			receipts[receipt.ID] = receipt
			merged.Receipts = append(merged.Receipts, receipt)
			continue
		}
		if !sameReceipt(currentReceipt, receipt) {
			return benchslot.Ledger{}, fmt.Errorf("conflicting receipt %q", receipt.ID)
		}
	}
	if err := merged.Validate(); err != nil {
		return benchslot.Ledger{}, err
	}
	return merged, nil
}

func mergeReservation(current, proposed benchslot.Reservation) (benchslot.Reservation, error) {
	if current.Bench != proposed.Bench || !current.Start.Equal(proposed.Start) || !current.End.Equal(proposed.End) {
		return benchslot.Reservation{}, fmt.Errorf("conflicting reservation %q", proposed.ID)
	}
	currentRank := stateRank(current.State)
	proposedRank := stateRank(proposed.State)
	if currentRank == proposedRank {
		if !sameReservation(current, proposed) {
			return benchslot.Reservation{}, fmt.Errorf("conflicting reservation %q", proposed.ID)
		}
		return current, nil
	}
	if currentRank > proposedRank {
		return current, nil
	}
	if current.CheckedInAt != nil && !sameTime(current.CheckedInAt, proposed.CheckedInAt) {
		return benchslot.Reservation{}, fmt.Errorf("conflicting reservation %q", proposed.ID)
	}
	return proposed, nil
}

func stateRank(state benchslot.State) int {
	switch state {
	case benchslot.StatePending:
		return 0
	case benchslot.StateConfirmed:
		return 1
	case benchslot.StateOccupied:
		return 2
	case benchslot.StateCompleted:
		return 3
	default:
		return -1
	}
}

func sameReservation(left, right benchslot.Reservation) bool {
	return left.ID == right.ID && left.Bench == right.Bench && left.Start.Equal(right.Start) &&
		left.End.Equal(right.End) && left.State == right.State &&
		sameTime(left.CheckedInAt, right.CheckedInAt) && sameTime(left.ReleasedAt, right.ReleasedAt)
}

func sameReceipt(left, right benchslot.Receipt) bool {
	return left.ID == right.ID && left.Bench == right.Bench && left.Start.Equal(right.Start) &&
		left.End.Equal(right.End) && sameTime(left.CheckedInAt, right.CheckedInAt) &&
		sameTime(left.ReleasedAt, right.ReleasedAt)
}

func sameTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func closeWith(primary error, file *os.File) error {
	return errors.Join(primary, file.Close())
}
