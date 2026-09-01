package cachyos

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/runner"
)

func TestDesktopLiveVerifierRunsOnlyBehindExactIntegrationTarget(t *testing.T) {
	home := filepath.Join("/fixture", "home")
	packages := []string{"niri", "noctalia"}
	known := []app.KnownHost{{Name: "galaxy"}, {Name: "portable-fixture"}}

	mismatchRunner := runner.NewFakeRunner()
	mismatchVerifier, err := NewDesktopLiveVerifier(mismatchRunner, "receipt-fixture", home, packages)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ResolveVerification(context.Background(), app.VerificationRequest{
		Host: "portable-fixture", IntegrationTarget: "galaxy", KnownHosts: known,
	}, nil, mismatchVerifier)
	if !errors.Is(err, app.ErrIntegrationTargetMismatch) || len(mismatchRunner.Requests()) != 0 {
		t.Fatalf("mismatch = %v, requests=%#v", err, mismatchRunner.Requests())
	}

	unknownRunner := runner.NewFakeRunner()
	unknownVerifier, err := NewDesktopLiveVerifier(unknownRunner, "receipt-fixture", home, packages)
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.ResolveVerification(context.Background(), app.VerificationRequest{
		Host: "generic-vm", IntegrationTarget: "generic-vm", KnownHosts: known,
	}, nil, unknownVerifier)
	if !errors.Is(err, app.ErrUnknownHost) || len(unknownRunner.Requests()) != 0 {
		t.Fatalf("unknown host = %v, requests=%#v", err, unknownRunner.Requests())
	}

	wantRequests, err := DesktopVerificationRequests(home, packages)
	if err != nil {
		t.Fatal(err)
	}
	expectations := make([]runner.Expectation, len(wantRequests))
	for i, request := range wantRequests {
		expectations[i] = runner.Expectation{Operation: request.Operation, Argv: request.Argv, Result: runner.CommandResult{ExitCode: 0}}
	}
	liveRunner := runner.NewFakeRunner(expectations...)
	liveVerifier, err := NewDesktopLiveVerifier(liveRunner, "receipt-fixture", home, packages)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := app.ResolveVerification(context.Background(), app.VerificationRequest{
		Host: "portable-fixture", IntegrationTarget: "portable-fixture", KnownHosts: known,
	}, nil, liveVerifier)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Kind != app.EvidenceLive || evidence.Host != "portable-fixture" || evidence.ReceiptID != "receipt-fixture" {
		t.Fatalf("evidence = %#v", evidence)
	}
	wantChecks := []app.EvidenceCheck{{Name: "niri", Status: "passed"}, {Name: "noctalia", Status: "passed"}, {Name: "packages", Status: "passed"}}
	if !reflect.DeepEqual(evidence.Checks, wantChecks) {
		t.Fatalf("checks = %#v, want %#v", evidence.Checks, wantChecks)
	}
	if err := liveRunner.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopLiveVerifierReportsAllowlistedFailureWithoutOutput(t *testing.T) {
	requests, err := DesktopVerificationRequests("/fixture/home", []string{"niri"})
	if err != nil {
		t.Fatal(err)
	}
	expectations := make([]runner.Expectation, len(requests))
	for i, request := range requests {
		expectations[i] = runner.Expectation{Operation: request.Operation, Argv: request.Argv, Result: runner.CommandResult{ExitCode: 1, Stderr: []byte("private-output")}, Err: errors.New("validator failed with private-output")}
	}
	run := runner.NewFakeRunner(expectations...)
	verifier, err := NewDesktopLiveVerifier(run, "receipt-fixture", "/fixture/home", []string{"niri"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := verifier.Verify(context.Background(), "portable-fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range result.Checks {
		if check.Status != "failed" || check.Name == "private-output" {
			t.Fatalf("check = %#v", check)
		}
	}
}

func TestDesktopVerificationRequestsValidateTheResolvedRoleFiles(t *testing.T) {
	home := "/fixture/home"
	requests, err := DesktopVerificationRequests(home, []string{"noctalia", "niri"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		desktopVerifyNiriOperation:     {"validate", "--config", filepath.Join(home, ".config", "niri", "config.kdl")},
		desktopVerifyNoctaliaOperation: {"config", "validate", filepath.Join(home, ".local", "state", "noctalia", "settings.toml")},
		desktopVerifyPackagesOperation: {"-Q", "niri", "noctalia"},
	}
	for _, request := range requests {
		if !reflect.DeepEqual(request.Argv, want[request.Operation]) || request.Network != runner.NetworkNone || request.OutputPolicy != runner.OutputDiscard {
			t.Fatalf("request = %#v", request)
		}
	}
}
