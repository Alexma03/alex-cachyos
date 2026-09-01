package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type EvidenceKind string

const (
	EvidenceFixture EvidenceKind = "fixture"
	EvidenceLive    EvidenceKind = "live"
)

var (
	ErrIntegrationTargetMismatch       = errors.New("integration target does not match resolved host")
	ErrLiveVerificationFailed          = errors.New("live verification failed")
	ErrInvalidLiveVerificationEvidence = errors.New("invalid live verification evidence")
)

var immutableReceiptIDPattern = regexp.MustCompile(`^(?:receipt|run)-[A-Za-z0-9][A-Za-z0-9._-]{0,95}$|^sha256:[0-9a-f]{64}$`)

var allowedEvidenceChecks = map[string]struct{}{
	"boot": {}, "failed-units": {}, "managed-files": {}, "niri": {},
	"noctalia": {}, "offline": {}, "packages": {}, "pacman-integrity": {},
	"pam": {}, "pacnew": {}, "services": {},
}

var allowedEvidenceStatuses = map[string]struct{}{"failed": {}, "passed": {}, "unknown": {}}

type IntegrationTargetMismatchError struct {
	Host   string
	Target string
}

func (e *IntegrationTargetMismatchError) Error() string {
	return fmt.Sprintf("%v: host %q, target %q", ErrIntegrationTargetMismatch, e.Host, e.Target)
}

func (e *IntegrationTargetMismatchError) Unwrap() error { return ErrIntegrationTargetMismatch }

type EvidenceCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type VerificationEvidence struct {
	Kind              EvidenceKind    `json:"kind"`
	Host              string          `json:"host"`
	IntegrationTarget string          `json:"integrationTarget,omitempty"`
	ReceiptID         string          `json:"receiptId,omitempty"`
	Checks            []EvidenceCheck `json:"checks"`
}

type LiveVerificationResult struct {
	ReceiptID string
	Checks    []EvidenceCheck
}

type LiveVerifier interface {
	Verify(context.Context, string) (LiveVerificationResult, error)
}

type VerificationRequest struct {
	Host              string
	IntegrationTarget string
	KnownHosts        []KnownHost
}

// ResolveVerification separates deterministic fixture evidence from explicitly
// authorized live verification. Both the selected host and integration target
// must be catalog-known before the live verifier port can be reached.
func ResolveVerification(ctx context.Context, request VerificationRequest, hostPort HostPort, live LiveVerifier) (VerificationEvidence, error) {
	host, err := ResolveHost(request.Host, request.KnownHosts, hostPort)
	if err != nil {
		return VerificationEvidence{}, err
	}
	if strings.TrimSpace(request.IntegrationTarget) == "" {
		return VerificationEvidence{Kind: EvidenceFixture, Host: host, Checks: []EvidenceCheck{}}, nil
	}
	target, err := ResolveHost(request.IntegrationTarget, request.KnownHosts, nil)
	if err != nil {
		return VerificationEvidence{}, err
	}
	if target != host {
		return VerificationEvidence{}, &IntegrationTargetMismatchError{Host: host, Target: target}
	}
	if live == nil {
		return VerificationEvidence{}, errors.New("live verifier is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := live.Verify(ctx, host)
	if err != nil {
		return VerificationEvidence{}, fmt.Errorf("%w for integration target %q", ErrLiveVerificationFailed, host)
	}
	if strings.TrimSpace(result.ReceiptID) == "" {
		return VerificationEvidence{}, ErrInvalidLiveVerificationEvidence
	}
	evidence := VerificationEvidence{Kind: EvidenceLive, Host: host, IntegrationTarget: target, ReceiptID: result.ReceiptID, Checks: cloneEvidenceChecks(result.Checks)}
	if err := validateLiveVerificationEvidence(evidence); err != nil {
		return VerificationEvidence{}, err
	}
	return evidence, nil
}

func RenderVerificationEvidence(evidence VerificationEvidence) ([]byte, error) {
	if evidence.Kind == EvidenceLive {
		if err := validateLiveVerificationEvidence(evidence); err != nil {
			return nil, err
		}
	}
	isolated := evidence
	isolated.Checks = cloneEvidenceChecks(evidence.Checks)
	sort.Slice(isolated.Checks, func(i, j int) bool {
		if isolated.Checks[i].Name == isolated.Checks[j].Name {
			return isolated.Checks[i].Status < isolated.Checks[j].Status
		}
		return isolated.Checks[i].Name < isolated.Checks[j].Name
	})
	return json.Marshal(isolated)
}

func validateLiveVerificationEvidence(evidence VerificationEvidence) error {
	if evidence.Kind != EvidenceLive || evidence.Host == "" || evidence.IntegrationTarget != evidence.Host || !immutableReceiptIDPattern.MatchString(evidence.ReceiptID) {
		return ErrInvalidLiveVerificationEvidence
	}
	if containsSensitiveMarker(evidence.ReceiptID) || len(evidence.Checks) == 0 || len(evidence.Checks) > 32 {
		return ErrInvalidLiveVerificationEvidence
	}
	seen := make(map[string]struct{}, len(evidence.Checks))
	for _, check := range evidence.Checks {
		if _, ok := allowedEvidenceChecks[check.Name]; !ok {
			return ErrInvalidLiveVerificationEvidence
		}
		if _, ok := allowedEvidenceStatuses[check.Status]; !ok {
			return ErrInvalidLiveVerificationEvidence
		}
		if _, duplicate := seen[check.Name]; duplicate {
			return ErrInvalidLiveVerificationEvidence
		}
		seen[check.Name] = struct{}{}
	}
	return nil
}

func containsSensitiveMarker(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{"authorization", "bearer", "credential", "password", "secret", "token"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func cloneEvidenceChecks(in []EvidenceCheck) []EvidenceCheck {
	if in == nil {
		return nil
	}
	return append([]EvidenceCheck(nil), in...)
}
