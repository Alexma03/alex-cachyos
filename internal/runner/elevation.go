package runner

import (
	"context"
	"errors"
	"path/filepath"
)

const pkexecExecutable = "/usr/bin/pkexec"

// ElevationRunner decorates system-scoped requests with pkexec and leaves
// user-scoped requests at the delegate unchanged.
type ElevationRunner struct {
	Runner Runner
}

func NewElevationRunner(runner Runner) *ElevationRunner {
	return &ElevationRunner{Runner: runner}
}

func (r *ElevationRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if r == nil || r.Runner == nil {
		return CommandResult{ExitCode: -1}, errors.New("nil elevation runner delegate")
	}
	if err := ValidateCommandRequest(request); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	if request.Scope == ScopeUser {
		return r.Runner.Run(ctx, request)
	}
	if !filepath.IsAbs(request.Executable) {
		return CommandResult{ExitCode: -1}, invalidRequest("executable", "must be absolute for system scope")
	}

	wrapped := request
	wrapped.Executable = pkexecExecutable
	wrapped.Scope = ScopeUser
	wrapped.Argv = make([]string, len(request.Argv)+1)
	wrapped.Argv[0] = request.Executable
	copy(wrapped.Argv[1:], request.Argv)
	return r.Runner.Run(ctx, wrapped)
}
