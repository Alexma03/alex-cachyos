package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/runner"
	"alex-cachyos/internal/statepath"
)

type commandRuntimeSpy struct {
	calls   int
	request app.CommandRequest
	result  app.CommandResult
	err     error
}

func (s *commandRuntimeSpy) Execute(_ context.Context, request app.CommandRequest) (app.CommandResult, error) {
	s.calls++
	s.request = request
	return s.result, s.err
}

func TestHiddenSelfHelperModeRequiresRootAndForwardsOnlyTypedInput(t *testing.T) {
	payload := []byte(`{"schema":"alex-cachyos.privileged-helper/v1","operation":"publish-files","files":[{"source":"/stage/file","destination":"/etc/managed","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mode":420}]}`)
	called := 0
	runtime := selfHelperRuntime{
		effectiveUID: func() int { return 0 },
		policy: func() (runner.SelfHelperPolicy, error) {
			return runner.SelfHelperPolicy{SourceUID: 1000, Destinations: map[string]fs.FileMode{}}, nil
		},
		execute: func(input io.Reader, _ runner.SelfHelperPolicy) error {
			called++
			got, _ := io.ReadAll(input)
			if !bytes.Equal(got, payload) {
				t.Fatalf("helper input = %q", got)
			}
			return nil
		},
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(payload), &stdout, &stderr, runtime); code != 0 || called != 1 || stdout.Len() != 0 {
		t.Fatalf("helper run = code %d called %d stdout %q stderr %q", code, called, stdout.String(), stderr.String())
	}
	runtime.effectiveUID = func() int { return 1000 }
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(payload), &stdout, &stderr, runtime); code == 0 || called != 1 {
		t.Fatalf("non-root helper run = code %d called %d", code, called)
	}
	if code := run([]string{runner.SelfHelperArgument, "unexpected"}, bytes.NewReader(payload), &stdout, &stderr, runtime); code == 0 || called != 1 {
		t.Fatalf("helper accepted extra argv = code %d called %d", code, called)
	}
}

func TestRunDispatchesTypedCommandAndPreservesSelections(t *testing.T) {
	runtime := &commandRuntimeSpy{result: app.CommandResult{Message: "plan ready"}}
	var stdout, stderr bytes.Buffer
	code := runWithRuntime([]string{"apply", "--host", "portable", "--only", "bootstrap,apps", "--without", "fingerprint", "--remove", "desktop", "--dry-run"}, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}, runtime)
	if code != 0 || runtime.calls != 1 || stderr.Len() != 0 {
		t.Fatalf("run = code %d calls %d stdout %q stderr %q", code, runtime.calls, stdout.String(), stderr.String())
	}
	if runtime.request.Command != app.CommandApply || runtime.request.Host != "portable" || !runtime.request.DryRun || strings.Join(runtime.request.Selection.Only, ",") != "bootstrap,apps" || strings.Join(runtime.request.Selection.Without, ",") != "fingerprint" || strings.Join(runtime.request.Remove, ",") != "desktop" {
		t.Fatalf("request = %#v", runtime.request)
	}
	if stdout.String() != "plan ready\n" {
		t.Fatalf("human output = %q", stdout.String())
	}
}

func TestRunRendersDeterministicJSONCheckAndUsesDriftExit(t *testing.T) {
	runtime := &commandRuntimeSpy{result: app.CommandResult{Check: &app.CheckReport{
		Drift: true, ExitIntent: app.ExitIntentNonZero,
		Findings: []app.CheckFinding{{Kind: app.FindingManagedFile, Code: "managed-file-drift", Class: app.DriftPostApply, Path: "/etc/example", Backup: "/etc/example.bak.alex-cachyos", Severity: "error"}},
	}}}
	var stdout, stderr bytes.Buffer
	code := runWithRuntime([]string{"check", "--host", "portable", "--json"}, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}, runtime)
	if code != 1 || stderr.Len() != 0 {
		t.Fatalf("run = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
	want := `{"command":"check","host":"portable","check":{"drift":true,"exitIntent":"nonzero","findings":[{"kind":"managed-file","code":"managed-file-drift","class":"post-apply","path":"/etc/example","backup":"/etc/example.bak.alex-cachyos","severity":"error"}],"warnings":null}}` + "\n"
	if stdout.String() != want {
		t.Fatalf("JSON output = %q, want %q", stdout.String(), want)
	}
}

func TestRunSanitizesRuntimeErrorsInHumanAndJSONModes(t *testing.T) {
	const secret = "token=raw-output-must-not-escape"
	for _, args := range [][]string{{"status"}, {"status", "--json"}} {
		runtime := &commandRuntimeSpy{err: errors.New(secret)}
		var stdout, stderr bytes.Buffer
		code := runWithRuntime(args, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}, runtime)
		combined := stdout.String() + stderr.String()
		if code == 0 || strings.Contains(combined, secret) || strings.Contains(combined, "token=") {
			t.Fatalf("run(%q) = code %d output %q", args, code, combined)
		}
		if !strings.Contains(combined, "command failed") {
			t.Fatalf("sanitized error output = %q", combined)
		}
	}
}

func TestRunRendersUsageErrorsInTheRequestedFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runWithRuntime([]string{"rollback", "--json"}, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}, &commandRuntimeSpy{})
	if code != 2 || stderr.Len() != 0 || stdout.String() != `{"error":{"code":"usage","message":"usage error: rollback requires exactly one of --receipt or --tag"}}`+"\n" {
		t.Fatalf("JSON usage = code %d stdout %q stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestRunHelpAndListDoNotRequireCommandRuntime(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--list"}} {
		runtime := &commandRuntimeSpy{err: errors.New("must not run")}
		var stdout, stderr bytes.Buffer
		if code := runWithRuntime(args, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}, runtime); code != 0 || runtime.calls != 0 || stderr.Len() != 0 || stdout.Len() == 0 {
			t.Fatalf("run(%q) = code %d calls %d stdout %q stderr %q", args, code, runtime.calls, stdout.String(), stderr.String())
		}
	}
}

func TestDefaultStatusAndReceiptReadTheAtomicCurrentReceipt(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	paths, err := statepath.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "receipts", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := receipt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	value.RunID = "run-current"
	if _, err := receipt.NewStoreFromPaths(paths).Publish(value); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"status", "--json"}, {"receipt", "show", "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, bytes.NewReader(nil), &stdout, &stderr, selfHelperRuntime{}); code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), `"runId":"run-current"`) {
			t.Fatalf("run(%q) = code %d stdout %q stderr %q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestHiddenSelfHelperModeDoesNotExposeExecutionErrors(t *testing.T) {
	runtime := selfHelperRuntime{
		effectiveUID: func() int { return 0 },
		policy:       func() (runner.SelfHelperPolicy, error) { return runner.SelfHelperPolicy{}, nil },
		execute:      func(io.Reader, runner.SelfHelperPolicy) error { return errors.New("secret-content") },
	}
	var stderr bytes.Buffer
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(nil), io.Discard, &stderr, runtime); code == 0 || stderr.String() != "privileged helper failed\n" {
		t.Fatalf("helper error = code %d stderr %q", code, stderr.String())
	}
}
