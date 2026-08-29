package runner

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func elevationRequest(operation, executable string, scope Scope, argv ...string) CommandRequest {
	return CommandRequest{
		Operation:    operation,
		Executable:   executable,
		Argv:         argv,
		Cwd:          "/fixture",
		Env:          map[string]string{"PATH": "/usr/bin"},
		Stdin:        []byte("input"),
		Scope:        scope,
		Network:      NetworkNone,
		OutputPolicy: OutputCaptureRedacted,
		Timeout:      time.Second,
		OutputLimit:  1024,
	}
}
func TestElevationRunnerDecoratesSystemAndPreservesUserRequests(t *testing.T) {
	system := elevationRequest("system-op", "/usr/bin/tool", ScopeSystem, "--flag", "value")
	user := elevationRequest("user-op", "/usr/bin/user-tool", ScopeUser, "--user")
	delegate := NewFakeRunner(
		Expectation{Operation: system.Operation, Argv: []string{"/usr/bin/tool", "--flag", "value"}, Result: CommandResult{ExitCode: 3}},
		Expectation{Operation: user.Operation, Argv: user.Argv, Result: CommandResult{ExitCode: 4}},
	)

	decorator := NewElevationRunner(delegate)
	result, err := decorator.Run(context.Background(), system)
	if err != nil || result.ExitCode != 3 {
		t.Fatalf("system Run() = %#v, %v", result, err)
	}
	result, err = decorator.Run(context.Background(), user)
	if err != nil || result.ExitCode != 4 {
		t.Fatalf("user Run() = %#v, %v", result, err)
	}

	requests := delegate.Requests()
	if len(requests) != 2 {
		t.Fatalf("delegated requests = %#v", requests)
	}
	wrapped := requests[0]
	if wrapped.Executable != "/usr/bin/pkexec" || wrapped.Scope != ScopeUser {
		t.Fatalf("wrapped request = %#v", wrapped)
	}
	if !reflect.DeepEqual(wrapped.Argv, []string{"/usr/bin/tool", "--flag", "value"}) {
		t.Fatalf("wrapped argv = %#v", wrapped.Argv)
	}
	if got := requests[1]; !reflect.DeepEqual(got, user) {
		t.Fatalf("user request changed: got %#v want %#v", got, user)
	}
	if err := delegate.Verify(); err != nil {
		t.Fatal(err)
	}
}
func TestElevationRunnerRejectsNonAbsoluteSystemTarget(t *testing.T) {
	delegate := NewFakeRunner()
	request := elevationRequest("system-op", "tool", ScopeSystem, "--flag")
	if _, err := NewElevationRunner(delegate).Run(context.Background(), request); !errors.Is(err, ErrInvalidCommandRequest) {
		t.Fatalf("relative target error = %v", err)
	}
	if got := delegate.Requests(); len(got) != 0 {
		t.Fatalf("delegate received rejected request: %#v", got)
	}
}
func TestElevationRunnerValidatesRequestsBeforeDelegating(t *testing.T) {
	cases := []CommandRequest{
		elevationRequest("bad-sudo-executable", "/usr/bin/sudo", ScopeUser),
		elevationRequest("bad-sudo-argv", "/usr/bin/tool", ScopeUser, "sudo"),
		elevationRequest("bad-shell", "/usr/bin/sh", ScopeUser),
	}
	for _, request := range cases {
		delegate := NewFakeRunner()
		if _, err := NewElevationRunner(delegate).Run(context.Background(), request); !errors.Is(err, ErrInvalidCommandRequest) {
			t.Errorf("%s error = %v", request.Operation, err)
		}
		if got := delegate.Requests(); len(got) != 0 {
			t.Errorf("%s reached delegate: %#v", request.Operation, got)
		}
	}
}
