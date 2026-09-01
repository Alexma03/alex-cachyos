package cachyos

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/runner"
)

const (
	desktopVerifyNiriOperation     = "verify.desktop.niri"
	desktopVerifyNoctaliaOperation = "verify.desktop.noctalia"
	desktopVerifyPackagesOperation = "verify.desktop.packages"
)

// DesktopLiveVerifier is a production-capable adapter for the allowlisted
// desktop checks. It carries no host authority; app.ResolveVerification must
// admit the exact host/target pair before Verify can be reached.
type DesktopLiveVerifier struct {
	runner    runner.Runner
	receiptID string
	requests  []runner.CommandRequest
}

func NewDesktopLiveVerifier(run runner.Runner, receiptID, homeRoot string, packages []string) (*DesktopLiveVerifier, error) {
	if run == nil || strings.TrimSpace(receiptID) == "" {
		return nil, errors.New("desktop live verifier requires runner and receipt identity")
	}
	requests, err := DesktopVerificationRequests(homeRoot, packages)
	if err != nil {
		return nil, err
	}
	return &DesktopLiveVerifier{runner: run, receiptID: receiptID, requests: requests}, nil
}

// DesktopVerificationRequests returns the three closed, argv-only checks in
// canonical evidence order. It does not execute them.
func DesktopVerificationRequests(homeRoot string, packages []string) ([]runner.CommandRequest, error) {
	if !canonicalDesktopRoot(homeRoot) {
		return nil, errors.New("desktop verification home must be canonical and absolute")
	}
	names := append([]string(nil), packages...)
	sort.Strings(names)
	if !canonicalDesktopPackages(names) {
		return nil, errors.New("desktop verification package inventory is invalid")
	}
	definitions := []struct {
		operation, executable string
		argv                  []string
	}{
		{desktopVerifyNiriOperation, "/usr/bin/niri", []string{"validate", "--config", filepath.Join(homeRoot, ".config", "niri", "config.kdl")}},
		{desktopVerifyNoctaliaOperation, "/usr/bin/noctalia", []string{"config", "validate", filepath.Join(homeRoot, ".local", "state", "noctalia", "settings.toml")}},
		{desktopVerifyPackagesOperation, "/usr/bin/pacman", append([]string{"-Q"}, names...)},
	}
	requests := make([]runner.CommandRequest, 0, len(definitions))
	for _, definition := range definitions {
		request, err := makeRequest(definition.operation, definition.executable, definition.argv, runner.ScopeUser, runner.NetworkNone, nil)
		if err != nil {
			return nil, err
		}
		request.OutputPolicy = runner.OutputDiscard
		requests = append(requests, request)
	}
	return requests, nil
}

func (verifier *DesktopLiveVerifier) Verify(ctx context.Context, host string) (app.LiveVerificationResult, error) {
	if verifier == nil || verifier.runner == nil || strings.TrimSpace(host) == "" {
		return app.LiveVerificationResult{}, errors.New("desktop live verifier unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checks := make([]app.EvidenceCheck, 0, len(verifier.requests))
	for _, request := range verifier.requests {
		status := "passed"
		result, err := verifier.runner.Run(ctx, request)
		if err != nil || result.ExitCode != 0 {
			status = "failed"
		}
		name, ok := desktopEvidenceCheck(request.Operation)
		if !ok {
			return app.LiveVerificationResult{}, fmt.Errorf("desktop verifier contains an unknown operation")
		}
		checks = append(checks, app.EvidenceCheck{Name: name, Status: status})
	}
	return app.LiveVerificationResult{ReceiptID: verifier.receiptID, Checks: checks}, nil
}

func desktopEvidenceCheck(operation string) (string, bool) {
	switch operation {
	case desktopVerifyNiriOperation:
		return "niri", true
	case desktopVerifyNoctaliaOperation:
		return "noctalia", true
	case desktopVerifyPackagesOperation:
		return "packages", true
	default:
		return "", false
	}
}

var _ app.LiveVerifier = (*DesktopLiveVerifier)(nil)
