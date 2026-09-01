package cachyos

import (
	"fmt"
	"strings"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

// EvidenceState is deny-only runtime evidence for an already authorized risky
// capability. Missing evidence is unknown, never ready.
type EvidenceState string

const (
	EvidenceReady   EvidenceState = "ready"
	EvidenceBlocked EvidenceState = "blocked"
	EvidenceUnknown EvidenceState = "unknown"
)

// CapabilityEvidence records whether an authorized factory has sufficient
// runtime evidence to construct its risky step.
type CapabilityEvidence struct {
	State  EvidenceState
	Detail string
}

// PlatformEvidence groups read-only observations consumed by CachyOS module
// factories. Capabilities never grant authority; the catalog policy does.
type PlatformEvidence struct {
	Capabilities    map[catalog.RiskCapability]CapabilityEvidence
	Bootstrap       BootstrapObservation
	Devtools        DevtoolsObservation
	Apps            AppsObservation
	AppsIconFetcher IconFetcher
	Vicinae         VicinaeObservation
	Desktop         DesktopObservation
}

// ObservationRequest is the canonical, immutable-by-copy list of authorized
// risky observations a caller may collect.
type ObservationRequest struct {
	Capabilities []catalog.RiskCapability
}

// ObservationRequestFor requests only host-authorized capabilities.
func ObservationRequestFor(policy catalog.ResolvedHostPolicy) ObservationRequest {
	request := ObservationRequest{}
	for _, capability := range catalog.RiskCapabilities() {
		if policy.Allows(capability) {
			request.Capabilities = append(request.Capabilities, capability)
		}
	}
	return request
}

// BuildModules constructs the CachyOS factories implemented by this slice.
// Authorization stays local to each factory and never enters planner types.
func BuildModules(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) ([]planner.Module, error) {
	if err := validateAssetOwnership(policy); err != nil {
		return nil, err
	}
	bootstrap, err := buildBootstrapModuleForPolicy(policy, evidence)
	if err != nil {
		return nil, err
	}
	fingerprint, err := buildFingerprintModule(policy, evidence)
	if err != nil {
		return nil, err
	}
	devtools := planner.Module{Name: DevtoolsModuleName, Enabled: moduleEnabled(policy, DevtoolsModuleName)}
	if devtools.Enabled {
		devtools, err = BuildDevtoolsModule(evidence.Devtools)
		if err != nil {
			return nil, err
		}
	}
	apps := planner.Module{Name: AppsModuleName, Enabled: moduleEnabled(policy, AppsModuleName)}
	if apps.Enabled {
		appsObservation := evidence.Apps
		// The merged policy is the sole pin authority at this boundary. Runtime
		// observations may describe local state, but cannot replace desired pins.
		appsObservation.AURPins = nil
		appsObservation.Catalog = &policy.Desired
		apps, err = BuildAppsModule(appsObservation, evidence.AppsIconFetcher)
		if err != nil {
			return nil, err
		}
	}
	vicinae := planner.Module{Name: VicinaeModuleName, Enabled: moduleEnabled(policy, VicinaeModuleName)}
	if vicinae.Enabled {
		vicinaeObservation := evidence.Vicinae
		vicinaeObservation.Catalog = &policy.Desired
		vicinae, err = BuildVicinaeModule(vicinaeObservation)
		if err != nil {
			return nil, err
		}
	}
	desktop, err := buildDesktopModule(policy, evidence)
	if err != nil {
		return nil, err
	}
	return []planner.Module{bootstrap, fingerprint, devtools, apps, vicinae, desktop}, nil
}

// BuildHostPlan runs the pure fake/runtime planning path for the implemented
// policy-aware factories.
func BuildHostPlan(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) (planner.Plan, error) {
	modules, err := BuildModules(policy, evidence)
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.BuildPlan(modules, planner.Selection{})
}

func capabilityEvidence(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence, capability catalog.RiskCapability) (CapabilityEvidence, bool, error) {
	if !policy.Allows(capability) {
		return CapabilityEvidence{}, false, nil
	}
	observed, exists := evidence.Capabilities[capability]
	if !exists || observed.State == "" {
		observed.State = EvidenceUnknown
	}
	switch observed.State {
	case EvidenceReady, EvidenceBlocked, EvidenceUnknown:
		return observed, true, nil
	default:
		return CapabilityEvidence{}, true, fmt.Errorf("invalid evidence state %q for capability %q", observed.State, capability)
	}
}

func policyStepID(capability catalog.RiskCapability) string {
	return "policy." + string(capability)
}

func blockedPolicyStep(module string, capability catalog.RiskCapability, evidence CapabilityEvidence) planner.Step {
	return planner.Step{
		ID:          policyStepID(capability),
		Module:      module,
		Description: "authorized capability awaiting ready evidence",
		Scope:       planner.ScopeSystem,
		Network:     planner.NetworkNone,
		Operation:   planner.Operation(policyStepID(capability)),
		Disposition: planner.DispositionBlocked,
		Desired:     mustJSON(map[string]any{"capability": capability, "authorized": true}),
		Observed:    mustJSON(map[string]any{"state": evidence.State, "detail": evidence.Detail}),
	}
}

func moduleEnabled(policy catalog.ResolvedHostPolicy, name string) bool {
	return policy.Desired.Modules != nil && policy.Desired.Modules[name]
}

func validateAssetOwnership(policy catalog.ResolvedHostPolicy) error {
	for _, name := range policy.Desired.Templates {
		if strings.HasPrefix(name, "templates/hosts/") {
			owner, remainder, found := strings.Cut(strings.TrimPrefix(name, "templates/hosts/"), "/")
			if owner == "" || owner != policy.Name {
				return fmt.Errorf("host %q cannot consume template owned by host %q: %s", policy.Name, owner, name)
			}
			capabilityName, asset, separated := strings.Cut(remainder, "/")
			capability := catalog.RiskCapability(capabilityName)
			if !found || !separated || asset == "" || !knownRiskCapability(capability) {
				return fmt.Errorf("host %q template lacks a known risk-capability owner: %s", policy.Name, name)
			}
			if !policy.Allows(capability) {
				return fmt.Errorf("host %q cannot consume template for denied capability %q: %s", policy.Name, capability, name)
			}
		}
		if strings.HasPrefix(name, "templates/roles/") {
			owner, asset, found := strings.Cut(strings.TrimPrefix(name, "templates/roles/"), "/")
			if !found || owner == "" || asset == "" || !declaresRole(policy, owner) {
				return fmt.Errorf("host %q cannot consume template owned by undeclared role %q: %s", policy.Name, owner, name)
			}
		}
		if strings.HasPrefix(name, "templates/niri/") || strings.HasPrefix(name, "templates/noctalia/") || strings.HasPrefix(name, "templates/hyprwhspr/") {
			return fmt.Errorf("host %q cannot consume legacy mixed-ownership template: %s", policy.Name, name)
		}
	}
	for _, name := range policy.Desired.Overlays {
		if !strings.HasPrefix(name, "overlays/") {
			continue
		}
		owner := strings.TrimPrefix(name, "overlays/")
		owner, _, _ = strings.Cut(owner, "/")
		if owner == "" || owner != policy.Name {
			return fmt.Errorf("host %q cannot consume overlay owned by host %q: %s", policy.Name, owner, name)
		}
	}
	return nil
}

func declaresRole(policy catalog.ResolvedHostPolicy, role string) bool {
	for _, declared := range policy.Roles {
		if declared == role {
			return true
		}
	}
	return false
}

func knownRiskCapability(capability catalog.RiskCapability) bool {
	for _, known := range catalog.RiskCapabilities() {
		if known == capability {
			return true
		}
	}
	return false
}
