# BenchSlot

BenchSlot is a small command-line ledger for coordinating timed use of shared workshop benches. Each reservation follows a single workflow:

`pending` -> `confirmed` -> `occupied` -> `completed`

Completed reservations include a durable usage receipt with the check-in and release times. Reservations on the same bench use half-open intervals, so a booking may begin exactly when an earlier booking ends.

## Build and run

BenchSlot uses only the Go standard library and requires Go 1.22 or newer.

```text
go run ./cmd/benchslot --help
```

The default ledger is `benchslot.json`. Use `--ledger PATH` on each command to choose another local file.

## Workflow

Request a reservation with RFC3339 timestamps:

```text
go run ./cmd/benchslot request --ledger workshop.json --id r-100 --bench lathe-1 --start 2026-08-17T09:00:00Z --end 2026-08-17T10:30:00Z
go run ./cmd/benchslot confirm --ledger workshop.json r-100
go run ./cmd/benchslot check-in --ledger workshop.json r-100 --at 2026-08-17T09:05:00Z
go run ./cmd/benchslot release --ledger workshop.json r-100 --at 2026-08-17T10:20:00Z
go run ./cmd/benchslot show --ledger workshop.json r-100
```

The ledger is a versioned JSON document. Successful changes are written through a sibling temporary file, synchronized, closed, and atomically renamed into place. A missing ledger starts empty; an existing ledger must be complete and valid before a command can act on it.

## Verification

Run the package tests and the bounded local workflow check:

```text
go test ./...
go run ./cmd/benchslot smoke
```
