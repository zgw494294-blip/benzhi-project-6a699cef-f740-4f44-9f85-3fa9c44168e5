package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompletesReservationWorkflow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	var output, errors bytes.Buffer

	commands := [][]string{
		{"request", "--ledger", path, "--id", "r-500", "--bench", "bench-a", "--start", "2026-08-17T09:00:00Z", "--end", "2026-08-17T10:00:00Z"},
		{"confirm", "--ledger", path, "r-500"},
		{"check-in", "--ledger", path, "r-500", "--at", "2026-08-17T09:05:00Z"},
		{"release", "--ledger", path, "r-500", "--at", "2026-08-17T10:05:00Z"},
	}
	for _, command := range commands {
		if status := Run(command, &output, &errors); status != 0 {
			t.Fatalf("Run(%v) status = %d, errors = %q", command, status, errors.String())
		}
	}

	output.Reset()
	if status := Run([]string{"show", "--ledger", path, "r-500"}, &output, &errors); status != 0 {
		t.Fatalf("Run(show) status = %d, errors = %q", status, errors.String())
	}
	var receipt struct {
		ID          string  `json:"id"`
		CheckedInAt *string `json:"checked_in_at"`
		ReleasedAt  *string `json:"released_at"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.String())), &receipt); err != nil {
		t.Fatalf("show output is not JSON: %v; output = %q", err, output.String())
	}
	if receipt.ID != "r-500" || receipt.CheckedInAt == nil || receipt.ReleasedAt == nil {
		t.Fatalf("show receipt = %#v", receipt)
	}
}

func TestRunRejectsInvalidTransitionWithoutChangingOutputLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	var output, errors bytes.Buffer
	request := []string{"request", "--ledger", path, "--id", "r-501", "--bench", "bench-a", "--start", "2026-08-17T09:00:00Z", "--end", "2026-08-17T10:00:00Z"}
	if status := Run(request, &output, &errors); status != 0 {
		t.Fatalf("Run(request) status = %d, errors = %q", status, errors.String())
	}
	if status := Run([]string{"check-in", "--ledger", path, "r-501", "--at", "2026-08-17T09:05:00Z"}, &output, &errors); status == 0 {
		t.Fatal("Run(check-in before confirmation) status = 0")
	}
	if !strings.Contains(errors.String(), "cannot be checked in") {
		t.Fatalf("error output = %q", errors.String())
	}
}
