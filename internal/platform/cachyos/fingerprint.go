package cachyos

import (
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

const fingerprintPAMStep = "fingerprint.pam"

func buildFingerprintModule(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) (planner.Module, error) {
	module := planner.Module{Name: "fingerprint", Enabled: moduleEnabled(policy, "fingerprint")}
	if !module.Enabled {
		return module, nil
	}
	observed, authorized, err := capabilityEvidence(policy, evidence, catalog.RiskFingerprintPAM)
	if err != nil || !authorized {
		return module, err
	}
	if observed.State != EvidenceReady {
		module.Steps = []planner.Step{blockedPolicyStep(module.Name, catalog.RiskFingerprintPAM, observed)}
		return module, nil
	}
	module.Steps = []planner.Step{{
		ID:          fingerprintPAMStep,
		Module:      module.Name,
		Description: "install the pinned fingerprint package and host-owned PAM overlays",
		Scope:       planner.ScopeSystem,
		Network:     planner.NetworkRequired,
		Operation:   planner.Operation(fingerprintPAMStep),
		Disposition: planner.DispositionApply,
		Desired: mustJSON(map[string]any{
			"elevation":        "pkexec",
			"packaging":        "packaging/libfprint-egismoc-sdcp-git",
			"hostOverlays":     append([]string(nil), policy.Desired.Overlays...),
			"biometricStorage": "unmanaged",
		}),
		Observed: mustJSON(map[string]any{"state": observed.State, "detail": observed.Detail}),
	}}
	return module, nil
}
