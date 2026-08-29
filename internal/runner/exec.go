package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
)

var ErrDirectSystemExecution = errors.New("system-scoped command requires elevation")

// ExecRunner executes validated user-scoped requests without invoking a shell.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if err := validateExecRequest(request); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	if ctx == nil {
		return CommandResult{ExitCode: -1}, errors.New("nil command context")
	}
	if request.Scope == ScopeSystem {
		return CommandResult{ExitCode: -1}, ErrDirectSystemExecution
	}
	timedContext, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	command := exec.CommandContext(timedContext, request.Executable, request.Argv...)
	command.Dir = request.Cwd
	command.Env = explicitEnvironment(request.Env)
	if len(request.Stdin) > 0 {
		command.Stdin = bytes.NewReader(request.Stdin)
	}
	var stdout, stderr boundedBuffer
	switch request.OutputPolicy {
	case OutputDiscard:
		command.Stdout = io.Discard
		command.Stderr = io.Discard
	case OutputCaptureRedacted, OutputStreamSafe:
		stdout.limit = request.OutputLimit
		stderr.limit = request.OutputLimit
		command.Stdout = &stdout
		command.Stderr = &stderr
	}
	err := command.Run()
	result := CommandResult{ExitCode: 0}
	if request.OutputPolicy != OutputDiscard {
		result.Stdout = redactAndBound(stdout.Bytes(), request.OutputLimit)
		result.Stderr = redactAndBound(stderr.Bytes(), request.OutputLimit)
	}
	if err == nil {
		return result, nil
	}

	result.ExitCode = -1
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	}
	if timedContext.Err() != nil {
		return result, timedContext.Err()
	}
	return result, err
}

func validateExecRequest(request CommandRequest) error {
	if err := ValidateCommandRequest(request); err != nil {
		return err
	}
	if !filepath.IsAbs(request.Executable) {
		return invalidRequest("executable", "must be an absolute path")
	}
	return nil
}

func explicitEnvironment(environment map[string]string) []string {
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+environment[key])
	}
	return values
}

type boundedBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - int64(b.Len())
	if remaining > 0 {
		n := len(p)
		if int64(n) > remaining {
			n = int(remaining)
		}
		_, _ = b.Buffer.Write(p[:n])
	}
	return len(p), nil
}

var (
	bearerOutput = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]+`)
	secretOutput = regexp.MustCompile(`(?i)(\b(?:password|passwd|secret|token|api[-_]?key|authorization|auth[-_]?token|credential|cookie|private[-_]?key)\b\s*[:=]\s*)[^\s,;]+`)
)

func redactAndBound(output []byte, limit int64) []byte {
	if len(output) == 0 {
		return nil
	}
	redacted := bearerOutput.ReplaceAll(output, []byte("${1}[REDACTED]"))
	redacted = secretOutput.ReplaceAll(redacted, []byte("${1}[REDACTED]"))
	if int64(len(redacted)) > limit {
		redacted = redacted[:limit]
	}
	return redacted
}
