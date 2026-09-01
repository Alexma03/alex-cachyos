package cachyos

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"alex-cachyos/internal/executor"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

func TestRunnerDesktopPackagePortReturnsOnlyRequestedCanonicalIdentities(t *testing.T) {
	request, err := makeRequest("desktop.packages.observe", "/usr/bin/pacman", []string{"-Q", "niri", "noctalia"}, runner.ScopeUser, runner.NetworkNone, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := runner.NewFakeRunner(runner.Expectation{
		Operation: request.Operation,
		Argv:      request.Argv,
		Result:    runner.CommandResult{ExitCode: 1, Stdout: []byte("niri 25.05\n")},
		Err:       errors.New("one package is missing"),
	})
	installed, err := (RunnerDesktopPackagePort{Runner: run}).Installed(context.Background(), []string{"niri", "noctalia"})
	if err != nil {
		t.Fatal(err)
	}
	if installed["niri"] != "25.05" || installed["noctalia"] != "" || len(installed) != 1 {
		t.Fatalf("installed = %#v", installed)
	}
	if err := run.Verify(); err != nil {
		t.Fatal(err)
	}

	nonCanonical := runner.NewFakeRunner(runner.Expectation{
		Operation: request.Operation,
		Argv:      request.Argv,
		Result:    runner.CommandResult{Stdout: []byte("unrequested 1.0\n")},
	})
	if _, err := (RunnerDesktopPackagePort{Runner: nonCanonical}).Installed(context.Background(), []string{"niri", "noctalia"}); err == nil {
		t.Fatal("non-canonical package output was accepted")
	}
}

func TestOSDesktopFilePortPublishesAtomicallyAndAdoptsOnce(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "home", ".config", "niri", "config.kdl")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port, err := NewOSDesktopFilePort(filepath.Join(root, "state"), filepath.Join(root, "home"), "receipt-fixture")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := ManagedFileDescriptor{Path: target, Content: []byte("after\n"), Mode: 0o644, Kind: "niri-config"}
	if err := port.Publish(context.Background(), descriptor); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "after\n" {
		t.Fatalf("target = %q, %v", got, err)
	}
	if got, err := os.ReadFile(target + ".bak.alex-cachyos"); err != nil || string(got) != "before\n" {
		t.Fatalf("backup = %q, %v", got, err)
	}
	if err := port.Publish(context.Background(), descriptor); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target + ".bak.alex-cachyos"); err != nil || string(got) != "before\n" {
		t.Fatalf("backup changed = %q, %v", got, err)
	}
}

func TestOSDesktopFilePortRejectsSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "home", ".dmrc")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatal(err)
	}
	port, err := NewOSDesktopFilePort(filepath.Join(root, "state"), filepath.Join(root, "home"), "receipt-fixture")
	if err != nil {
		t.Fatal(err)
	}
	err = port.Publish(context.Background(), ManagedFileDescriptor{Path: target, Content: []byte("replacement"), Mode: 0o600, Kind: "session"})
	if err == nil || !errors.Is(err, ErrUnsafeDesktopFile) {
		t.Fatalf("symlink publish error = %v", err)
	}
	if got, _ := os.ReadFile(outside); string(got) != "outside" {
		t.Fatalf("outside file changed: %q", got)
	}
}

func TestOSDesktopFilePortRejectsTargetsOutsideResolvedHome(t *testing.T) {
	root := t.TempDir()
	port, err := NewOSDesktopFilePort(filepath.Join(root, "state"), filepath.Join(root, "home"), "receipt-fixture")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside", "config")
	if err := port.Publish(context.Background(), ManagedFileDescriptor{Path: outside, Content: []byte("no"), Mode: 0o600, Kind: "outside"}); !errors.Is(err, ErrUnsafeDesktopFile) {
		t.Fatalf("outside publish error = %v", err)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside target exists: %v", err)
	}
}

func TestDesktopRuntimeDryRunApplyAndSecondRunConverge(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	requestPlan, err := BuildDesktopRequestPlan(portableDesktopPolicy(), DesktopObservation{HomeRoot: home, UserName: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	packages := &memoryDesktopPackages{installed: map[string]string{}}
	files := &memoryDesktopFiles{values: map[string]ManagedFileDescriptor{}}
	run := &desktopRunner{after: func() {
		for _, name := range requestPlan.Packages {
			packages.installed[name] = "1.0"
		}
	}}
	runtime, err := NewDesktopRuntime(requestPlan, run, desktopNetworkPort{}, packages, files)
	if err != nil {
		t.Fatal(err)
	}
	plan := planner.Plan{Steps: requestPlan.Steps}
	dry, err := executor.NewExecutor(runtime, runtime).Execute(context.Background(), plan, executor.Options{DryRun: true})
	if err != nil || dry.NoChange || run.calls != 0 || files.publishes != 0 {
		t.Fatalf("dry run = %#v, err=%v, calls=%d, files=%d", dry, err, run.calls, files.publishes)
	}
	first, err := executor.NewExecutor(runtime, runtime).Execute(context.Background(), plan)
	if err != nil || first.NoChange || run.calls != 1 || files.publishes != len(requestPlan.Files) {
		t.Fatalf("first run = %#v, err=%v, calls=%d, files=%d", first, err, run.calls, files.publishes)
	}
	second, err := executor.NewExecutor(runtime, runtime).Execute(context.Background(), plan)
	if err != nil || !second.NoChange || run.calls != 1 || files.publishes != len(requestPlan.Files) {
		t.Fatalf("second run = %#v, err=%v, calls=%d, files=%d", second, err, run.calls, files.publishes)
	}
}

type memoryDesktopPackages struct{ installed map[string]string }

func (p *memoryDesktopPackages) Installed(context.Context, []string) (map[string]string, error) {
	out := make(map[string]string, len(p.installed))
	for name, version := range p.installed {
		out[name] = version
	}
	return out, nil
}

type memoryDesktopFiles struct {
	values    map[string]ManagedFileDescriptor
	publishes int
}

func (f *memoryDesktopFiles) Observe(_ context.Context, path string) (DesktopFileObservation, error) {
	value, ok := f.values[path]
	if !ok {
		return DesktopFileObservation{Path: path}, nil
	}
	return desktopObservationForDescriptor(value), nil
}

func (f *memoryDesktopFiles) Publish(_ context.Context, file ManagedFileDescriptor) error {
	f.publishes++
	f.values[file.Path] = file.Clone()
	return nil
}

type desktopRunner struct {
	calls int
	after func()
}

type desktopNetworkPort struct{}

func (desktopNetworkPort) Authorize(context.Context, runner.CommandRequest) error { return nil }

func (r *desktopRunner) Run(_ context.Context, request runner.CommandRequest) (runner.CommandResult, error) {
	r.calls++
	if request.Operation != DesktopPackagesOperation {
		return runner.CommandResult{ExitCode: -1}, errors.New("unexpected request")
	}
	if r.after != nil {
		r.after()
	}
	return runner.CommandResult{}, nil
}
