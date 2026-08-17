package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/benzhi/benchslot/internal/benchslot"
)

func TestSaveAndLoadRoundTripUsesOneLedgerFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	data := benchslot.NewLedger()
	start := time.Date(2026, time.August, 17, 9, 0, 0, 0, time.UTC)
	if _, err := benchslot.Request(&data, "r-400", "bench-a", start, start.Add(time.Hour)); err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if err := Save(path, data); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("loaded ledger validation error = %v", err)
	}
	if len(loaded.Reservations) != 1 || loaded.Reservations[0].ID != "r-400" {
		t.Fatalf("loaded reservations = %#v", loaded.Reservations)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "ledger.json" {
		t.Fatalf("ledger directory entries = %#v", entries)
	}
}

func TestLoadMissingLedgerReturnsEmptyVersionedLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("empty ledger validation error = %v", err)
	}
	if len(loaded.Reservations) != 0 || len(loaded.Receipts) != 0 {
		t.Fatalf("empty ledger = %#v", loaded)
	}
}

func TestLoadRejectsMalformedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte(`{"version":9,"reservations":[],"receipts":[]}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestSaveRemovesTemporaryFileWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ledger.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := Save(path, benchslot.NewLedger()); err == nil {
		t.Fatal("Save() error = nil")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "ledger.json" {
		t.Fatalf("ledger directory entries = %#v", entries)
	}
}
