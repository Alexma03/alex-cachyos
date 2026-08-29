package cli

import "errors"

const (
	ExitSuccess        = 0
	ExitUsage          = 2
	ExitLockContention = 75
)

type ExitCodeTable struct{ Usage, LockContention int }

func ExitCodes() ExitCodeTable { return ExitCodeTable{ExitUsage, ExitLockContention} }
func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	if errors.Is(err, ErrUsage) {
		return ExitUsage
	}
	return ExitUsage
}
