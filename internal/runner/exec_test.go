package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func helperCommand(mode string) CommandRequest {
	return CommandRequest{
		Operation:    "runner.helper",
		Executable:   os.Args[0],
		Argv:         []string{"-test.run=TestExecRunnerHelper", "literal argument"},
		Cwd:          "/",
		Env:          map[string]string{"RUNNER_HELPER": "1", "RUNNER_MODE": mode},
		Scope:        ScopeUser,
		Network:      NetworkNone,
		OutputPolicy: OutputCaptureRedacted,
		Timeout:      time.Second,
		OutputLimit:  1024,
	}
}

func TestExecRunnerHelper(t *testing.T) {
	if os.Getenv("RUNNER_HELPER") != "1" {
		return
	}

	switch os.Getenv("RUNNER_MODE") {
	case "inspect":
		cwd, _ := os.Getwd()
		stdin, _ := io.ReadAll(os.Stdin)
		args, _ := json.Marshal(os.Args)
		fmt.Fprintf(os.Stdout, "cwd=%s\nexplicit=%s\ninherited=%s\nstdin=%s\nargs=%s\n", cwd, os.Getenv("RUNNER_EXPLICIT"), os.Getenv("RUNNER_INHERITED"), stdin, args)
		fmt.Fprint(os.Stderr, "stderr")
	case "sensitive":
		fmt.Fprint(os.Stdout, "token=top-secret password=hunter2")
		fmt.Fprint(os.Stderr, "authorization: Bearer secret-token")
	case "large":
		fmt.Fprint(os.Stdout, strings.Repeat("o", 256))
		fmt.Fprint(os.Stderr, strings.Repeat("e", 256))
	case "sleep":
		time.Sleep(time.Second)
	case "exit":
		os.Exit(7)
	}
	os.Exit(0)
}
func TestExecRunnerExecutesArgvWithExplicitEnvironmentAndStdin(t *testing.T) {
	cwd := t.TempDir()
	t.Setenv("RUNNER_INHERITED", "must-not-leak")
	request := helperCommand("inspect")
	request.Cwd = cwd
	request.Env["RUNNER_EXPLICIT"] = "present"
	request.Stdin = []byte("input bytes")

	result, err := (ExecRunner{}).Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	output := string(result.Stdout)
	if !strings.Contains(output, "cwd="+cwd) || !strings.Contains(output, "explicit=present") || !strings.Contains(output, "stdin=input bytes") {
		t.Fatalf("helper output = %q", output)
	}
	if strings.Contains(output, "must-not-leak") || strings.Contains(output, "inherited=must-not-leak") {
		t.Fatalf("inherited environment leaked: %q", output)
	}
	if !strings.Contains(output, "literal argument") {
		t.Fatalf("argv was not passed literally: %q", output)
	}
	if string(result.Stderr) != "stderr" {
		t.Fatalf("stderr = %q", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}
func TestExecRunnerRedactsAndBoundsCapturedOutput(t *testing.T) {
	request := helperCommand("sensitive")
	result, err := (ExecRunner{}).Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if strings.Contains(string(result.Stdout), "top-secret") || strings.Contains(string(result.Stderr), "secret-token") {
		t.Fatalf("sensitive output was not redacted: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}

	request = helperCommand("large")
	request.OutputLimit = 16
	result, err = (ExecRunner{}).Run(context.Background(), request)
	if err != nil {
		t.Fatalf("bounded Run() error = %v", err)
	}
	if len(result.Stdout) > int(request.OutputLimit) || len(result.Stderr) > int(request.OutputLimit) {
		t.Fatalf("output exceeded limit: stdout=%d stderr=%d", len(result.Stdout), len(result.Stderr))
	}
}

func TestExecRunnerDiscardsOutput(t *testing.T) {
	request := helperCommand("large")
	request.OutputPolicy = OutputDiscard
	result, err := (ExecRunner{}).Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Stdout) != 0 || len(result.Stderr) != 0 {
		t.Fatalf("discarded output = %#v", result)
	}
}
func TestExecRunnerUsesTimeoutAndExitStatus(t *testing.T) {
	request := helperCommand("sleep")
	request.Timeout = 30 * time.Millisecond
	_, err := (ExecRunner{}).Run(context.Background(), request)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %v", err)
	}

	request = helperCommand("exit")
	result, err := (ExecRunner{}).Run(context.Background(), request)
	if err == nil || result.ExitCode != 7 {
		t.Fatalf("non-zero result = %#v, error = %v", result, err)
	}
}
func TestExecRunnerRejectsRelativeExecutableAndSystemScope(t *testing.T) {
	request := helperCommand("inspect")
	request.Executable = "runner-helper"
	if _, err := (ExecRunner{}).Run(context.Background(), request); !errors.Is(err, ErrInvalidCommandRequest) {
		t.Fatalf("relative executable error = %v", err)
	}

	request = helperCommand("inspect")
	request.Scope = ScopeSystem
	if _, err := (ExecRunner{}).Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "system") {
		t.Fatalf("system scope error = %v", err)
	}
}
