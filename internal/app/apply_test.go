package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"alex-cachyos/internal/executor"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/runner"
	"alex-cachyos/internal/statepath"
)

type applyFixture struct {
	value                               string
	bytes                               []byte
	observes, mutates, networkMutations int
}

func fixtureExecutor(f *applyFixture) *executor.Executor {
	observer := executor.ObserverFunc(func(context.Context, planner.Step) ([]byte, error) {
		f.observes++
		return json.Marshal(map[string]string{"value": f.value})
	})
	mutator := executor.MutatorFunc(func(_ context.Context, step planner.Step) error {
		f.mutates++
		if step.Network == planner.NetworkRequired {
			f.networkMutations++
		}
		f.value = "ready"
		f.bytes = []byte("changed")
		return nil
	})
	return executor.NewExecutor(observer, mutator)
}

type executorFunc func(context.Context, planner.Plan, ...executor.Options) (executor.Result, error)

func (f executorFunc) Execute(ctx context.Context, plan planner.Plan, options ...executor.Options) (executor.Result, error) {
	return f(ctx, plan, options...)
}

const applyEnvelopeJSON = `{"schema":"alex-cachyos.receipt/v1","runId":"seed","command":"apply","startedAt":"2026-01-02T03:04:01Z","finishedAt":"2026-01-02T03:04:02Z","status":"pending","noChange":false,"host":{"requested":"fixture","resolved":"fixture","hostname":"fixture"},"catalog":{"catalogVersion":"1","release":"catalog-v1","tag":"","digest":"catalog","source":"test"},"selection":{"only":[],"with":[],"without":[],"remove":[],"update":false,"dryRun":false},"plan":{"digest":"prebuilt","networkRequired":[]},"steps":[],"managedFiles":[],"mutations":[],"desiredPackages":{"pacmanNames":[],"pacmanRepositoryPolicy":"","pacmanTransactionPolicy":""},"desiredExactPins":{"npm":{},"sourceCheckouts":{},"aurLocalSources":{},"patches":{},"remoteArtifacts":{},"optionalPacmanArtifacts":{}},"resolvedInstalledVersions":{"pacman":{},"aur":{},"npm":{}},"systemTransactions":[],"checkouts":[],"piRuntime":{"gentlePiCommit":"","gentleAiCommit":"","activePath":"","version":"","binarySha256":"","buildManifestSha256":"","resolverSource":"","signedFallbackVersion":"","signedFallbackManifestSha256":""},"gentleAiInvocations":[],"managedAssets":{"manifestPath":"","manifestSha256":"","verifiedEntries":[]},"reviewMode":{"effective":"","deciding":""},"credentials":{"referencedNames":[]},"warnings":[],"errors":[]}`

func applyEnvelope(n string) receipt.Receipt {
	var result receipt.Receipt
	if err := json.Unmarshal([]byte(applyEnvelopeJSON), &result); err != nil {
		panic(err)
	}
	result.RunID = "run-" + n
	result.StartedAt = "2026-01-02T03:04:0" + n + "Z"
	result.FinishedAt = "2026-01-02T03:04:1" + n + "Z"
	return result
}

func applyPaths(t *testing.T) statepath.Paths {
	t.Helper()
	run := t.TempDir()
	if err := os.Chmod(run, 0700); err != nil {
		t.Fatal(err)
	}
	return statepath.Paths{StateHome: filepath.Join(t.TempDir(), "state"), RuntimeDir: run}
}

func applyPlan(network planner.NetworkClass) planner.Plan {
	return planner.Plan{
		Selection: planner.Selection{Only: []string{"apps"}},
		Steps: []planner.Step{{
			ID: "apps.install", Module: "apps", Scope: planner.ScopeUser, Network: network,
			Operation: "install", Disposition: planner.DispositionApply, Desired: []byte(`{"value":"ready"}`),
		}},
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readApplyReceipt(t *testing.T, path string) receipt.Receipt {
	t.Helper()
	result, err := receipt.Parse(mustRead(t, path))
	if err != nil {
		t.Fatalf("receipt parse: %v", err)
	}
	return result
}

func receiptCount(t *testing.T, paths statepath.Paths) int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(paths.StateHome, "alex-cachyos", "receipts", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	return len(files)
}

func mustApply(t *testing.T, applier *Applier, input ApplyInput) ApplyResult {
	t.Helper()
	result, err := applier.Apply(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertLockFree(t *testing.T, paths statepath.Paths) {
	t.Helper()
	lock, err := AcquireLock(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPublishesAndConverges(t *testing.T) {
	paths := applyPaths(t)
	fixture := &applyFixture{value: "old", bytes: []byte("before")}
	lockHeldDuringPublish := false
	store := func(paths statepath.Paths) *receipt.Store {
		_, err := AcquireLock(paths)
		lockHeldDuringPublish = errors.Is(err, ErrLockContention)
		return receipt.NewStoreFromPaths(paths)
	}
	applier := NewApplier(fixtureExecutor(fixture), store)
	input := ApplyInput{Envelope: applyEnvelope("1"), Plan: applyPlan(planner.NetworkNone), Paths: paths}

	first := mustApply(t, applier, input)
	firstReceipt := readApplyReceipt(t, first.ReceiptPath)
	if firstReceipt.Status != "success" || firstReceipt.NoChange || firstReceipt.Selection.DryRun ||
		firstReceipt.Steps[0].Outcome != executor.OutcomeApplied || !lockHeldDuringPublish {
		t.Fatalf("first receipt = %#v", firstReceipt)
	}
	before := append([]byte(nil), mustRead(t, first.ReceiptPath)...)

	input.Envelope = applyEnvelope("2")
	second := mustApply(t, applier, input)
	secondReceipt := readApplyReceipt(t, second.ReceiptPath)
	if !secondReceipt.NoChange || secondReceipt.Status != "success" || fixture.mutates != 1 || receiptCount(t, paths) != 2 {
		t.Fatalf("second receipt = %#v, fixture = %#v", secondReceipt, fixture)
	}
	if !bytes.Equal(before, mustRead(t, first.ReceiptPath)) {
		t.Fatal("first receipt changed")
	}
}

func TestApplyDryRunGroupsAndSkipsLock(t *testing.T) {
	paths := applyPaths(t)
	fixture := &applyFixture{value: "old", bytes: []byte("before")}
	applier := NewApplier(fixtureExecutor(fixture), receipt.NewStoreFromPaths)
	lockCalled := false
	applier.lockFactory = func(statepath.Paths) (*Lock, error) {
		lockCalled = true
		return nil, errors.New("dry-run must not acquire the lock")
	}
	plan := applyPlan(planner.NetworkRequired)
	plan.Steps = append(plan.Steps, planner.Step{
		ID: "apps.fetch", Module: "apps", Scope: planner.ScopeUser, Network: planner.NetworkRequired,
		Operation: "fetch", Disposition: planner.DispositionApply, Desired: []byte(`{"value":"ready"}`),
	})
	beforeBytes := append([]byte(nil), fixture.bytes...)

	result := mustApply(t, applier, ApplyInput{Envelope: applyEnvelope("3"), Plan: plan, Paths: paths, DryRun: true})
	if lockCalled || fixture.mutates != 0 || fixture.networkMutations != 0 || fixture.value != "old" ||
		!bytes.Equal(beforeBytes, fixture.bytes) || len(result.NetworkGroups) != 2 || !result.Receipt.Selection.DryRun || result.Receipt.NoChange {
		t.Fatalf("dry run mutated: result=%#v fixture=%#v", result, fixture)
	}
	if result.NetworkGroups[0].Operation != planner.Operation("install") || result.NetworkGroups[1].Operation != planner.Operation("fetch") {
		t.Fatalf("network groups = %#v", result.NetworkGroups)
	}
	got := readApplyReceipt(t, result.ReceiptPath)
	if got.Status != "success" || len(got.Plan.NetworkRequired) != 2 || got.Plan.NetworkRequired[0] != "install" || got.Plan.NetworkRequired[1] != "fetch" {
		t.Fatalf("dry receipt = %#v", got)
	}
}

func TestCommandApplierRunsSystemMutationThroughFakeElevation(t *testing.T) {
	request := runner.CommandRequest{
		Operation: "system.publish", Executable: "/usr/bin/tool", Cwd: "/", Scope: runner.ScopeSystem,
		Network: runner.NetworkNone, OutputPolicy: runner.OutputDiscard, Timeout: time.Second, OutputLimit: 1024,
	}
	delegate := runner.NewFakeRunner(runner.Expectation{Operation: request.Operation, Argv: []string{request.Executable}})
	observations := 0
	observer := executor.ObserverFunc(func(context.Context, planner.Step) ([]byte, error) {
		observations++
		if observations < 3 {
			return []byte(`{"value":"old"}`), nil
		}
		return []byte(`{"value":"ready"}`), nil
	})
	applier, err := NewCommandApplier(observer, runner.NewElevationRunner(delegate), []runner.CommandRequest{request}, nil, receipt.NewStoreFromPaths)
	if err != nil {
		t.Fatal(err)
	}
	plan := applyPlan(planner.NetworkNone)
	plan.Steps[0].ID, plan.Steps[0].Scope = request.Operation, planner.ScopeSystem
	result := mustApply(t, applier, ApplyInput{Envelope: applyEnvelope("8"), Plan: plan, Paths: applyPaths(t)})
	requests := delegate.Requests()
	if result.Receipt.Status != "success" || len(requests) != 1 || requests[0].Executable != "/usr/bin/pkexec" ||
		!reflect.DeepEqual(requests[0].Argv, []string{request.Executable}) {
		t.Fatalf("production apply result/requests = %#v / %#v", result, requests)
	}
}

func TestApplyRejectsInvalidBoundaryBeforePorts(t *testing.T) {
	paths := applyPaths(t)
	lockCalls, executorCalls, publicationCalls := 0, 0, 0
	applier := NewApplier(executorFunc(func(context.Context, planner.Plan, ...executor.Options) (executor.Result, error) {
		executorCalls++
		return executor.Result{}, nil
	}), func(statepath.Paths) *receipt.Store {
		publicationCalls++
		return nil
	})
	applier.lockFactory = func(statepath.Paths) (*Lock, error) {
		lockCalls++
		return nil, errors.New("invalid boundary must stop first")
	}
	badPlan := applyPlan(planner.NetworkNone)
	badPlan.Steps[0].Desired = []byte("{")
	inputs := []ApplyInput{
		{Envelope: receipt.Receipt{}, Plan: applyPlan(planner.NetworkNone), Paths: paths},
		{Envelope: applyEnvelope("7"), Plan: badPlan, Paths: paths},
	}

	for _, input := range inputs {
		if _, err := applier.Apply(context.Background(), input); err == nil {
			t.Fatal("invalid boundary was accepted")
		}
	}
	if lockCalls != 0 || executorCalls != 0 || publicationCalls != 0 {
		t.Fatalf("invalid boundary called ports: lock=%d executor=%d publication=%d", lockCalls, executorCalls, publicationCalls)
	}
}

func TestApplyFailuresReleaseLockAndPublishOnce(t *testing.T) {
	paths := applyPaths(t)
	const secret = "callback-secret"
	failingExecutor := executor.NewExecutor(executor.ObserverFunc(func(context.Context, planner.Step) ([]byte, error) {
		return nil, errors.New(secret)
	}), nil)
	result, err := NewApplier(failingExecutor, receipt.NewStoreFromPaths).Apply(context.Background(), ApplyInput{
		Envelope: applyEnvelope("8"), Plan: applyPlan(planner.NetworkNone), Paths: paths,
	})
	if err == nil || result.ReceiptPath == "" || receiptCount(t, paths) != 1 {
		t.Fatalf("executor failure: result=%#v err=%v", result, err)
	}
	data := mustRead(t, result.ReceiptPath)
	if bytes.Contains(data, []byte(secret)) || readApplyReceipt(t, result.ReceiptPath).Status != "failed" {
		t.Fatalf("unsanitized receipt: %s", data)
	}
	assertLockFree(t, paths)

	paths = applyPaths(t)
	held, err := AcquireLock(paths)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &applyFixture{value: "old", bytes: []byte("before")}
	applier := NewApplier(fixtureExecutor(fixture), receipt.NewStoreFromPaths)
	result, err = applier.Apply(context.Background(), ApplyInput{Envelope: applyEnvelope("9"), Plan: applyPlan(planner.NetworkNone), Paths: paths})
	if releaseErr := held.Release(); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if !errors.Is(err, ErrLockContention) || result.ReceiptPath == "" || fixture.observes != 0 || receiptCount(t, paths) != 1 {
		t.Fatalf("contention: result=%#v err=%v fixture=%#v", result, err, fixture)
	}
	contentionReceipt := readApplyReceipt(t, result.ReceiptPath)
	if contentionReceipt.Status != "failed" || contentionReceipt.Errors[0] != "lock-contention" {
		t.Fatalf("contention receipt = %#v", contentionReceipt)
	}

	paths = applyPaths(t)
	fixture = &applyFixture{value: "old", bytes: []byte("before")}
	applier = NewApplier(fixtureExecutor(fixture), receipt.NewStoreFromPaths)
	mustApply(t, applier, ApplyInput{Envelope: applyEnvelope("0"), Plan: applyPlan(planner.NetworkNone), Paths: paths})
	currentPath := filepath.Join(paths.StateHome, "alex-cachyos", "current.json")
	current := append([]byte(nil), mustRead(t, currentPath)...)
	brokenRoot := filepath.Join(paths.StateHome, "not-a-store-directory")
	if err := os.WriteFile(brokenRoot, nil, 0600); err != nil {
		t.Fatal(err)
	}
	brokenStore := NewApplier(fixtureExecutor(fixture), func(statepath.Paths) *receipt.Store {
		return receipt.NewStore(brokenRoot)
	})
	result, err = brokenStore.Apply(context.Background(), ApplyInput{Envelope: applyEnvelope("a"), Plan: applyPlan(planner.NetworkNone), Paths: paths})
	if err == nil || result.ReceiptPath != "" || !bytes.Equal(current, mustRead(t, currentPath)) || receiptCount(t, paths) != 1 {
		t.Fatalf("publication failure claimed path or changed current: result=%#v err=%v", result, err)
	}
	assertLockFree(t, paths)
}

func TestApplyClonesInputsAndMapsBlockedSafely(t *testing.T) {
	paths := applyPaths(t)
	input := ApplyInput{Envelope: applyEnvelope("b"), Plan: applyPlan(planner.NetworkRequired), Paths: paths}
	input.Envelope.Warnings = []string{"retain"}
	input.Plan.Steps[0].DependsOn = []string{}
	beforeDesired := append([]byte(nil), input.Plan.Steps[0].Desired...)
	callbackCalls, nilWithout, emptyDepends := 0, false, false
	callback := executorFunc(func(_ context.Context, plan planner.Plan, _ ...executor.Options) (executor.Result, error) {
		callbackCalls++
		nilWithout = plan.Selection.Without == nil
		emptyDepends = plan.Steps[0].DependsOn != nil && len(plan.Steps[0].DependsOn) == 0
		plan.Selection.Only[0] = "callback-mutated"
		plan.Steps[0].Desired[0] = 'x'
		step := plan.Steps[0]
		return executor.Result{Outcomes: []executor.Outcome{
			{Step: step, StepID: step.ID, Module: step.Module, Disposition: step.Disposition, Outcome: executor.OutcomeBlocked, ErrorCode: "blocked"},
			{Step: step, StepID: "apps.callback", Module: step.Module, Disposition: step.Disposition, Outcome: executor.OutcomeBlocked, ErrorCode: "callback-secret"},
		}}, nil
	})

	result, err := NewApplier(callback, receipt.NewStoreFromPaths).Apply(context.Background(), input)
	if err != nil || callbackCalls != 1 || !nilWithout || !emptyDepends ||
		!bytes.Equal(input.Plan.Steps[0].Desired, beforeDesired) || input.Plan.Selection.Only[0] != "apps" {
		t.Fatalf("input alias: result=%#v err=%v plan=%#v", result, err, input.Plan)
	}
	got := readApplyReceipt(t, result.ReceiptPath)
	if got.Status != "failed" || len(got.Steps) != 2 || got.Steps[0].ErrorCode != "blocked" ||
		got.Steps[1].ErrorCode != "executor-blocked" || strings.Contains(string(mustRead(t, result.ReceiptPath)), "callback-secret") {
		t.Fatalf("blocked mapping = %#v", got)
	}

	input.Envelope.Warnings[0] = "caller-mutated"
	input.Envelope.DesiredExactPins.NPM[0] = 'x'
	input.Plan.Steps[0].Desired[0] = 'y'
	if result.Receipt.Warnings[0] != "retain" || string(result.Receipt.DesiredExactPins.NPM) != "{}" ||
		!bytes.Equal(result.NetworkGroups[0].Steps[0].Desired, beforeDesired) || result.NetworkGroups[0].Steps[0].DependsOn == nil {
		t.Fatalf("retained result aliased caller: %#v", result)
	}
}
