package cachyos

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
)

func TestBootstrapModuleBindsCompleteRequestIdentityAndEvidence(t *testing.T) {
	home := t.TempDir()
	input := BootstrapObservation{
		InstalledPackages: map[string]string{
			"zsh":                "5.9",
			"paru":               "2.0",
			"firefox":            "1",
			"vim":                "9",
			"cachyos-zsh-config": "1",
			"firefox-i18n-de":    "1",
			"ananicy-cpp":        "1",
			"ufw":                "0.36",
			"linux-cachyos-lts":  "6.12",
			"unrelated-lts":      "1.0",
		},
		ExplicitSet: []string{"zsh"},
		Services: []ServiceObservation{
			{Unit: "ananicy-cpp.service", Enabled: false, Active: false},
			{Unit: "ufw.service", Enabled: true, Active: false},
		},
		Boot:     BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
		HomeRoot: home,
		Catalog:  &catalog.Catalog{Pins: &catalog.Pins{AURLocal: map[string]catalog.AURLocalPin{"google-chrome": testChromePin()}}},
	}
	requestPlan := mustPlan(t, BootstrapInputs{
		InstalledPackages: []InstalledPackage{
			{Name: "zsh", Explicit: true},
			{Name: "paru"},
			{Name: "firefox", Explicit: true},
			{Name: "vim", Explicit: true},
			{Name: "cachyos-zsh-config", Explicit: true},
			{Name: "firefox-i18n-de", Explicit: true},
			{Name: "ananicy-cpp"},
			{Name: "ufw"},
		},
		FirefoxI18N: []string{"firefox-i18n-de"},
		Services:    input.Services,
		HomeRoot:    home,
		ChromePin:   testChromePin(),
		Boot:        input.Boot,
	})
	module := mustBootstrapModule(t, input)

	for _, request := range requestPlan.Requests {
		step := bootstrapStep(t, module, request.Operation)
		var desired map[string]any
		if err := json.Unmarshal(step.Desired, &desired); err != nil {
			t.Fatalf("decode %s desired: %v", request.Operation, err)
		}
		identity, ok := desired["request"].(map[string]any)
		if !ok {
			t.Fatalf("%s has no request identity: %#v", request.Operation, desired)
		}
		if identity["operation"] != request.Operation || identity["executable"] != request.Executable || identity["cwd"] != request.Cwd || identity["scope"] != string(request.Scope) || identity["network"] != string(request.Network) || identity["outputPolicy"] != string(request.OutputPolicy) || identity["timeout"] != float64(int64(request.Timeout)) || identity["outputLimit"] != float64(request.OutputLimit) {
			t.Fatalf("%s identity = %#v", request.Operation, identity)
		}
		argv, ok := identity["argv"].([]any)
		if !ok || len(argv) != len(request.Argv) {
			t.Fatalf("%s argv identity = %#v", request.Operation, identity["argv"])
		}
		for i, arg := range request.Argv {
			if argv[i] != arg {
				t.Fatalf("%s argv[%d] = %#v, want %q", request.Operation, i, argv[i], arg)
			}
		}
		wantHash := fmt.Sprintf("%x", sha256.Sum256(request.Stdin))
		if identity["stdinSha256"] != wantHash {
			t.Fatalf("%s stdin hash = %#v, want %s", request.Operation, identity["stdinSha256"], wantHash)
		}
		if _, present := identity["stdin"]; present || strings.Contains(string(step.Desired), "import pathlib") {
			t.Fatalf("%s leaked raw stdin in plan JSON: %s", request.Operation, step.Desired)
		}
	}

	var installDesired map[string]any
	if err := json.Unmarshal(bootstrapStep(t, module, bootstrapPackageInstall).Desired, &installDesired); err != nil {
		t.Fatal(err)
	}
	if installDesired["pacmanRepositoryPolicy"] != BootstrapPacmanRepositoryPolicy || installDesired["pacmanTransactionPolicy"] != BootstrapPacmanTransactionPolicy {
		t.Fatalf("install policy evidence = %#v", installDesired)
	}
	installStep := bootstrapStep(t, module, bootstrapPackageInstall)
	var installObserved map[string]any
	if err := json.Unmarshal(installStep.Observed, &installObserved); err != nil {
		t.Fatal(err)
	}
	installBefore, ok := installObserved["beforeVersions"].(map[string]any)
	if !ok || len(installBefore) != len(input.InstalledPackages) {
		t.Fatalf("full-system pre-transaction snapshot = %#v", installObserved)
	}
	for name, version := range input.InstalledPackages {
		if installBefore[name] != version {
			t.Fatalf("pre-transaction version %q = %#v, want %q", name, installBefore[name], version)
		}
	}
	if installStep.Inverse == nil || installStep.Inverse.Operation != planner.Operation("bootstrap.packages.rollback.external-system") {
		t.Fatalf("full-system inverse = %#v", installStep.Inverse)
	}
	var rollback map[string]any
	if err := json.Unmarshal(installStep.Inverse.Value, &rollback); err != nil {
		t.Fatal(err)
	}
	policy, ok := rollback["rollbackPolicy"].(map[string]any)
	if !ok || policy["type"] != "external-system" || policy["provider"] != "snapper" || policy["scope"] != "system" {
		t.Fatalf("full-system rollback policy = %#v", rollback)
	}
	if _, present := rollback["requestedNames"]; present {
		t.Fatalf("full-system rollback still claims package removal: %#v", rollback)
	}
	if rollback["pacmanRepositoryPolicy"] != BootstrapPacmanRepositoryPolicy || rollback["pacmanTransactionPolicy"] != BootstrapPacmanTransactionPolicy {
		t.Fatalf("full-system rollback transaction policy = %#v", rollback)
	}
	rollbackBefore, ok := rollback["beforeVersions"].(map[string]any)
	if !ok || len(rollbackBefore) != len(input.InstalledPackages) {
		t.Fatalf("full-system rollback version evidence = %#v", rollback)
	}
	var removeObserved map[string]any
	if err := json.Unmarshal(bootstrapStep(t, module, bootstrapPackageRemove).Observed, &removeObserved); err != nil {
		t.Fatal(err)
	}
	beforeVersions, ok := removeObserved["beforeVersions"].(map[string]any)
	if !ok || beforeVersions["firefox"] != "1" || beforeVersions["vim"] != "9" || beforeVersions["firefox-i18n-de"] != "1" {
		t.Fatalf("remove version evidence = %#v", removeObserved)
	}
	if _, protected := beforeVersions["linux-cachyos-lts"]; protected {
		t.Fatalf("LTS kernel appeared in removal evidence: %#v", beforeVersions)
	}
}

func TestBootstrapModuleUsesIndependentBootDeltaChains(t *testing.T) {
	tests := []struct {
		name       string
		boot       BootObservation
		present    []string
		absent     []string
		dependency [][2]string
	}{
		{
			name:       "Plymouth only",
			boot:       BootObservation{MkinitcpioHasPlymouth: true},
			present:    []string{bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio},
			absent:     []string{bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate, bootstrapBootGRUBPublish},
			dependency: [][2]string{{bootstrapBootMkinitcpio, bootstrapBootPlymouthEdit}},
		},
		{
			name:       "GRUB only",
			boot:       BootObservation{GrubHasSplash: true, GrubGeneratorAvailable: true},
			present:    []string{bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate, bootstrapBootGRUBPublish},
			absent:     []string{bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio},
			dependency: [][2]string{{bootstrapBootGRUBGenerate, bootstrapBootGRUBEdit}, {bootstrapBootGRUBPublish, bootstrapBootGRUBGenerate}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			module := mustBootstrapModule(t, bootstrapObservationForBoot(test.boot))
			ids := stepIDs(module.Steps)
			for _, want := range test.present {
				if !slices.Contains(ids, want) {
					t.Fatalf("missing %s in %v", want, ids)
				}
			}
			for _, unwanted := range test.absent {
				if slices.Contains(ids, unwanted) {
					t.Fatalf("unexpected %s in %v", unwanted, ids)
				}
			}
			for _, edge := range test.dependency {
				if !slices.Contains(bootstrapStep(t, module, edge[0]).DependsOn, edge[1]) {
					t.Fatalf("%s lacks dependency %s", edge[0], edge[1])
				}
			}
			if test.name == "Plymouth only" {
				if slices.Contains(bootstrapStep(t, module, bootstrapBootMkinitcpio).DependsOn, bootstrapBootGRUBEdit) {
					t.Fatal("Plymouth chain depends on unrelated GRUB edit")
				}
			} else if slices.Contains(bootstrapStep(t, module, bootstrapBootGRUBGenerate).DependsOn, bootstrapBootPlymouthEdit) {
				t.Fatal("GRUB chain depends on unrelated Plymouth edit")
			}
		})
	}
}

func TestBootstrapPlanOrdersIndependentBootChains(t *testing.T) {
	plan := mustBootstrapPlan(t, bootstrapObservationForBoot(BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true}))
	got := stepIDs(plan.Steps)
	want := []string{bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate, bootstrapBootGRUBPublish, bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio, bootstrapLTS, bootstrapZsh}
	if !slices.Equal(got, want) {
		t.Fatalf("bootstrap plan order = %v, want %v", got, want)
	}
}

func TestBootstrapModuleNormalizesServicesFromPackages(t *testing.T) {
	observation := bootstrapObservationForBoot(BootObservation{})
	observation.InstalledPackages["ananicy-cpp"] = "1"
	observation.InstalledPackages["ufw"] = "1"
	observation.Services = []ServiceObservation{
		{Unit: "ananicy-cpp", Installed: false, Enabled: false, Active: false},
		{Unit: "ufw.service", Installed: false, Enabled: true, Active: true},
		{Unit: "docker.service", Installed: true, Enabled: false, Active: false},
	}
	module := mustBootstrapModule(t, observation)
	ananicy := bootstrapStep(t, module, "bootstrap.service.ananicy-cpp")
	if slices.Contains(stepIDs(module.Steps), "bootstrap.service.ufw") || slices.Contains(stepIDs(module.Steps), "bootstrap.service.docker") {
		t.Fatalf("service steps = %v", stepIDs(module.Steps))
	}
	var observed map[string]any
	if err := json.Unmarshal(ananicy.Observed, &observed); err != nil {
		t.Fatal(err)
	}
	if observed["installed"] != true || observed["enabled"] != false || observed["active"] != false {
		t.Fatalf("package-authoritative service observation = %#v", observed)
	}
}

func TestBootstrapModuleRejectsDuplicateOrContradictoryServices(t *testing.T) {
	tests := []struct {
		name     string
		services []ServiceObservation
	}{
		{
			name: "normalized duplicate",
			services: []ServiceObservation{
				{Unit: "ufw", Enabled: false, Active: false},
				{Unit: "ufw.service", Enabled: false, Active: false},
			},
		},
		{
			name: "contradictory state",
			services: []ServiceObservation{
				{Unit: "ananicy-cpp.service", Enabled: false, Active: false},
				{Unit: "ananicy-cpp", Enabled: true, Active: false},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildBootstrapModule(BootstrapObservation{InstalledPackages: map[string]string{"ufw": "1", "ananicy-cpp": "1"}, ChromeInstalled: true, Services: test.services})
			if err == nil || !strings.Contains(err.Error(), "service") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestBootstrapModuleUsesExactCachyOSLTSAllowlist(t *testing.T) {
	input := bootstrapObservationForBoot(BootObservation{})
	input.InstalledPackages["linux-cachyos-lts"] = "6.12"
	input.InstalledPackages["linux-cachyos-lts-headers"] = "6.12"
	input.InstalledPackages["unrelated-lts"] = "2.0"
	module := mustBootstrapModule(t, input)
	lts := bootstrapStep(t, module, bootstrapLTS)
	var desired, observed map[string]any
	if err := json.Unmarshal(lts.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(lts.Observed, &observed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(desired["packages"], []any{"linux-cachyos-lts", "linux-cachyos-lts-headers"}) {
		t.Fatalf("LTS desired allowlist = %#v", desired["packages"])
	}
	before := observed["beforeVersions"].(map[string]any)
	if len(before) != 2 || before["linux-cachyos-lts"] != "6.12" || before["linux-cachyos-lts-headers"] != "6.12" {
		t.Fatalf("LTS observed versions = %#v", before)
	}
	if lts.Disposition != planner.DispositionBlocked {
		t.Fatalf("LTS disposition = %s", lts.Disposition)
	}

	requestPlan := mustPlan(t, BootstrapInputs{
		InstalledPackages: append(wantedPackages(), InstalledPackage{Name: "unrelated-lts"}, InstalledPackage{Name: "firefox-i18n-lts"}),
		FirefoxI18N:       []string{"firefox-i18n-lts"},
		ChromeInstalled:   true,
	})
	remove := request(t, requestPlan, bootstrapPackageRemove)
	if !slices.Contains(remove.Argv, "firefox-i18n-lts") {
		t.Fatalf("dynamic locale was incorrectly treated as CachyOS LTS: %#v", remove.Argv)
	}
}

func TestBootstrapModuleAndRequestKernelDoNotAliasMutationInputs(t *testing.T) {
	input := BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "2", "zsh": "5", "ananicy-cpp": "1"},
		ExplicitSet:       []string{"zsh"},
		Services:          []ServiceObservation{{Unit: "ananicy-cpp.service", Enabled: false, Active: false}},
		ChromeInstalled:   true,
		Boot:              BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
	}
	first := mustBootstrapModule(t, input)
	firstPlan := mustBootstrapPlan(t, input)
	firstDesired := make([]byte, len(bootstrapStep(t, first, bootstrapPackageExplicit).Desired))
	copy(firstDesired, bootstrapStep(t, first, bootstrapPackageExplicit).Desired)

	input.InstalledPackages["paru"] = "mutated"
	input.ExplicitSet[0] = "mutated"
	input.Services[0].Unit = "ufw.service"
	input.Boot.GrubGeneratorAvailable = false

	if got := bootstrapStep(t, first, bootstrapPackageExplicit).Desired; !bytesEqual(got, firstDesired) {
		t.Fatalf("module output changed after input mutation: %s", got)
	}
	second := mustBootstrapModule(t, BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "2", "zsh": "5", "ananicy-cpp": "1"},
		ExplicitSet:       []string{"zsh"},
		Services:          []ServiceObservation{{Unit: "ananicy-cpp.service", Enabled: false, Active: false}},
		ChromeInstalled:   true,
		Boot:              BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("equivalent inputs produced different module values after mutation")
	}

	firstPlan.Steps[0].Desired[0] = 'x'
	freshPlan := mustBootstrapPlan(t, BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "2", "zsh": "5", "ananicy-cpp": "1"},
		ExplicitSet:       []string{"zsh"},
		Services:          []ServiceObservation{{Unit: "ananicy-cpp.service", Enabled: false, Active: false}},
		ChromeInstalled:   true,
		Boot:              BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
	})
	freshEdit := bootstrapStep(t, planner.Module{Steps: freshPlan.Steps}, bootstrapBootGRUBEdit)
	if freshEdit.Desired[0] == 'x' {
		t.Fatal("plan construction reused mutable JSON storage")
	}
}

func TestBootstrapModuleReconcilesDependencyOwnedChrome(t *testing.T) {
	observation := bootstrapObservationForBoot(BootObservation{})
	observation.ChromeInstalled = false
	observation.ExplicitSet = []string{"paru", "cosmic-store", "flatpak", "zsh", "nano"}
	module := mustBootstrapModule(t, observation)
	if slices.Contains(stepIDs(module.Steps), bootstrapChromeInstall) {
		t.Fatalf("dependency-owned Chrome was incorrectly reinstalled: %v", stepIDs(module.Steps))
	}
	explicit := bootstrapStep(t, module, bootstrapPackageExplicit)
	if explicit.Disposition != planner.DispositionApply {
		t.Fatalf("Chrome ownership reconciliation disposition = %s", explicit.Disposition)
	}
	var desired map[string]any
	if err := json.Unmarshal(explicit.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(desired["requestedNames"], []any{"google-chrome"}) {
		t.Fatalf("Chrome ownership reconciliation names = %#v", desired["requestedNames"])
	}
}

func TestBootstrapModuleUsesResolvedPolicyChromePin(t *testing.T) {
	policy := galaxyPolicy("bootstrap", "", false)
	policy.Desired.Pins = &catalog.Pins{AURLocal: map[string]catalog.AURLocalPin{
		"google-chrome": testChromePin(),
	}}
	evidence := readyEvidence()
	delete(evidence.Bootstrap.InstalledPackages, "google-chrome")
	evidence.Bootstrap.ChromeInstalled = false
	evidence.Bootstrap.HomeRoot = t.TempDir()
	evidence.Bootstrap.Catalog = &catalog.Catalog{Pins: &catalog.Pins{AURLocal: map[string]catalog.AURLocalPin{
		"google-chrome": {SourceCommit: strings.Repeat("0", 40), PatchSHA256: strings.Repeat("f", 64)},
	}}}

	modules, err := BuildModules(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := moduleNamed(t, modules, bootstrapModuleName)
	assertStepSourceCommit(t, bootstrap, "bootstrap.chrome.checkout", testChromeCommit)
	assertStepPatchSHA256(t, bootstrap, "bootstrap.chrome.patch.verify", testChromePatchSHA)
	for _, edge := range [][2]string{
		{bootstrapChromeCheckout, bootstrapChromeFetch},
		{bootstrapChromePatchMaterialize, bootstrapChromeCheckout},
		{bootstrapChromePatchVerify, bootstrapChromePatchMaterialize},
		{bootstrapChromeInstall, bootstrapChromePatchVerify},
	} {
		if !slices.Contains(bootstrapStep(t, bootstrap, edge[0]).DependsOn, edge[1]) {
			t.Fatalf("Chrome step %q lacks verified-source dependency %q", edge[0], edge[1])
		}
	}
	install := bootstrapStep(t, bootstrap, bootstrapChromeInstall)
	var desired struct {
		PackageManager string `json:"packageManager"`
		Request        struct {
			Executable string   `json:"executable"`
			Argv       []string `json:"argv"`
		} `json:"request"`
	}
	if err := json.Unmarshal(install.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if desired.PackageManager != "paru-local-build" || desired.Request.Executable != "/usr/bin/paru" || len(desired.Request.Argv) == 0 || desired.Request.Argv[0] != "-B" || slices.Contains(desired.Request.Argv, "-S") {
		t.Fatalf("Chrome install is not bound to the resolved local source: %#v", desired)
	}
}

func TestBootstrapPlanIsDeterministicAndConverged(t *testing.T) {
	observation := BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "2", "cosmic-store": "1", "flatpak": "1", "zsh": "5", "nano": "8", "ananicy-cpp": "1", "ufw": "1", "google-chrome": "1"},
		ExplicitSet:       []string{"paru", "cosmic-store", "flatpak", "zsh", "nano", "google-chrome"},
		Services: []ServiceObservation{
			{Unit: "ananicy-cpp.service", Installed: false, Enabled: true, Active: true},
			{Unit: "ufw.service", Installed: false, Enabled: true, Active: true},
		},
		ChromeInstalled: true,
		ZshConverged:    true,
	}
	first := mustBootstrapPlan(t, observation)
	second := mustBootstrapPlan(t, observation)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repeated bootstrap plans differ")
	}
	for _, step := range first.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("converged step is actionable: %#v", step)
		}
	}
	if digest, err := planner.Digest(first); err != nil || digest == "" {
		t.Fatalf("digest = %q, %v", digest, err)
	}
}

func mustBootstrapModule(t *testing.T, observation BootstrapObservation) planner.Module {
	t.Helper()
	module, err := BuildBootstrapModule(observation)
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func mustBootstrapPlan(t *testing.T, observation BootstrapObservation) planner.Plan {
	t.Helper()
	plan, err := BuildBootstrapPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func bootstrapStep(t *testing.T, module planner.Module, id string) planner.Step {
	t.Helper()
	for _, step := range module.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("missing step %q", id)
	return planner.Step{}
}

func stepIDs(steps []planner.Step) []string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	return ids
}

func bytesEqual(left, right []byte) bool {
	return reflect.DeepEqual(left, right)
}

func bootstrapObservationForBoot(boot BootObservation) BootstrapObservation {
	return BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "2", "cosmic-store": "1", "flatpak": "1", "zsh": "5", "nano": "8", "google-chrome": "1"},
		ExplicitSet:       []string{"paru", "cosmic-store", "flatpak", "zsh", "nano", "google-chrome"},
		ChromeInstalled:   true,
		ZshConverged:      true,
		Boot:              boot,
	}
}
