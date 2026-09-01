package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

type networkSpy struct{ calls []string }

func (s *networkSpy) Authorize(_ context.Context, request runner.CommandRequest) error {
	s.calls = append(s.calls, request.Operation)
	return nil
}

func commandRequest(operation string, network runner.NetworkPolicy) runner.CommandRequest {
	return runner.CommandRequest{Operation: operation, Executable: "/usr/bin/tool", Cwd: "/", Scope: runner.ScopeUser, Network: network, OutputPolicy: runner.OutputDiscard, Timeout: time.Second, OutputLimit: 1024}
}

func TestCommandMutatorExecutesTypedRequestAndAuthorizesNetwork(t *testing.T) {
	request := commandRequest("apps.fetch", runner.NetworkRequired)
	run := runner.NewFakeRunner(runner.Expectation{Operation: request.Operation})
	network := &networkSpy{}
	mutator, err := NewCommandMutator(run, []runner.CommandRequest{request}, network)
	if err != nil {
		t.Fatal(err)
	}
	step := planner.Step{ID: request.Operation, Module: "apps", Scope: planner.ScopeUser, Network: planner.NetworkRequired, Operation: planner.Operation("fetch"), Disposition: planner.DispositionApply, Desired: []byte(`{"value":"ready"}`)}
	if err := mutator.Mutate(context.Background(), step); err != nil {
		t.Fatal(err)
	}
	if len(network.calls) != 1 || network.calls[0] != request.Operation || len(run.Requests()) != 1 {
		t.Fatalf("network/runner calls = %#v / %#v", network.calls, run.Requests())
	}
}

func TestCommandMutatorDryRunThroughExecutorCallsNeitherNetworkNorRunner(t *testing.T) {
	request := commandRequest("apps.fetch", runner.NetworkRequired)
	run := runner.NewFakeRunner(runner.Expectation{Operation: request.Operation})
	network := &networkSpy{}
	mutator, err := NewCommandMutator(run, []runner.CommandRequest{request}, network)
	if err != nil {
		t.Fatal(err)
	}
	observer := ObserverFunc(func(context.Context, planner.Step) ([]byte, error) { return []byte(`{"value":"old"}`), nil })
	step := planner.Step{ID: request.Operation, Module: "apps", Scope: planner.ScopeUser, Network: planner.NetworkRequired, Operation: planner.Operation("fetch"), Disposition: planner.DispositionApply, Desired: []byte(`{"value":"ready"}`)}
	result, err := NewExecutor(observer, mutator).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step}}, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(network.calls) != 0 || len(run.Requests()) != 0 || len(result.Outcomes) != 1 || result.Outcomes[0].Outcome != OutcomeProjected {
		t.Fatalf("dry-run escaped isolation: network=%#v requests=%#v result=%#v", network.calls, run.Requests(), result)
	}
}

func TestCommandMutatorRejectsMissingNetworkPortAndMetadataMismatch(t *testing.T) {
	request := commandRequest("apps.fetch", runner.NetworkRequired)
	if _, err := NewCommandMutator(runner.NewFakeRunner(), []runner.CommandRequest{request}, nil); !errors.Is(err, ErrNetworkPortRequired) {
		t.Fatalf("missing network port error = %v", err)
	}
	request.Network = runner.NetworkNone
	mutator, err := NewCommandMutator(runner.NewFakeRunner(), []runner.CommandRequest{request}, nil)
	if err != nil {
		t.Fatal(err)
	}
	step := planner.Step{ID: request.Operation, Scope: planner.ScopeSystem, Network: planner.NetworkNone}
	if err := mutator.Mutate(context.Background(), step); !errors.Is(err, ErrCommandMetadataMismatch) {
		t.Fatalf("metadata mismatch error = %v", err)
	}
}
