package benchslot

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const LedgerVersion = 1

type State string

const (
	StatePending   State = "pending"
	StateConfirmed State = "confirmed"
	StateOccupied  State = "occupied"
	StateCompleted State = "completed"
)

type Reservation struct {
	ID          string     `json:"id"`
	Bench       string     `json:"bench"`
	Start       time.Time  `json:"start"`
	End         time.Time  `json:"end"`
	State       State      `json:"state"`
	CheckedInAt *time.Time `json:"checked_in_at"`
	ReleasedAt  *time.Time `json:"released_at"`
}

type Receipt struct {
	ID          string     `json:"id"`
	Bench       string     `json:"bench"`
	Start       time.Time  `json:"start"`
	End         time.Time  `json:"end"`
	CheckedInAt *time.Time `json:"checked_in_at"`
	ReleasedAt  *time.Time `json:"released_at"`
}

type Ledger struct {
	Version      int           `json:"version"`
	Reservations []Reservation `json:"reservations"`
	Receipts     []Receipt     `json:"receipts"`
}

func (ledger Ledger) Validate() error {
	if ledger.Version != LedgerVersion {
		return fmt.Errorf("unsupported ledger version %d", ledger.Version)
	}
	if ledger.Reservations == nil || ledger.Receipts == nil {
		return errors.New("ledger collections are required")
	}
	reservationIDs := make(map[string]struct{}, len(ledger.Reservations))
	for _, reservation := range ledger.Reservations {
		if _, exists := reservationIDs[reservation.ID]; exists {
			return fmt.Errorf("duplicate reservation id %q", reservation.ID)
		}
		reservationIDs[reservation.ID] = struct{}{}
		if strings.TrimSpace(reservation.ID) == "" || strings.TrimSpace(reservation.Bench) == "" {
			return errors.New("reservation id and bench are required")
		}
		if !reservation.Start.Before(reservation.End) {
			return fmt.Errorf("reservation %q has an invalid interval", reservation.ID)
		}
		switch reservation.State {
		case StatePending, StateConfirmed:
			if reservation.CheckedInAt != nil || reservation.ReleasedAt != nil {
				return fmt.Errorf("reservation %q has unexpected transition timestamps", reservation.ID)
			}
		case StateOccupied:
			if !hasTime(reservation.CheckedInAt) || reservation.ReleasedAt != nil {
				return fmt.Errorf("reservation %q has invalid occupied timestamps", reservation.ID)
			}
		case StateCompleted:
			if !hasTime(reservation.CheckedInAt) || !hasTime(reservation.ReleasedAt) {
				return fmt.Errorf("reservation %q is missing transition timestamps", reservation.ID)
			}
			if reservation.ReleasedAt.Before(*reservation.CheckedInAt) {
				return fmt.Errorf("reservation %q has invalid transition order", reservation.ID)
			}
		default:
			return fmt.Errorf("reservation %q has unknown state %q", reservation.ID, reservation.State)
		}
	}
	for index, reservation := range ledger.Reservations {
		if !isAllocated(reservation.State) {
			continue
		}
		for _, other := range ledger.Reservations[index+1:] {
			if isAllocated(other.State) && reservation.Bench == other.Bench &&
				overlaps(reservation.Start, reservation.End, other.Start, other.End) {
				return fmt.Errorf("active reservations %q and %q overlap", reservation.ID, other.ID)
			}
		}
	}
	receiptIDs := make(map[string]struct{}, len(ledger.Receipts))
	for _, receipt := range ledger.Receipts {
		if _, exists := receiptIDs[receipt.ID]; exists {
			return fmt.Errorf("duplicate receipt id %q", receipt.ID)
		}
		receiptIDs[receipt.ID] = struct{}{}
		if err := validateReceipt(receipt); err != nil {
			return err
		}
		reservation, err := findReservation(ledger, receipt.ID)
		if err != nil {
			return fmt.Errorf("receipt %q has no reservation: %w", receipt.ID, err)
		}
		if reservation.State != StateCompleted || reservation.Bench != receipt.Bench ||
			!reservation.Start.Equal(receipt.Start) || !reservation.End.Equal(receipt.End) ||
			!reservation.CheckedInAt.Equal(*receipt.CheckedInAt) || !reservation.ReleasedAt.Equal(*receipt.ReleasedAt) {
			return fmt.Errorf("receipt %q does not match its completed reservation", receipt.ID)
		}
	}
	for index, receipt := range ledger.Receipts {
		for _, other := range ledger.Receipts[index+1:] {
			if receipt.Bench == other.Bench &&
				receipt.CheckedInAt.Before(*receipt.ReleasedAt) && other.CheckedInAt.Before(*other.ReleasedAt) &&
				overlaps(*receipt.CheckedInAt, *receipt.ReleasedAt, *other.CheckedInAt, *other.ReleasedAt) {
				return fmt.Errorf("usage receipts %q and %q overlap", receipt.ID, other.ID)
			}
		}
	}
	for _, reservation := range ledger.Reservations {
		_, hasReceipt := receiptIDs[reservation.ID]
		if (reservation.State == StateCompleted) != hasReceipt {
			return fmt.Errorf("completed receipt mismatch for reservation %q", reservation.ID)
		}
	}
	return nil
}

func NewLedger() Ledger {
	return Ledger{Version: LedgerVersion, Reservations: []Reservation{}, Receipts: []Receipt{}}
}

func Request(ledger *Ledger, id, bench string, start, end time.Time) (Reservation, error) {
	if ledger == nil {
		return Reservation{}, errors.New("ledger is required")
	}
	if strings.TrimSpace(id) == "" {
		return Reservation{}, errors.New("reservation id is required")
	}
	if strings.TrimSpace(bench) == "" {
		return Reservation{}, errors.New("bench is required")
	}
	if !start.Before(end) {
		return Reservation{}, errors.New("reservation start must be before end")
	}
	if _, err := findReservation(*ledger, id); err == nil {
		return Reservation{}, fmt.Errorf("reservation %q already exists", id)
	}
	if _, err := findReceipt(*ledger, id); err == nil {
		return Reservation{}, fmt.Errorf("receipt %q already exists", id)
	}
	reservation := Reservation{
		ID: id, Bench: bench, Start: start.UTC(), End: end.UTC(), State: StatePending,
	}
	ledger.Reservations = append(ledger.Reservations, reservation)
	return reservation, nil
}

func Confirm(ledger *Ledger, id string) (Reservation, error) {
	if ledger == nil {
		return Reservation{}, errors.New("ledger is required")
	}
	index, reservation, err := reservationIndex(*ledger, id)
	if err != nil {
		return Reservation{}, err
	}
	if reservation.State != StatePending {
		return Reservation{}, fmt.Errorf("reservation %q cannot be confirmed from state %q", id, reservation.State)
	}
	for _, other := range ledger.Reservations {
		if other.ID == id || !isAllocated(other.State) || other.Bench != reservation.Bench {
			continue
		}
		if overlaps(reservation.Start, reservation.End, other.Start, other.End) {
			return Reservation{}, fmt.Errorf("reservation %q conflicts with %q", id, other.ID)
		}
	}
	reservation.State = StateConfirmed
	ledger.Reservations[index] = reservation
	return reservation, nil
}

func CheckIn(ledger *Ledger, id string, at time.Time) (Reservation, error) {
	if ledger == nil {
		return Reservation{}, errors.New("ledger is required")
	}
	if at.IsZero() {
		return Reservation{}, errors.New("check-in time is required")
	}
	index, reservation, err := reservationIndex(*ledger, id)
	if err != nil {
		return Reservation{}, err
	}
	if reservation.State != StateConfirmed {
		return Reservation{}, fmt.Errorf("reservation %q cannot be checked in from state %q", id, reservation.State)
	}
	checkedInAt := at.UTC()
	reservation.State = StateOccupied
	reservation.CheckedInAt = &checkedInAt
	ledger.Reservations[index] = reservation
	return reservation, nil
}

func Release(ledger *Ledger, id string, at time.Time) (Reservation, error) {
	if ledger == nil {
		return Reservation{}, errors.New("ledger is required")
	}
	if at.IsZero() {
		return Reservation{}, errors.New("release time is required")
	}
	index, reservation, err := reservationIndex(*ledger, id)
	if err != nil {
		return Reservation{}, err
	}
	if reservation.State != StateOccupied {
		return Reservation{}, fmt.Errorf("reservation %q cannot be released from state %q", id, reservation.State)
	}
	if reservation.CheckedInAt == nil {
		return Reservation{}, fmt.Errorf("reservation %q has no check-in time", id)
	}
	releasedAt := at.UTC()
	if releasedAt.Before(*reservation.CheckedInAt) {
		return Reservation{}, errors.New("release time cannot precede check-in time")
	}
	if _, err := findReceipt(*ledger, id); err == nil {
		return Reservation{}, fmt.Errorf("receipt %q already exists", id)
	}
	reservation.State = StateCompleted
	reservation.ReleasedAt = &releasedAt
	ledger.Reservations[index] = reservation
	ledger.Receipts = append(ledger.Receipts, Receipt{
		ID:          reservation.ID,
		Bench:       reservation.Bench,
		Start:       reservation.Start,
		End:         reservation.End,
		CheckedInAt: copyTime(reservation.CheckedInAt),
		ReleasedAt:  copyTime(reservation.ReleasedAt),
	})
	return reservation, nil
}

func FindReceipt(ledger Ledger, id string) (Receipt, error) {
	receipt, err := findReceipt(ledger, id)
	if err != nil {
		return Receipt{}, err
	}
	if err := validateReceipt(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

func validateReceipt(receipt Receipt) error {
	if strings.TrimSpace(receipt.ID) == "" || strings.TrimSpace(receipt.Bench) == "" {
		return errors.New("receipt id and bench are required")
	}
	if !receipt.Start.Before(receipt.End) {
		return fmt.Errorf("receipt %q has an invalid interval", receipt.ID)
	}
	if !hasTime(receipt.CheckedInAt) || !hasTime(receipt.ReleasedAt) {
		return fmt.Errorf("receipt %q is missing transition timestamps", receipt.ID)
	}
	if receipt.ReleasedAt.Before(*receipt.CheckedInAt) {
		return fmt.Errorf("receipt %q has an invalid transition order", receipt.ID)
	}
	return nil
}

func hasTime(value *time.Time) bool {
	return value != nil && !value.IsZero()
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func reservationIndex(ledger Ledger, id string) (int, Reservation, error) {
	for index, reservation := range ledger.Reservations {
		if reservation.ID == id {
			return index, reservation, nil
		}
	}
	return 0, Reservation{}, fmt.Errorf("reservation %q not found", id)
}

func isAllocated(state State) bool {
	return state == StateConfirmed || state == StateOccupied
}

func overlaps(start, end, otherStart, otherEnd time.Time) bool {
	return start.Before(otherEnd) && otherStart.Before(end)
}

func findReservation(ledger Ledger, id string) (Reservation, error) {
	for _, reservation := range ledger.Reservations {
		if reservation.ID == id {
			return reservation, nil
		}
	}
	return Reservation{}, fmt.Errorf("reservation %q not found", id)
}

func findReceipt(ledger Ledger, id string) (Receipt, error) {
	for _, receipt := range ledger.Receipts {
		if receipt.ID == id {
			return receipt, nil
		}
	}
	return Receipt{}, fmt.Errorf("receipt %q not found", id)
}
