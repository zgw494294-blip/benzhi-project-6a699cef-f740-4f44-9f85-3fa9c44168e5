package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

func closeWith(primary error, file *os.File) error {
	return errors.Join(primary, file.Close())
}
