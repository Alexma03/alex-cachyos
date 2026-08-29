package gitx

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

type CommandRequest struct {
	Operation  string
	Executable string
	Argv       []string
	Cwd        string
}
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}
type Runner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if request.Executable == "" {
		return CommandResult{ExitCode: -1}, errors.New("git command has no executable")
	}
	command := exec.CommandContext(ctx, request.Executable, request.Argv...)
	command.Dir = request.Cwd
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	result := CommandResult{ExitCode: 0}
	err := command.Run()
	result.Stdout, result.Stderr = stdout.Bytes(), stderr.Bytes()
	if err == nil {
		return result, nil
	}
	result.ExitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	return result, err
}
