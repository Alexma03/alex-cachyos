package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"alex-cachyos/internal/app"
)

// RenderCommandResult owns both public output formats. Application code
// returns typed evidence and never writes to process streams directly.
func RenderCommandResult(machine bool, result app.CommandResult, output io.Writer) error {
	if machine {
		encoder := json.NewEncoder(output)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(result)
	}
	if result.Message != "" {
		_, err := fmt.Fprintln(output, result.Message)
		return err
	}
	if result.Check != nil {
		status := "clean"
		if result.Check.Drift {
			status = "drift"
		}
		if _, err := fmt.Fprintf(output, "check %s: %s\n", result.Host, status); err != nil {
			return err
		}
		for _, finding := range result.Check.Findings {
			if _, err := fmt.Fprintf(output, "%s %s %s", finding.Class, finding.Kind, firstNonempty(finding.Path, finding.Subject)); err != nil {
				return err
			}
			if finding.Backup != "" {
				if _, err := fmt.Fprintf(output, " backup=%s", finding.Backup); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(output); err != nil {
				return err
			}
		}
		for _, warning := range result.Check.Warnings {
			if _, err := fmt.Fprintf(output, "warning: %s\n", warning); err != nil {
				return err
			}
		}
		return nil
	}
	if result.Receipt != nil {
		_, err := fmt.Fprintf(output, "%s %s\n", result.Receipt.RunID, result.ReceiptPath)
		return err
	}
	_, err := fmt.Fprintln(output, result.Command)
	return err
}

type renderedError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// RenderCommandError exposes only allowlisted diagnostics. Arbitrary adapter,
// runner, or observer errors become a stable generic failure.
func RenderCommandError(machine bool, err error, stdout, stderr io.Writer) int {
	code, message, exit := safeCommandError(err)
	if machine {
		value := renderedError{}
		value.Error.Code, value.Error.Message = code, message
		if encodeErr := json.NewEncoder(stdout).Encode(value); encodeErr != nil {
			fmt.Fprintln(stderr, "command failed")
		}
		return exit
	}
	fmt.Fprintln(stderr, message)
	return exit
}

func safeCommandError(err error) (string, string, int) {
	var unknown *app.UnknownHostError
	switch {
	case errors.Is(err, ErrUsage):
		return "usage", err.Error(), ExitUsage
	case errors.As(err, &unknown):
		return "unknown-host", unknown.Error(), ExitUsage
	case errors.Is(err, app.ErrLockContention):
		return "lock-contention", "another mutating command is already running", ExitLockContention
	case errors.Is(err, app.ErrCommandUnavailable):
		return "runtime-unavailable", "command runtime unavailable", ExitUsage
	default:
		return "command-failed", "command failed", 1
	}
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "-"
}
