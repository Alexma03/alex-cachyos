package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type integrationHostPort struct{ calls int }

func (p *integrationHostPort) Hostname() (string, error) {
	p.calls++
	return "galaxy", nil
}

type recordingLiveVerifier struct {
	calls  int
	result LiveVerificationResult
	err    error
}

func (v *recordingLiveVerifier) Verify(_ context.Context, host string) (LiveVerificationResult, error) {
	v.calls++
	return v.result, v.err
}

func TestResolveVerificationRedactsLiveVerifierErrors(t *testing.T) {
	verifier := &recordingLiveVerifier{err: errors.New("fixture-secret-value raw command output")}
	_, err := ResolveVerification(context.Background(), VerificationRequest{Host: "galaxy", IntegrationTarget: "galaxy", KnownHosts: []KnownHost{{Name: "galaxy"}}}, &integrationHostPort{}, verifier)
	if err == nil || strings.Contains(err.Error(), "fixture-secret-value") || strings.Contains(err.Error(), "raw command output") {
		t.Fatalf("live verifier error was not redacted: %v", err)
	}
}

func TestResolveVerificationRejectsSensitiveSuccessfulResult(t *testing.T) {
	tests := []struct {
		name   string
		result LiveVerificationResult
	}{
		{"receipt identity", LiveVerificationResult{ReceiptID: "receipt-secret=fixture-secret-value", Checks: []EvidenceCheck{{Name: "offline", Status: "passed"}}}},
		{"check name", LiveVerificationResult{ReceiptID: "receipt-safe", Checks: []EvidenceCheck{{Name: "raw command output: fixture-secret-value", Status: "passed"}}}},
		{"check status", LiveVerificationResult{ReceiptID: "receipt-safe", Checks: []EvidenceCheck{{Name: "offline", Status: "fixture-secret-value"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &recordingLiveVerifier{result: test.result}
			evidence, err := ResolveVerification(context.Background(), VerificationRequest{Host: "galaxy", IntegrationTarget: "galaxy", KnownHosts: []KnownHost{{Name: "galaxy"}}}, &integrationHostPort{}, verifier)
			if !errors.Is(err, ErrInvalidLiveVerificationEvidence) {
				t.Fatalf("error = %v, want invalid live evidence", err)
			}
			if strings.Contains(err.Error(), "fixture-secret-value") || strings.Contains(err.Error(), "raw command output") {
				t.Fatalf("successful verifier data leaked through rejection: %v", err)
			}
			if evidence.Kind != "" || evidence.Host != "" || evidence.ReceiptID != "" || len(evidence.Checks) != 0 {
				t.Fatalf("rejected evidence = %#v", evidence)
			}
		})
	}
}

func TestResolveVerificationTargetGate(t *testing.T) {
	known := []KnownHost{{Name: "galaxy"}, {Name: "portable-synthetic"}}
	tests := []struct {
		name       string
		host       string
		target     string
		wantKind   EvidenceKind
		wantErr    error
		wantPort   int
		wantVerify int
	}{
		{"absent target is fixture", "portable-synthetic", "", EvidenceFixture, nil, 0, 0},
		{"mismatch blocks live verifier", "portable-synthetic", "galaxy", "", ErrIntegrationTargetMismatch, 0, 0},
		{"matching target permits live verifier", "galaxy", "galaxy", EvidenceLive, nil, 0, 1},
		{"unknown host blocks before ports", "unknown", "unknown", "", ErrUnknownHost, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			port := &integrationHostPort{}
			verifier := &recordingLiveVerifier{result: LiveVerificationResult{ReceiptID: "receipt-fixture", Checks: []EvidenceCheck{{Name: "offline", Status: "passed"}}}}
			evidence, err := ResolveVerification(context.Background(), VerificationRequest{Host: test.host, IntegrationTarget: test.target, KnownHosts: known}, port, verifier)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if evidence.Kind != test.wantKind || port.calls != test.wantPort || verifier.calls != test.wantVerify {
				t.Fatalf("evidence/calls = %#v/%d/%d", evidence, port.calls, verifier.calls)
			}
		})
	}
}

func TestVerificationEvidenceCanonicalAndSecretFree(t *testing.T) {
	evidence := VerificationEvidence{Kind: EvidenceLive, Host: "galaxy", IntegrationTarget: "galaxy", ReceiptID: "receipt-1", Checks: []EvidenceCheck{{Name: "services", Status: "passed"}, {Name: "offline", Status: "passed"}}}
	first, err := RenderVerificationEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderVerificationEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("evidence rendering is unstable")
	}
	if strings.Index(string(first), `"name":"offline"`) > strings.Index(string(first), `"name":"services"`) {
		t.Fatalf("checks not canonical: %s", first)
	}
	if strings.Contains(string(first), "super-secret") {
		t.Fatal("secret leaked")
	}
	var decoded VerificationEvidence
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatal(err)
	}
}
