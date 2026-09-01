package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
)

type commandRepositorySpy struct {
	known        []string
	resolveCalls int
	resolved     catalog.ResolvedHostPolicy
}

func (s *commandRepositorySpy) KnownHosts() []string { return append([]string(nil), s.known...) }
func (s *commandRepositorySpy) Resolve(name string) (catalog.ResolvedHostPolicy, error) {
	s.resolveCalls++
	value := s.resolved
	value.Name = name
	return value, nil
}

type commandHostSpy struct{ calls int }

func (s *commandHostSpy) Hostname() (string, error) { s.calls++; return "portable", nil }

type commandOperationsSpy struct {
	calls    []CommandName
	requests []CommandRequest
	result   CommandResult
	err      error
}

func (s *commandOperationsSpy) Execute(_ context.Context, request CommandRequest, _ catalog.ResolvedHostPolicy) (CommandResult, error) {
	s.calls = append(s.calls, request.Command)
	s.requests = append(s.requests, request)
	return s.result, s.err
}

func TestCommandServiceResolvesHostBeforeOperationsAndPropagatesSelection(t *testing.T) {
	repository := &commandRepositorySpy{known: []string{"portable", "galaxy"}}
	host := &commandHostSpy{}
	operations := &commandOperationsSpy{result: CommandResult{Message: "planned"}}
	service := NewCommandService(repository, host, operations)
	request := CommandRequest{
		Command: CommandApply,
		Host:    "portable",
		Selection: planner.Selection{
			Only: []string{"bootstrap"}, With: []string{"apps"}, Without: []string{"fingerprint"},
		},
		Remove: []string{"desktop"}, DryRun: true,
	}

	got, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != CommandApply || got.Host != "portable" || got.Message != "planned" {
		t.Fatalf("result = %#v", got)
	}
	if repository.resolveCalls != 1 || host.calls != 0 || len(operations.requests) != 1 {
		t.Fatalf("calls: repository=%d host=%d operations=%d", repository.resolveCalls, host.calls, len(operations.requests))
	}
	called := operations.requests[0]
	if !reflect.DeepEqual(called.Selection, request.Selection) || !reflect.DeepEqual(called.Remove, request.Remove) || !called.DryRun || called.Host != "portable" {
		t.Fatalf("operation request = %#v, want %#v", called, request)
	}
	called.Selection.Only[0] = "mutated"
	called.Remove[0] = "mutated"
	if request.Selection.Only[0] != "bootstrap" || request.Remove[0] != "desktop" {
		t.Fatal("service request aliases caller slices")
	}
}

func TestCommandServiceRejectsUnknownHostBeforeRepositoryAndOperations(t *testing.T) {
	repository := &commandRepositorySpy{known: []string{"portable", "galaxy"}}
	host := &commandHostSpy{}
	operations := &commandOperationsSpy{}
	service := NewCommandService(repository, host, operations)

	_, err := service.Execute(context.Background(), CommandRequest{Command: CommandApply, Host: "missing"})
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) || !reflect.DeepEqual(unknown.Known, []string{"galaxy", "portable"}) {
		t.Fatalf("error = %#v, want typed sorted unknown host", err)
	}
	if repository.resolveCalls != 0 || host.calls != 0 || len(operations.calls) != 0 {
		t.Fatalf("unknown host escaped early gate: repository=%d host=%d operations=%d", repository.resolveCalls, host.calls, len(operations.calls))
	}
}

func TestCommandServiceUsesHostnameOnlyForHostScopedCommands(t *testing.T) {
	repository := &commandRepositorySpy{known: []string{"portable"}}
	host := &commandHostSpy{}
	operations := &commandOperationsSpy{}
	service := NewCommandService(repository, host, operations)

	if _, err := service.Execute(context.Background(), CommandRequest{Command: CommandCheck}); err != nil {
		t.Fatal(err)
	}
	if host.calls != 1 || repository.resolveCalls != 1 {
		t.Fatalf("check calls: host=%d repository=%d", host.calls, repository.resolveCalls)
	}
	if _, err := service.Execute(context.Background(), CommandRequest{Command: CommandReceipt}); err != nil {
		t.Fatal(err)
	}
	if host.calls != 1 || repository.resolveCalls != 1 {
		t.Fatalf("receipt unexpectedly resolved host: host=%d repository=%d", host.calls, repository.resolveCalls)
	}
}

func TestCommandServiceDoesNotRequireCatalogForReceiptOrStatus(t *testing.T) {
	operations := &commandOperationsSpy{result: CommandResult{Message: "current"}}
	service := NewCommandService(nil, nil, operations)
	for _, command := range []CommandName{CommandReceipt, CommandStatus} {
		got, err := service.Execute(context.Background(), CommandRequest{Command: command})
		if err != nil || got.Message != "current" {
			t.Fatalf("%s = %#v, %v", command, got, err)
		}
	}
}

func TestCommandResultCarriesCheckAndReceiptEvidence(t *testing.T) {
	check := CheckReport{Drift: true, ExitIntent: ExitIntentNonZero, Findings: []CheckFinding{{Kind: FindingManagedFile, Code: "managed-file-drift", Class: DriftPostApply, Path: "/etc/example", Backup: "/etc/example.bak.alex-cachyos", Severity: "error"}}}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "receipts", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := receipt.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	stored.RunID = "run-1"
	operations := &commandOperationsSpy{result: CommandResult{Check: &check, Receipt: &stored, ReceiptPath: "/state/receipts/run-1.json"}}
	service := NewCommandService(&commandRepositorySpy{known: []string{"portable"}}, &commandHostSpy{}, operations)

	got, err := service.Execute(context.Background(), CommandRequest{Command: CommandCheck, Host: "portable"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode() != 1 || got.Check == nil || got.Receipt == nil || got.Receipt.RunID != "run-1" {
		t.Fatalf("result = %#v", got)
	}
}

func TestCommandServiceRejectsInvalidReceiptResultsInsteadOfAliasingThem(t *testing.T) {
	invalid := receipt.Receipt{RunID: "not-a-valid-receipt"}
	operations := &commandOperationsSpy{result: CommandResult{Receipt: &invalid}}
	service := NewCommandService(&commandRepositorySpy{known: []string{"portable"}}, &commandHostSpy{}, operations)
	if _, err := service.Execute(context.Background(), CommandRequest{Command: CommandReceipt}); !errors.Is(err, receipt.ErrInvalid) {
		t.Fatalf("invalid receipt result error = %v, want %v", err, receipt.ErrInvalid)
	}
}

func TestCommandHandlersRouteEverySurfaceToItsTypedBoundary(t *testing.T) {
	tests := []struct {
		command CommandName
		build   func(CommandOperationsFunc) CommandHandlers
	}{
		{CommandApply, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Apply: handler} }},
		{CommandCheck, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Check: handler} }},
		{CommandAdopt, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Adopt: handler} }},
		{CommandRollback, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Rollback: handler} }},
		{CommandCheckpoint, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Checkpoint: handler} }},
		{CommandReceipt, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Receipt: handler} }},
		{CommandStatus, func(handler CommandOperationsFunc) CommandHandlers { return CommandHandlers{Status: handler} }},
	}
	for _, tc := range tests {
		t.Run(string(tc.command), func(t *testing.T) {
			calls := 0
			handlers := tc.build(func(_ context.Context, request CommandRequest, _ catalog.ResolvedHostPolicy) (CommandResult, error) {
				calls++
				if request.Command != tc.command {
					t.Fatalf("request command = %q, want %q", request.Command, tc.command)
				}
				return CommandResult{Message: "ok"}, nil
			})
			got, err := handlers.Execute(context.Background(), CommandRequest{Command: tc.command}, catalog.ResolvedHostPolicy{})
			if err != nil || calls != 1 || got.Message != "ok" {
				t.Fatalf("route = %#v, %v; calls=%d", got, err, calls)
			}
		})
	}
}

func TestCommandHandlersFailClosedWhenACommandBoundaryIsMissing(t *testing.T) {
	_, err := (CommandHandlers{}).Execute(context.Background(), CommandRequest{Command: CommandRollback}, catalog.ResolvedHostPolicy{})
	if !errors.Is(err, ErrCommandUnavailable) {
		t.Fatalf("missing rollback handler error = %v", err)
	}
}

func TestCheckCommandBuildsHostInventoryAndRunsOfflineChecker(t *testing.T) {
	factoryCalls, observerCalls := 0, 0
	factory := CheckRequestFactoryFunc(func(policy catalog.ResolvedHostPolicy, selection planner.Selection) (CheckRequest, error) {
		factoryCalls++
		if policy.Name != "portable" || !reflect.DeepEqual(selection.Only, []string{"verify"}) {
			t.Fatalf("factory input = policy %#v selection %#v", policy, selection)
		}
		return CheckRequest{ManagedFiles: []ManagedFileInventory{{Path: "/etc/example", DesiredHash: "desired", ReceiptAfterHash: "after", ReceiptBackup: "/etc/example.bak.alex-cachyos", Ownership: OwnershipCreated}}}, nil
	})
	observer := CheckObserverFunc(func(_ context.Context, request CheckRequest) (CheckSnapshot, error) {
		observerCalls++
		if len(request.ManagedFiles) != 1 || request.ManagedFiles[0].Path != "/etc/example" {
			t.Fatalf("observer request = %#v", request)
		}
		return CheckSnapshot{ManagedFiles: []ManagedFileObservation{{Path: "/etc/example", State: FileObserved, LiveHash: "changed"}}}, nil
	})
	handler := NewCheckCommand(factory, observer)
	got, err := handler.Execute(context.Background(), CommandRequest{Command: CommandCheck, Selection: planner.Selection{Only: []string{"verify"}}}, catalog.ResolvedHostPolicy{Name: "portable"})
	if err != nil {
		t.Fatal(err)
	}
	if factoryCalls != 1 || observerCalls != 1 || got.Check == nil || !got.Check.Drift || got.Check.Findings[0].Backup != "/etc/example.bak.alex-cachyos" {
		t.Fatalf("check result = %#v; factory=%d observer=%d", got, factoryCalls, observerCalls)
	}
}
