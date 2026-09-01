package cachyos

import (
	"fmt"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

const (
	desktopWorkstationStep       = "desktop.workstation"
	desktopFixedDisplaysStep     = "desktop.fixed-displays"
	desktopFixedInputDevicesStep = "desktop.fixed-input-devices"
	desktopLiteralHomePathsStep  = "desktop.literal-home-paths"
	desktopCosmicPruneStep       = "desktop.cosmic-prune"
)

var workstationAssets = []string{
	"templates/roles/workstation/niri/config.kdl",
	"templates/roles/workstation/noctalia/settings.toml",
	"templates/roles/workstation/hyprwhspr/config.json",
}

func buildDesktopModule(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) (planner.Module, error) {
	module := planner.Module{Name: "desktop", Enabled: moduleEnabled(policy, "desktop")}
	if !module.Enabled {
		return module, nil
	}
	if !declaresRole(policy, "workstation") {
		for _, capability := range []catalog.RiskCapability{
			catalog.RiskFixedDisplays,
			catalog.RiskFixedInputDevices,
			catalog.RiskLiteralHomePaths,
			catalog.RiskCosmicPrune,
		} {
			if policy.Allows(capability) {
				return planner.Module{}, fmt.Errorf("host %q authorizes %q without the workstation role", policy.Name, capability)
			}
		}
		return module, nil
	}
	roleAssets := ownedRoleTemplates(policy, "workstation")
	if len(roleAssets) == 0 {
		return planner.Module{}, fmt.Errorf("host %q declares workstation role without owned role templates", policy.Name)
	}
	module.Steps = append(module.Steps, planner.Step{
		ID:          desktopWorkstationStep,
		Module:      module.Name,
		Description: "configure the hardware-independent CachyOS niri and Noctalia workstation",
		Scope:       planner.ScopeSystem,
		Network:     planner.NetworkRequired,
		Operation:   planner.Operation(desktopWorkstationStep),
		Disposition: planner.DispositionApply,
		Desired: mustJSON(map[string]any{
			"assets":   roleAssets,
			"packages": []string{"niri", "noctalia", "noctalia-greeter", "greetd", "accountsservice"},
			"session":  "niri",
			"shell":    "noctalia",
		}),
		Observed: mustJSON(map[string]any{"state": "fixture"}),
	})

	for _, gated := range []struct {
		capability catalog.RiskCapability
		stepID     string
	}{
		{catalog.RiskFixedDisplays, desktopFixedDisplaysStep},
		{catalog.RiskFixedInputDevices, desktopFixedInputDevicesStep},
		{catalog.RiskLiteralHomePaths, desktopLiteralHomePathsStep},
	} {
		observed, authorized, err := capabilityEvidence(policy, evidence, gated.capability)
		if err != nil {
			return planner.Module{}, err
		}
		if !authorized {
			continue
		}
		if observed.State != EvidenceReady {
			module.Steps = append(module.Steps, blockedPolicyStep(module.Name, gated.capability, observed))
			continue
		}
		assets := ownedHostTemplates(policy, gated.capability)
		if len(assets) == 0 {
			return planner.Module{}, fmt.Errorf("host %q authorizes %q without an owned host template", policy.Name, gated.capability)
		}
		module.Steps = append(module.Steps, planner.Step{
			ID:          gated.stepID,
			Module:      module.Name,
			DependsOn:   []string{desktopWorkstationStep},
			Description: "apply explicitly authorized host-owned desktop values",
			Scope:       planner.ScopeUser,
			Network:     planner.NetworkNone,
			Operation:   planner.Operation(gated.stepID),
			Disposition: planner.DispositionApply,
			Desired:     mustJSON(map[string]any{"capability": gated.capability, "assets": assets}),
			Observed:    mustJSON(map[string]any{"state": observed.State, "detail": observed.Detail}),
		})
	}

	observed, authorized, err := capabilityEvidence(policy, evidence, catalog.RiskCosmicPrune)
	if err != nil {
		return planner.Module{}, err
	}
	if authorized {
		if observed.State != EvidenceReady {
			module.Steps = append(module.Steps, blockedPolicyStep(module.Name, catalog.RiskCosmicPrune, observed))
		} else {
			module.Steps = append(module.Steps, planner.Step{
				ID:          desktopCosmicPruneStep,
				Module:      module.Name,
				DependsOn:   []string{desktopWorkstationStep},
				Description: "prune the legacy Cosmic stack after the workstation target exists",
				Scope:       planner.ScopeSystem,
				Network:     planner.NetworkNone,
				Operation:   planner.Operation(desktopCosmicPruneStep),
				Disposition: planner.DispositionRemove,
				Desired: mustJSON(map[string]any{
					"prerequisites":  []string{"niri", "noctalia", "noctalia-greeter", "greetd", "accountsservice"},
					"classification": "externally-managed",
				}),
				Observed: mustJSON(map[string]any{"state": observed.State, "detail": observed.Detail}),
			})
		}
	}
	return module, nil
}

func ownedHostTemplates(policy catalog.ResolvedHostPolicy, capability catalog.RiskCapability) []string {
	prefix := "templates/hosts/" + policy.Name + "/" + string(capability) + "/"
	var assets []string
	for _, name := range policy.Desired.Templates {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			assets = append(assets, name)
		}
	}
	return assets
}

func ownedRoleTemplates(policy catalog.ResolvedHostPolicy, role string) []string {
	if !declaresRole(policy, role) {
		return nil
	}
	prefix := "templates/roles/" + role + "/"
	var assets []string
	for _, name := range policy.Desired.Templates {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			assets = append(assets, name)
		}
	}
	return assets
}
