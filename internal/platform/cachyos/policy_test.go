package cachyos

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

func TestBuildModulesWiresEnabledReproducibleFactoriesFromResolvedPolicy(t *testing.T) {
	policyPins := testAppsPins()
	policyPins[VicinaePackageName] = catalog.AURLocalPin{
		SourceCommit: testVicinaeCommit,
		PatchSHA256:  testVicinaePatchSHA,
	}
	policy := galaxyPolicy("", "", false)
	policy.Desired.Modules = catalog.ModuleSet{
		"bootstrap":   true,
		"fingerprint": true,
		"devtools":    true,
		"apps":        true,
		"vicinae":     true,
		"desktop":     true,
	}
	policy.Desired.Pins = &catalog.Pins{AURLocal: policyPins}

	evidence := readyEvidence()
	evidence.Devtools = DevtoolsObservation{HomeRoot: t.TempDir()}
	evidence.Apps = AppsObservation{
		HomeRoot: t.TempDir(),
		UserName: "alex",
		AURPins: map[string]catalog.AURLocalPin{
			"warp-terminal-bin": {SourceCommit: strings.Repeat("0", 40), PatchSHA256: strings.Repeat("f", 64)},
		},
	}
	evidence.AppsIconFetcher = embeddedAppsIconFetcher(t)
	evidence.Vicinae = newVicinaeObservation(t, []byte(testVicinaeStock))
	evidence.Vicinae.Catalog = &catalog.Catalog{Pins: &catalog.Pins{AURLocal: map[string]catalog.AURLocalPin{
		VicinaePackageName: {SourceCommit: strings.Repeat("0", 40), PatchSHA256: strings.Repeat("f", 64)},
	}}}

	modules, err := BuildModules(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"bootstrap", "fingerprint", "devtools", "apps", "vicinae", "desktop"}
	gotNames := make([]string, len(modules))
	for i, module := range modules {
		gotNames[i] = module.Name
		if !module.Enabled {
			t.Errorf("module %q is disabled", module.Name)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("module order = %#v, want %#v", gotNames, wantNames)
	}
	for _, check := range []struct {
		module string
		step   string
	}{
		{DevtoolsModuleName, "devtools.mise.install"},
		{AppsModuleName, "apps.aur.warp-terminal-bin.checkout"},
		{VicinaeModuleName, "vicinae.package.checkout"},
	} {
		if !hasStep(moduleNamed(t, modules, check.module), check.step) {
			t.Fatalf("module %q lacks reproducible factory step %q", check.module, check.step)
		}
	}
	assertStepSourceCommit(t, moduleNamed(t, modules, AppsModuleName), "apps.aur.warp-terminal-bin.checkout", policyPins["warp-terminal-bin"].SourceCommit)
	assertStepSourceCommit(t, moduleNamed(t, modules, VicinaeModuleName), "vicinae.package.checkout", policyPins[VicinaePackageName].SourceCommit)
	assertStepPatchSHA256(t, moduleNamed(t, modules, AppsModuleName), "apps.aur.warp-terminal-bin.patch.verify", policyPins["warp-terminal-bin"].PatchSHA256)
	assertStepPatchSHA256(t, moduleNamed(t, modules, VicinaeModuleName), "vicinae.package.patch.verify", policyPins[VicinaePackageName].PatchSHA256)

	plan, err := planner.BuildPlan(modules, planner.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	lastModuleIndex := map[string]int{}
	for index, step := range plan.Steps {
		lastModuleIndex[step.Module] = index
	}
	if !(lastModuleIndex[DevtoolsModuleName] < lastModuleIndex[AppsModuleName] &&
		lastModuleIndex[AppsModuleName] < lastModuleIndex[VicinaeModuleName] &&
		lastModuleIndex[VicinaeModuleName] < lastModuleIndex["desktop"]) {
		t.Fatalf("combined module order = %#v", lastModuleIndex)
	}
}

func TestBuildModulesKeepsDisabledFactoriesInert(t *testing.T) {
	policy := galaxyPolicy("desktop", "", false)
	called := false
	evidence := readyEvidence()
	evidence.AppsIconFetcher = IconFetcherFunc(func(context.Context, IconFetchRequest) ([]byte, error) {
		called = true
		return nil, nil
	})

	modules, err := BuildModules(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{DevtoolsModuleName, AppsModuleName, VicinaeModuleName} {
		module := moduleNamed(t, modules, name)
		if module.Enabled || len(module.Steps) != 0 {
			t.Fatalf("disabled module %q was constructed: %#v", name, module)
		}
	}
	if called {
		t.Fatal("disabled apps module crossed the icon-fetch network seam")
	}
}

func assertStepSourceCommit(t *testing.T, module planner.Module, stepID, want string) {
	t.Helper()
	step := stepByID(module, stepID)
	if step == nil {
		t.Fatalf("missing step %q", stepID)
	}
	var desired struct {
		SourceCommit string `json:"sourceCommit"`
	}
	if err := json.Unmarshal(step.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if desired.SourceCommit != want {
		t.Fatalf("step %q source commit = %q, want resolved policy pin %q", stepID, desired.SourceCommit, want)
	}
}

func assertStepPatchSHA256(t *testing.T, module planner.Module, stepID, want string) {
	t.Helper()
	step := stepByID(module, stepID)
	if step == nil {
		t.Fatalf("missing step %q", stepID)
	}
	var desired struct {
		PatchSHA256 string `json:"patchSHA256"`
	}
	if err := json.Unmarshal(step.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if desired.PatchSHA256 != want {
		t.Fatalf("step %q patch SHA-256 = %q, want resolved policy pin %q", stepID, desired.PatchSHA256, want)
	}
}

func TestCapabilityPolicyMatrix(t *testing.T) {
	tests := []struct {
		name       string
		capability catalog.RiskCapability
		module     string
		readyStep  string
	}{
		{"fingerprint PAM", catalog.RiskFingerprintPAM, "fingerprint", fingerprintPAMStep},
		{"fixed displays", catalog.RiskFixedDisplays, "desktop", desktopFixedDisplaysStep},
		{"fixed input devices", catalog.RiskFixedInputDevices, "desktop", desktopFixedInputDevicesStep},
		{"literal home paths", catalog.RiskLiteralHomePaths, "desktop", desktopLiteralHomePathsStep},
		{"bootstrap system update", catalog.RiskBootstrapSystemUpdate, "bootstrap", bootstrapPackageInstall},
		{"bootstrap package removal", catalog.RiskBootstrapPackageRemoval, "bootstrap", bootstrapPackageRemove},
		{"bootstrap boot mutation", catalog.RiskBootstrapBootMutation, "bootstrap", bootstrapBootPlymouthEdit},
		{"Cosmic prune", catalog.RiskCosmicPrune, "desktop", desktopCosmicPruneStep},
	}
	for _, test := range tests {
		for _, state := range []struct {
			name    string
			allowed bool
			status  EvidenceState
		}{
			{"omitted despite observation", false, EvidenceReady},
			{"allowed and ready", true, EvidenceReady},
			{"allowed but blocked", true, EvidenceBlocked},
			{"allowed but unknown", true, EvidenceUnknown},
		} {
			t.Run(test.name+"/"+state.name, func(t *testing.T) {
				policy := galaxyPolicy(test.module, test.capability, state.allowed)
				evidence := readyEvidence()
				evidence.Capabilities[test.capability] = CapabilityEvidence{State: state.status}

				request := ObservationRequestFor(policy)
				if got := slices.Contains(request.Capabilities, test.capability); got != state.allowed {
					t.Fatalf("request contains %q = %v, want %v", test.capability, got, state.allowed)
				}
				modules, err := BuildModules(policy, evidence)
				if err != nil {
					t.Fatal(err)
				}
				module := moduleNamed(t, modules, test.module)
				ready := hasStep(module, test.readyStep)
				blocked := stepByID(module, policyStepID(test.capability))
				switch {
				case !state.allowed:
					if ready || blocked != nil {
						t.Fatalf("denied capability produced steps: %#v", module.Steps)
					}
				case state.status == EvidenceReady:
					if !ready || blocked != nil {
						t.Fatalf("ready capability steps = %#v", module.Steps)
					}
				default:
					if ready || blocked == nil || blocked.Disposition != planner.DispositionBlocked {
						t.Fatalf("non-ready capability steps = %#v", module.Steps)
					}
				}
			})
		}
	}
}

func TestPortableFakePlanExcludesRiskyStepsAndGalaxyAssetsDespiteObservations(t *testing.T) {
	policy := catalog.ResolvedHostPolicy{
		Name:  "portable-fixture",
		Roles: []string{"workstation"},
		Desired: catalog.Catalog{
			CatalogVersion: 1,
			Kind:           catalog.KindHost,
			Modules:        catalog.ModuleSet{"bootstrap": true, "fingerprint": true, "desktop": true},
			Templates:      append([]string(nil), workstationAssets...),
		},
	}
	evidence := readyEvidence()
	for _, capability := range catalog.RiskCapabilities() {
		evidence.Capabilities[capability] = CapabilityEvidence{State: EvidenceReady}
	}

	plan, err := BuildHostPlan(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !hasPlanStep(plan, DesktopPackagesOperation) || !hasPlanStep(plan, DesktopSessionOperation) {
		t.Fatalf("portable plan lacks workstation base: %#v", plan.Steps)
	}
	for _, forbiddenID := range []string{
		fingerprintPAMStep,
		desktopFixedDisplaysStep,
		desktopFixedInputDevicesStep,
		desktopLiteralHomePathsStep,
		desktopCosmicPruneStep,
		bootstrapPackageInstall,
		bootstrapPackageRemove,
		bootstrapBootPlymouthEdit,
		bootstrapBootMkinitcpio,
		bootstrapBootGRUBEdit,
		bootstrapBootGRUBGenerate,
		bootstrapBootGRUBPublish,
	} {
		if hasPlanStep(plan, forbiddenID) {
			t.Fatalf("portable plan contains risky step %q", forbiddenID)
		}
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"templates/hosts/galaxy", "overlays/galaxy", `output \"DP-`, "/home/alex", "AT Translated Set 2 keyboard"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("portable plan leaks %q: %s", forbidden, encoded)
		}
	}
}

func TestGalaxyPolicyPreservesRiskyBootstrapAndOwnedAssets(t *testing.T) {
	policy := galaxyPolicy("bootstrap", "", false)
	policy.Desired.Modules["desktop"] = true
	policy.Desired.Modules["fingerprint"] = true
	policy.Risks = allRiskPolicy()
	for _, capability := range []catalog.RiskCapability{catalog.RiskFixedDisplays, catalog.RiskFixedInputDevices, catalog.RiskLiteralHomePaths} {
		policy.Desired.Templates = append(policy.Desired.Templates, galaxyCapabilityAssets(capability)...)
	}
	evidence := readyEvidence()
	for _, capability := range catalog.RiskCapabilities() {
		evidence.Capabilities[capability] = CapabilityEvidence{State: EvidenceReady}
	}

	modules, err := BuildModules(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := moduleNamed(t, modules, "bootstrap")
	for _, id := range []string{bootstrapPackageInstall, bootstrapPackageRemove, bootstrapBootPlymouthEdit, bootstrapBootGRUBPublish} {
		if !hasStep(bootstrap, id) {
			t.Fatalf("Galaxy bootstrap lost %q: %v", id, stepIDs(bootstrap.Steps))
		}
	}
	desktop := moduleNamed(t, modules, "desktop")
	for _, id := range []string{DesktopPackagesOperation, DesktopSessionOperation, desktopFixedDisplaysStep, desktopFixedInputDevicesStep, desktopLiteralHomePathsStep, desktopCosmicPruneStep} {
		if !hasStep(desktop, id) {
			t.Fatalf("Galaxy desktop lost %q: %v", id, stepIDs(desktop.Steps))
		}
	}
	if !hasStep(moduleNamed(t, modules, "fingerprint"), fingerprintPAMStep) {
		t.Fatal("Galaxy fingerprint/PAM behavior was not authorized")
	}
}

func TestBuildModulesRejectsForeignHostAssets(t *testing.T) {
	policy := catalog.ResolvedHostPolicy{
		Name: "portable-fixture",
		Desired: catalog.Catalog{
			CatalogVersion: 1,
			Kind:           catalog.KindHost,
			Modules:        catalog.ModuleSet{"desktop": true},
			Templates:      []string{"templates/hosts/galaxy/niri/config.kdl"},
			Overlays:       []string{"overlays/galaxy"},
		},
	}
	if _, err := BuildModules(policy, readyEvidence()); err == nil {
		t.Fatal("portable host accepted Galaxy assets")
	} else if !strings.Contains(err.Error(), "portable-fixture") || !strings.Contains(err.Error(), "galaxy") {
		t.Fatalf("error does not identify ownership mismatch: %v", err)
	}
}

func TestDesktopHostAssetsAreCapabilityIsolated(t *testing.T) {
	for _, test := range []struct {
		name       string
		capability catalog.RiskCapability
		stepID     string
	}{
		{"fixed displays", catalog.RiskFixedDisplays, desktopFixedDisplaysStep},
		{"fixed input devices", catalog.RiskFixedInputDevices, desktopFixedInputDevicesStep},
		{"literal home paths", catalog.RiskLiteralHomePaths, desktopLiteralHomePathsStep},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := galaxyPolicy("desktop", test.capability, true)
			evidence := readyEvidence()
			evidence.Capabilities[test.capability] = CapabilityEvidence{State: EvidenceReady}

			modules, err := BuildModules(policy, evidence)
			if err != nil {
				t.Fatal(err)
			}
			step := stepByID(moduleNamed(t, modules, "desktop"), test.stepID)
			if step == nil {
				t.Fatalf("missing step %q", test.stepID)
			}
			var desired struct {
				Assets []string `json:"assets"`
			}
			if err := json.Unmarshal(step.Desired, &desired); err != nil {
				t.Fatal(err)
			}
			wantSegment := "/" + string(test.capability) + "/"
			if len(desired.Assets) == 0 {
				t.Fatalf("%q has no governed assets", test.capability)
			}
			for _, asset := range desired.Assets {
				if !strings.Contains(asset, wantSegment) {
					t.Fatalf("%q exposed cross-capability asset %q; want segment %q", test.capability, asset, wantSegment)
				}
			}
		})
	}
}

func TestDesktopRejectsWorkstationAssetsWithoutDeclaredRole(t *testing.T) {
	policy := catalog.ResolvedHostPolicy{
		Name: "roleless-fixture",
		Desired: catalog.Catalog{
			CatalogVersion: 1,
			Kind:           catalog.KindHost,
			Modules:        catalog.ModuleSet{"desktop": true},
			Templates:      append([]string(nil), workstationAssets...),
		},
	}
	if _, err := BuildModules(policy, readyEvidence()); err == nil {
		t.Fatal("host without workstation role consumed workstation assets")
	} else if !strings.Contains(err.Error(), "workstation") || !strings.Contains(err.Error(), "roleless-fixture") {
		t.Fatalf("role ownership error = %v", err)
	}
}

func galaxyPolicy(module string, capability catalog.RiskCapability, allowed bool) catalog.ResolvedHostPolicy {
	modules := catalog.ModuleSet{}
	if module != "" {
		modules[module] = true
	}
	policy := catalog.ResolvedHostPolicy{
		Name:  "galaxy",
		Roles: []string{"workstation"},
		Desired: catalog.Catalog{
			CatalogVersion: 1,
			Kind:           catalog.KindHost,
			Modules:        modules,
			Templates:      append([]string(nil), workstationAssets...),
			Overlays:       []string{"overlays/galaxy"},
		},
	}
	if allowed {
		policy.Risks = riskPolicyFor(capability)
		policy.Desired.Templates = append(policy.Desired.Templates, galaxyCapabilityAssets(capability)...)
	}
	return policy
}

func galaxyCapabilityAssets(capability catalog.RiskCapability) []string {
	prefix := "templates/hosts/galaxy/" + string(capability) + "/"
	switch capability {
	case catalog.RiskFixedDisplays:
		return []string{prefix + "niri/config.kdl", prefix + "noctalia/settings.toml"}
	case catalog.RiskFixedInputDevices:
		return []string{prefix + "hyprwhspr/config.json", prefix + "noctalia/settings.toml"}
	case catalog.RiskLiteralHomePaths:
		return []string{prefix + "noctalia/settings.toml"}
	default:
		return nil
	}
}

func readyEvidence() PlatformEvidence {
	return PlatformEvidence{
		Capabilities: map[catalog.RiskCapability]CapabilityEvidence{},
		Bootstrap: BootstrapObservation{
			InstalledPackages: map[string]string{
				"paru":          "1",
				"cosmic-store":  "1",
				"flatpak":       "1",
				"zsh":           "1",
				"google-chrome": "1",
				"firefox":       "1",
				"plymouth":      "1",
			},
			Boot: BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
		},
		Desktop: DesktopObservation{HomeRoot: "/fixture/home", UserName: "fixture"},
	}
}

func riskPolicyFor(capability catalog.RiskCapability) catalog.RiskPolicy {
	var policy catalog.RiskPolicy
	switch capability {
	case catalog.RiskFingerprintPAM:
		policy.FingerprintPAM = true
	case catalog.RiskFixedDisplays:
		policy.FixedDisplays = true
	case catalog.RiskFixedInputDevices:
		policy.FixedInputDevices = true
	case catalog.RiskLiteralHomePaths:
		policy.LiteralHomePaths = true
	case catalog.RiskBootstrapSystemUpdate:
		policy.BootstrapSystemUpdate = true
	case catalog.RiskBootstrapPackageRemoval:
		policy.BootstrapPackageRemoval = true
	case catalog.RiskBootstrapBootMutation:
		policy.BootstrapBootMutation = true
	case catalog.RiskCosmicPrune:
		policy.CosmicPrune = true
	}
	return policy
}

func allRiskPolicy() catalog.RiskPolicy {
	policy := catalog.RiskPolicy{}
	for _, capability := range catalog.RiskCapabilities() {
		one := riskPolicyFor(capability)
		if one.Allows(capability) {
			switch capability {
			case catalog.RiskFingerprintPAM:
				policy.FingerprintPAM = true
			case catalog.RiskFixedDisplays:
				policy.FixedDisplays = true
			case catalog.RiskFixedInputDevices:
				policy.FixedInputDevices = true
			case catalog.RiskLiteralHomePaths:
				policy.LiteralHomePaths = true
			case catalog.RiskBootstrapSystemUpdate:
				policy.BootstrapSystemUpdate = true
			case catalog.RiskBootstrapPackageRemoval:
				policy.BootstrapPackageRemoval = true
			case catalog.RiskBootstrapBootMutation:
				policy.BootstrapBootMutation = true
			case catalog.RiskCosmicPrune:
				policy.CosmicPrune = true
			}
		}
	}
	return policy
}

func moduleNamed(t *testing.T, modules []planner.Module, name string) planner.Module {
	t.Helper()
	for _, module := range modules {
		if module.Name == name {
			return module
		}
	}
	t.Fatalf("module %q not found in %#v", name, modules)
	return planner.Module{}
}

func hasStep(module planner.Module, id string) bool { return stepByID(module, id) != nil }

func stepByID(module planner.Module, id string) *planner.Step {
	for i := range module.Steps {
		if module.Steps[i].ID == id {
			return &module.Steps[i]
		}
	}
	return nil
}

func hasPlanStep(plan planner.Plan, id string) bool {
	return slices.ContainsFunc(plan.Steps, func(step planner.Step) bool { return step.ID == id })
}

func TestObservationRequestIsCanonicalAndImmutable(t *testing.T) {
	policy := galaxyPolicy("desktop", "", false)
	policy.Risks = allRiskPolicy()
	first := ObservationRequestFor(policy)
	second := ObservationRequestFor(policy)
	if !reflect.DeepEqual(first.Capabilities, catalog.RiskCapabilities()) {
		t.Fatalf("request order = %v", first.Capabilities)
	}
	first.Capabilities[0] = "mutated"
	if reflect.DeepEqual(first, second) {
		t.Fatal("observation requests alias mutable capability storage")
	}
}
