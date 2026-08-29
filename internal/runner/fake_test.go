package runner

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func testRequest(operation string, argv ...string) CommandRequest {
	return CommandRequest{Operation: operation, Executable: "tool", Argv: append([]string(nil), argv...), Cwd: "/fixture", Env: map[string]string{"PATH": "/usr/bin"}, Stdin: []byte("input"), Scope: ScopeUser, Network: NetworkNone, OutputPolicy: OutputCaptureRedacted, Timeout: time.Second, OutputLimit: 1024}
}

func TestValidateCommandRequest(t *testing.T) {
	base := testRequest("observe", "--name", "fixture")
	if err := ValidateCommandRequest(base); err != nil { t.Fatal(err) }
	cases := []struct { name string; edit func(*CommandRequest) }{
		{"shell executable", func(r *CommandRequest) { r.Executable = "/bin/sh" }},
		{"shell option", func(r *CommandRequest) { r.Argv = []string{"-c", "echo unsafe"} }},
		{"shell syntax", func(r *CommandRequest) { r.Argv = []string{"x;y"} }},
		{"sudo", func(r *CommandRequest) { r.Argv = []string{"sudo", "tool"} }},
		{"nul", func(r *CommandRequest) { r.Argv = []string{"bad\x00arg"} }},
		{"nul stdin", func(r *CommandRequest) { r.Stdin = []byte{'x', 0} }},
		{"unbounded timeout", func(r *CommandRequest) { r.Timeout = 0 }},
		{"unbounded output", func(r *CommandRequest) { r.OutputLimit = 0 }},
		{"secret environment", func(r *CommandRequest) { r.Env = map[string]string{"API_TOKEN": "literal-secret"} }},
		{"oversized stdin", func(r *CommandRequest) { r.Stdin = make([]byte, MaxStdinBytes+1) }},
	}
	for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { request := base; tc.edit(&request); if err := ValidateCommandRequest(request); err == nil { t.Fatal("unsafe request accepted") } }) }
}

func TestFakeRunnerMatchesInOrderAndCopies(t *testing.T) {
	fake := NewFakeRunner(
		Expectation{Operation: "observe", Argv: []string{"--name", "fixture"}, Result: CommandResult{Stdout: []byte("ok")}, Mutate: func(state *FixtureState) { state.Values["observed"] = "yes" }},
		Expectation{Operation: "apply", Argv: []string{"--name", "fixture"}, Result: CommandResult{ExitCode: 0}},
	)
	request := testRequest("observe", "--name", "fixture")
	result, err := fake.Run(context.Background(), request)
	if err != nil || string(result.Stdout) != "ok" { t.Fatalf("first Run() = %#v, %v", result, err) }
	request.Argv[0] = "changed"; request.Env["PATH"] = "/changed"; request.Stdin[0] = 'X'
	got := fake.Requests()
	if len(got) != 1 || got[0].Argv[0] != "--name" || got[0].Env["PATH"] != "/usr/bin" || string(got[0].Stdin) != "input" { t.Fatalf("request was not defensive: %#v", got) }
	got[0].Argv[0] = "changed copy"
	if fake.Requests()[0].Argv[0] != "--name" || fake.State().Values["observed"] != "yes" { t.Fatal("fake exposed internal data or skipped callback") }
	if _, err := fake.Run(context.Background(), testRequest("apply", "--name", "fixture")); err != nil { t.Fatal(err) }
	if err := fake.Verify(); err != nil { t.Fatal(err) }
}

func TestFakeRunnerReportsUnmatchedAndUnconsumed(t *testing.T) {
	fake := NewFakeRunner(Expectation{Operation: "expected", Argv: []string{"one"}})
	if _, err := fake.Run(context.Background(), testRequest("unexpected", "one")); !errors.Is(err, ErrUnmatchedExpectation) { t.Fatalf("unmatched error = %v", err) }
	if len(fake.UnmatchedRequests()) != 1 { t.Fatalf("unmatched requests = %#v", fake.UnmatchedRequests()) }
	if err := fake.Verify(); !errors.Is(err, ErrUnconsumedExpectations) || !strings.Contains(err.Error(), "expected") { t.Fatalf("unconsumed error = %v", err) }
}

func TestFakeRunnerSerializesConcurrentCallbacks(t *testing.T) {
	const calls = 16
	expectations := make([]Expectation, calls)
	for i := range expectations { expectations[i] = Expectation{Operation: "observe", Argv: []string{"fixture"}, Mutate: func(state *FixtureState) { state.Count++ }} }
	fake := NewFakeRunner(expectations...)
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ { wg.Add(1); go func() { defer wg.Done(); if _, err := fake.Run(context.Background(), testRequest("observe", "fixture")); err != nil { t.Errorf("Run() = %v", err) } }() }
	wg.Wait()
	if fake.State().Count != calls || len(fake.Requests()) != calls { t.Fatalf("fixture/request count = %d/%d", fake.State().Count, len(fake.Requests())) }
	if err := fake.Verify(); err != nil { t.Fatal(err) }
}
