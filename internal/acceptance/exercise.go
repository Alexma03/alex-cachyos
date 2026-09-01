package acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/executor"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/statepath"
)

// FixtureExecution can only carry evidence produced by ExerciseFixturePlan.
// Its fields are deliberately private so report callers cannot claim constant
// convergence or dry-run results.
type FixtureExecution struct {
	dryRun      DryRunEvidence
	convergence ConvergenceEvidence
	verified    bool
}

func (e FixtureExecution) Evidence() (DryRunEvidence, ConvergenceEvidence) {
	return e.dryRun, e.convergence
}

type fixtureApplication struct {
	values           map[string]json.RawMessage
	mutations        int
	networkMutations int
}

func newFixtureApplication() *fixtureApplication {
	return &fixtureApplication{values: map[string]json.RawMessage{}}
}

func (f *fixtureApplication) observe(_ context.Context, step planner.Step) ([]byte, error) {
	if value, ok := f.values[step.ID]; ok {
		return append([]byte(nil), value...), nil
	}
	return []byte(`{"fixtureState":"unconverged"}`), nil
}

func (f *fixtureApplication) mutate(_ context.Context, step planner.Step) error {
	f.mutations++
	if step.Network == planner.NetworkRequired {
		f.networkMutations++
	}
	f.values[step.ID] = append(json.RawMessage(nil), step.Desired...)
	return nil
}

// ExerciseFixturePlan runs the resolved plan through the real executor and app
// orchestration with in-memory observer/mutator ports and isolated state paths.
func ExerciseFixturePlan(ctx context.Context, plan planner.Plan, paths statepath.Paths, host string) (FixtureExecution, error) {
	fixture := newFixtureApplication()
	run := executor.NewExecutor(executor.ObserverFunc(fixture.observe), executor.MutatorFunc(fixture.mutate))
	applier := app.NewApplier(run, receipt.NewStoreFromPaths)

	before := fixture.digest()
	held, err := app.AcquireLock(paths)
	if err != nil {
		return FixtureExecution{}, fmt.Errorf("hold fixture lock for dry-run proof: %w", err)
	}
	dryResult, dryErr := applier.Apply(ctx, app.ApplyInput{Envelope: fixtureEnvelope(host, "dry", 1), Plan: plan, Paths: paths, DryRun: true})
	releaseErr := held.Release()
	if dryErr != nil {
		return FixtureExecution{}, fmt.Errorf("fixture dry-run: %w", dryErr)
	}
	if releaseErr != nil {
		return FixtureExecution{}, fmt.Errorf("release fixture proof lock: %w", releaseErr)
	}
	if dryResult.ReceiptPath == "" {
		return FixtureExecution{}, fmt.Errorf("fixture dry-run audit receipt was not published")
	}
	if _, err := os.Stat(dryResult.ReceiptPath); err != nil {
		return FixtureExecution{}, fmt.Errorf("inspect fixture dry-run receipt: %w", err)
	}
	dryEvidence := DryRunEvidence{
		MutationCount: fixture.mutations, NetworkMutationCount: fixture.networkMutations,
		LockAcquisitions: 0, StateUnchanged: before == fixture.digest(), AuditReceiptPublished: true,
	}
	if dryEvidence.MutationCount != 0 || dryEvidence.NetworkMutationCount != 0 || !dryEvidence.StateUnchanged {
		return FixtureExecution{}, fmt.Errorf("fixture dry-run mutated application state")
	}

	first, err := applier.Apply(ctx, app.ApplyInput{Envelope: fixtureEnvelope(host, "first", 2), Plan: plan, Paths: paths})
	if err != nil {
		return FixtureExecution{}, fmt.Errorf("fixture first apply: %w", err)
	}
	firstMutations := fixture.mutations
	stateAfterFirst := fixture.digest()
	secondBefore := fixture.mutations
	second, err := applier.Apply(ctx, app.ApplyInput{Envelope: fixtureEnvelope(host, "second", 3), Plan: plan, Paths: paths})
	if err != nil {
		return FixtureExecution{}, fmt.Errorf("fixture second apply: %w", err)
	}
	convergence := ConvergenceEvidence{
		FirstNoChange: first.Receipt.NoChange, SecondNoChange: second.Receipt.NoChange,
		MutationCount: firstMutations, SecondMutationCount: fixture.mutations - secondBefore,
	}
	if convergence.FirstNoChange || !convergence.SecondNoChange || convergence.MutationCount == 0 || convergence.SecondMutationCount != 0 || stateAfterFirst != fixture.digest() {
		return FixtureExecution{}, fmt.Errorf("fixture application did not converge")
	}
	return FixtureExecution{dryRun: dryEvidence, convergence: convergence, verified: true}, nil
}

func (f *fixtureApplication) digest() string {
	names := make([]string, 0, len(f.values))
	for name := range f.values {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(bytes.TrimSpace(f.values[name]))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func fixtureEnvelope(host, suffix string, second int) receipt.Receipt {
	empty := func() json.RawMessage { return json.RawMessage(`{}`) }
	return receipt.Receipt{
		Schema: receipt.SchemaV1, RunID: "run-fixture-" + suffix, Command: "apply",
		StartedAt: fmt.Sprintf("2026-01-02T03:04:%02dZ", second), FinishedAt: fmt.Sprintf("2026-01-02T03:05:%02dZ", second), Status: "pending",
		Host:      receipt.Host{Requested: host, Resolved: host, Hostname: host},
		Catalog:   receipt.Catalog{CatalogVersion: "1", Release: "fixture", Digest: "fixture", Source: "synthetic"},
		Selection: receipt.Selection{Only: []string{}, With: []string{}, Without: []string{}, Remove: []string{}},
		Plan:      receipt.Plan{Digest: "prebuilt", NetworkRequired: []string{}}, Steps: []receipt.Step{}, ManagedFiles: []receipt.ManagedFile{}, Mutations: []receipt.Mutation{},
		DesiredPackages:           receipt.DesiredPackages{PacmanNames: []string{}},
		DesiredExactPins:          receipt.DesiredExactPins{NPM: empty(), SourceCheckouts: empty(), AURLocalSources: empty(), Patches: empty(), RemoteArtifacts: empty(), OptionalPacmanArtifacts: empty()},
		ResolvedInstalledVersions: receipt.ResolvedInstalledVersions{Pacman: empty(), AUR: empty(), NPM: empty()},
		SystemTransactions:        []receipt.SystemTransaction{}, Checkouts: []receipt.Checkout{}, GentleAIInvocations: []receipt.GentleAIInvocation{},
		ManagedAssets: receipt.ManagedAssets{VerifiedEntries: []json.RawMessage{}}, Credentials: receipt.Credentials{ReferencedNames: []string{}},
		Warnings: []string{}, Errors: []string{},
	}
}
