package benchslot

import (
	"testing"
	"time"
)

func TestRequestCreatesPendingReservation(t *testing.T) {
	ledger := NewLedger()
	start := time.Date(2026, time.August, 17, 9, 0, 0, 0, time.UTC)
	end := start.Add(90 * time.Minute)

	reservation, err := Request(&ledger, "r-100", "lathe-1", start, end)
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if reservation.State != StatePending {
		t.Fatalf("state = %q, want %q", reservation.State, StatePending)
	}
	if reservation.CheckedInAt != nil || reservation.ReleasedAt != nil {
		t.Fatal("transition timestamps should be absent for a new reservation")
	}
	if len(ledger.Reservations) != 1 {
		t.Fatalf("reservation count = %d, want 1", len(ledger.Reservations))
	}
}

func TestRequestRejectsInvalidIntervalWithoutMutation(t *testing.T) {
	ledger := NewLedger()
	start := time.Date(2026, time.August, 17, 10, 0, 0, 0, time.UTC)

	if _, err := Request(&ledger, "r-101", "laser-1", start, start); err == nil {
		t.Fatal("Request() error = nil, want invalid interval error")
	}
	if len(ledger.Reservations) != 0 {
		t.Fatal("invalid request mutated the ledger")
	}
}

func TestConfirmRejectsConflictsAndAllowsHalfOpenBoundary(t *testing.T) {
	ledger := NewLedger()
	base := time.Date(2026, time.August, 17, 9, 0, 0, 0, time.UTC)
	requestForTest(t, &ledger, "r-200", "bench-a", base, base.Add(time.Hour))
	requestForTest(t, &ledger, "r-201", "bench-a", base.Add(30*time.Minute), base.Add(90*time.Minute))
	requestForTest(t, &ledger, "r-202", "bench-a", base.Add(time.Hour), base.Add(2*time.Hour))

	if _, err := Confirm(&ledger, "r-200"); err != nil {
		t.Fatalf("Confirm(first) error = %v", err)
	}
	if _, err := Confirm(&ledger, "r-201"); err == nil {
		t.Fatal("Confirm(overlap) error = nil")
	}
	if got, _ := reservationForTest(ledger, "r-201"); got.State != StatePending {
		t.Fatalf("overlapping reservation state = %q, want %q", got.State, StatePending)
	}
	if _, err := Confirm(&ledger, "r-202"); err != nil {
		t.Fatalf("Confirm(boundary) error = %v", err)
	}
	if _, err := Confirm(&ledger, "r-200"); err == nil {
		t.Fatal("Confirm(repeated) error = nil")
	}
}

func requestForTest(t *testing.T, ledger *Ledger, id, bench string, start, end time.Time) {
	t.Helper()
	if _, err := Request(ledger, id, bench, start, end); err != nil {
		t.Fatalf("Request(%q) error = %v", id, err)
	}
}

func reservationForTest(ledger Ledger, id string) (Reservation, bool) {
	for _, reservation := range ledger.Reservations {
		if reservation.ID == id {
			return reservation, true
		}
	}
	return Reservation{}, false
}

func TestCheckInAndReleaseCreateCompletedReceipt(t *testing.T) {
	ledger := NewLedger()
	start := time.Date(2026, time.August, 17, 13, 0, 0, 0, time.UTC)
	checkInAt := start.Add(5 * time.Minute)
	releaseAt := start.Add(70 * time.Minute)
	requestForTest(t, &ledger, "r-300", "bench-c", start, start.Add(2*time.Hour))

	if _, err := CheckIn(&ledger, "r-300", checkInAt); err == nil {
		t.Fatal("CheckIn(pending) error = nil")
	}
	if _, err := Confirm(&ledger, "r-300"); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	occupied, err := CheckIn(&ledger, "r-300", checkInAt)
	if err != nil {
		t.Fatalf("CheckIn() error = %v", err)
	}
	if occupied.State != StateOccupied || occupied.CheckedInAt == nil {
		t.Fatalf("checked-in reservation = %#v", occupied)
	}
	if _, err := CheckIn(&ledger, "r-300", checkInAt); err == nil {
		t.Fatal("CheckIn(repeated) error = nil")
	}

	completed, err := Release(&ledger, "r-300", releaseAt)
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if completed.State != StateCompleted || completed.ReleasedAt == nil {
		t.Fatalf("released reservation = %#v", completed)
	}
	receipt, err := FindReceipt(ledger, "r-300")
	if err != nil {
		t.Fatalf("FindReceipt() error = %v", err)
	}
	if receipt.CheckedInAt == nil || receipt.ReleasedAt == nil {
		t.Fatalf("receipt timestamps = %#v", receipt)
	}
	if !receipt.CheckedInAt.Equal(checkInAt) || !receipt.ReleasedAt.Equal(releaseAt) {
		t.Fatalf("receipt timestamps = %#v", receipt)
	}
	if _, err := Release(&ledger, "r-300", releaseAt.Add(time.Minute)); err == nil {
		t.Fatal("Release(repeated) error = nil")
	}
	if len(ledger.Receipts) != 1 {
		t.Fatalf("receipt count = %d, want 1", len(ledger.Receipts))
	}
}

func TestReleaseRequiresCheckedInTimestamp(t *testing.T) {
	ledger := NewLedger()
	start := time.Date(2026, time.August, 17, 15, 0, 0, 0, time.UTC)
	requestForTest(t, &ledger, "r-301", "bench-d", start, start.Add(time.Hour))
	if _, err := Confirm(&ledger, "r-301"); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	reservation, ok := reservationForTest(ledger, "r-301")
	if !ok {
		t.Fatal("reservation not found")
	}
	reservation.State = StateOccupied
	for index := range ledger.Reservations {
		if ledger.Reservations[index].ID == reservation.ID {
			ledger.Reservations[index] = reservation
		}
	}
	if _, err := Release(&ledger, "r-301", start.Add(30*time.Minute)); err == nil {
		t.Fatal("Release(missing check-in) error = nil")
	}
	if got, _ := reservationForTest(ledger, "r-301"); got.State != StateOccupied || got.ReleasedAt != nil {
		t.Fatalf("failed release mutated reservation = %#v", got)
	}
}

func TestValidateRejectsOverlappingActiveReservations(t *testing.T) {
	ledger := NewLedger()
	start := time.Date(2026, time.August, 17, 16, 0, 0, 0, time.UTC)
	requestForTest(t, &ledger, "r-302", "bench-e", start, start.Add(time.Hour))
	requestForTest(t, &ledger, "r-303", "bench-e", start.Add(30*time.Minute), start.Add(90*time.Minute))
	if _, err := Confirm(&ledger, "r-302"); err != nil {
		t.Fatalf("Confirm(first) error = %v", err)
	}
	for index := range ledger.Reservations {
		if ledger.Reservations[index].ID == "r-303" {
			ledger.Reservations[index].State = StateConfirmed
		}
	}
	if err := ledger.Validate(); err == nil {
		t.Fatal("Validate() error = nil")
	}
}
