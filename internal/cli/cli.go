package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/benzhi/benchslot/internal/benchslot"
	"github.com/benzhi/benchslot/internal/ledger"
)

const defaultLedgerPath = "benchslot.json"

func Run(args []string, output, errorsOut io.Writer) int {
	if output == nil {
		output = io.Discard
	}
	if errorsOut == nil {
		errorsOut = io.Discard
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		writeUsage(output)
		return 0
	}
	switch args[0] {
	case "request":
		return runRequest(args[1:], output, errorsOut)
	case "confirm":
		return runConfirm(args[1:], output, errorsOut)
	case "check-in":
		return runCheckIn(args[1:], output, errorsOut)
	case "release":
		return runRelease(args[1:], output, errorsOut)
	case "show":
		return runShow(args[1:], output, errorsOut)
	case "smoke":
		if len(args) != 1 {
			return fail(errorsOut, "smoke does not accept arguments")
		}
		return runSmoke(output, errorsOut)
	default:
		return fail(errorsOut, "unknown command %q", args[0])
	}
}

func runRequest(args []string, output, errorsOut io.Writer) int {
	flags := flag.NewFlagSet("request", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ledgerPath := flags.String("ledger", defaultLedgerPath, "ledger path")
	idFlag := flags.String("id", "", "reservation id")
	bench := flags.String("bench", "", "bench name")
	startFlag := flags.String("start", "", "start time in RFC3339 format")
	endFlag := flags.String("end", "", "end time in RFC3339 format")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(normalizeArgs(args, "ledger", "id", "bench", "start", "end")); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if *help {
		writeCommandUsage(output, "request --ledger PATH --id ID --bench NAME --start RFC3339 --end RFC3339")
		return 0
	}
	id, err := chooseID(*idFlag, flags.Args())
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	start, err := parseTime("start", *startFlag)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	end, err := parseTime("end", *endFlag)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	value, err := ledger.Load(*ledgerPath)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	if _, err := benchslot.Request(&value, id, *bench, start, end); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if err := ledger.Save(*ledgerPath, value); err != nil {
		return fail(errorsOut, "%v", err)
	}
	fmt.Fprintf(output, "requested %s\n", id)
	return 0
}

func runConfirm(args []string, output, errorsOut io.Writer) int {
	help, ledgerPath, id, ok := parseTransitionFlags("confirm", args, output, errorsOut)
	if !ok {
		return 1
	}
	if help {
		return 0
	}
	return mutateReservation(ledgerPath, id, func(value *benchslot.Ledger) (benchslot.Reservation, error) {
		return benchslot.Confirm(value, id)
	}, "confirmed", output, errorsOut)
}

func runCheckIn(args []string, output, errorsOut io.Writer) int {
	flags := flag.NewFlagSet("check-in", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ledgerPath := flags.String("ledger", defaultLedgerPath, "ledger path")
	idFlag := flags.String("id", "", "reservation id")
	atFlag := flags.String("at", "", "check-in time in RFC3339 format")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(normalizeArgs(args, "ledger", "id", "at")); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if *help {
		writeCommandUsage(output, "check-in --ledger PATH ID --at RFC3339")
		return 0
	}
	id, err := chooseID(*idFlag, flags.Args())
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	at, err := parseTime("at", *atFlag)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	return mutateReservationAt(*ledgerPath, id, at, func(value *benchslot.Ledger, when time.Time) (benchslot.Reservation, error) {
		return benchslot.CheckIn(value, id, when)
	}, "checked-in", output, errorsOut)
}

func runRelease(args []string, output, errorsOut io.Writer) int {
	flags := flag.NewFlagSet("release", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ledgerPath := flags.String("ledger", defaultLedgerPath, "ledger path")
	idFlag := flags.String("id", "", "reservation id")
	atFlag := flags.String("at", "", "release time in RFC3339 format")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(normalizeArgs(args, "ledger", "id", "at")); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if *help {
		writeCommandUsage(output, "release --ledger PATH ID --at RFC3339")
		return 0
	}
	id, err := chooseID(*idFlag, flags.Args())
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	at, err := parseTime("at", *atFlag)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	return mutateReservationAt(*ledgerPath, id, at, func(value *benchslot.Ledger, when time.Time) (benchslot.Reservation, error) {
		return benchslot.Release(value, id, when)
	}, "completed", output, errorsOut)
}

func runShow(args []string, output, errorsOut io.Writer) int {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ledgerPath := flags.String("ledger", defaultLedgerPath, "ledger path")
	idFlag := flags.String("id", "", "receipt id")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(normalizeArgs(args, "ledger", "id")); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if *help {
		writeCommandUsage(output, "show --ledger PATH ID")
		return 0
	}
	id, err := chooseID(*idFlag, flags.Args())
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	value, err := ledger.Load(*ledgerPath)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	receipt, err := benchslot.FindReceipt(value, id)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(receipt); err != nil {
		return fail(errorsOut, "render receipt: %v", err)
	}
	return 0
}

func parseTransitionFlags(name string, args []string, output, errorsOut io.Writer) (bool, string, string, bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	ledgerPath := flags.String("ledger", defaultLedgerPath, "ledger path")
	idFlag := flags.String("id", "", "reservation id")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(normalizeArgs(args, "ledger", "id")); err != nil {
		fail(errorsOut, "%v", err)
		return false, "", "", false
	}
	if *help {
		writeCommandUsage(output, name+" --ledger PATH ID")
		return true, "", "", true
	}
	id, err := chooseID(*idFlag, flags.Args())
	if err != nil {
		fail(errorsOut, "%v", err)
		return false, "", "", false
	}
	return false, *ledgerPath, id, true
}

func mutateReservation(path, id string, action func(*benchslot.Ledger) (benchslot.Reservation, error), message string, output, errorsOut io.Writer) int {
	value, err := ledger.Load(path)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	if _, err := action(&value); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if err := ledger.Save(path, value); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if _, err := fmt.Fprintf(output, "%s %s\n", message, id); err != nil {
		return fail(errorsOut, "write response: %v", err)
	}
	return 0
}

func mutateReservationAt(path, id string, at time.Time, action func(*benchslot.Ledger, time.Time) (benchslot.Reservation, error), message string, output, errorsOut io.Writer) int {
	value, err := ledger.Load(path)
	if err != nil {
		return fail(errorsOut, "%v", err)
	}
	if _, err := action(&value, at); err != nil {
		return fail(errorsOut, "%v", err)
	}
	if err := ledger.Save(path, value); err != nil {
		return fail(errorsOut, "%v", err)
	}
	fmt.Fprintf(output, "%s %s\n", message, id)
	return 0
}

func runSmoke(output, errorsOut io.Writer) int {
	directory, err := os.MkdirTemp("", "benchslot-smoke-")
	if err != nil {
		return fail(errorsOut, "create smoke directory: %v", err)
	}
	status := 0
	path := filepath.Join(directory, "ledger.json")
	commands := [][]string{
		{"request", "--ledger", path, "--id", "smoke-1", "--bench", "bench-smoke", "--start", "2026-08-17T09:00:00Z", "--end", "2026-08-17T10:00:00Z"},
		{"confirm", "--ledger", path, "smoke-1"},
		{"check-in", "--ledger", path, "smoke-1", "--at", "2026-08-17T09:05:00Z"},
		{"release", "--ledger", path, "smoke-1", "--at", "2026-08-17T10:05:00Z"},
	}
	var childOutput, childErrors bytes.Buffer
	for _, command := range commands {
		if Run(command, &childOutput, &childErrors) != 0 {
			status = 1
			break
		}
	}
	if status == 0 {
		childOutput.Reset()
		if Run([]string{"show", "--ledger", path, "smoke-1"}, &childOutput, &childErrors) != 0 {
			status = 1
		} else {
			var receipt benchslot.Receipt
			if err := json.Unmarshal(childOutput.Bytes(), &receipt); err != nil {
				childErrors.WriteString(err.Error())
				status = 1
			} else if receipt.CheckedInAt == nil || receipt.ReleasedAt == nil {
				childErrors.WriteString("receipt is missing transition timestamps")
				status = 1
			}
		}
	}
	if cleanupErr := os.RemoveAll(directory); cleanupErr != nil {
		childErrors.WriteString(cleanupErr.Error())
		status = 1
	}
	if status != 0 {
		return fail(errorsOut, "smoke workflow failed: %s", strings.TrimSpace(childErrors.String()))
	}
	fmt.Fprintln(output, "smoke passed")
	return 0
}

func chooseID(flagValue string, positional []string) (string, error) {
	if flagValue != "" && len(positional) != 0 {
		return "", errors.New("provide an id flag or positional id, not both")
	}
	if flagValue != "" {
		return flagValue, nil
	}
	if len(positional) == 1 && strings.TrimSpace(positional[0]) != "" {
		return positional[0], nil
	}
	return "", errors.New("one reservation id is required")
}

func normalizeArgs(args []string, valueFlags ...string) []string {
	values := make(map[string]struct{}, len(valueFlags))
	for _, name := range valueFlags {
		values[name] = struct{}{}
	}
	var flags []string
	var positional []string
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") {
			positional = append(positional, argument)
			continue
		}
		flags = append(flags, argument)
		name := strings.TrimLeft(argument, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if _, isValue := values[name]; isValue && index+1 < len(args) {
			index++
			flags = append(flags, args[index])
		}
	}
	return append(flags, positional...)
}

func parseTime(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("%s time is required", name)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s time must use RFC3339: %w", name, err)
	}
	return parsed, nil
}

func fail(output io.Writer, format string, values ...any) int {
	fmt.Fprintf(output, "benchslot: %s\n", fmt.Sprintf(format, values...))
	return 1
}

func writeUsage(output io.Writer) {
	fmt.Fprintln(output, "benchslot: timed workshop bench reservations")
	fmt.Fprintln(output, "commands: request, confirm, check-in, release, show, smoke")
	fmt.Fprintln(output, "use benchslot COMMAND --help for command details")
}

func writeCommandUsage(output io.Writer, usage string) {
	fmt.Fprintf(output, "benchslot %s\n", usage)
}
