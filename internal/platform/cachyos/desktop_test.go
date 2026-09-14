package cachyos

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

func TestDesktopRequestPlanUsesAuthoritativePackagesAndTypedOperations(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	policy := portableDesktopPolicy()
	plan, err := BuildDesktopRequestPlan(policy, DesktopObservation{HomeRoot: home, UserName: "fixture", InstalledPackages: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	wantPackages := []string{
		"accountsservice",
		"ddcutil",
		"foot",
		"gnome-keyring",
		"nautilus",
		"niri",
		"noctalia",
		"noctalia-greeter",
		"playerctl",
		"power-profiles-daemon",
		"python-gobject",
		"quickshell",
		"ttf-meslo-nerd",
		"xdg-desktop-portal-gnome",
		"xdg-desktop-portal-gtk",
		"xwayland-satellite",
	}
	if !reflect.DeepEqual(plan.Packages, wantPackages) {
		t.Fatalf("packages = %#v, want %#v", plan.Packages, wantPackages)
	}
	if len(plan.Requests) != 1 {
		t.Fatalf("requests = %#v, want one package request", plan.Requests)
	}
	request := plan.Requests[0]
	if request.Operation != DesktopPackagesOperation || request.Executable != "/usr/bin/pacman" || request.Scope != runner.ScopeSystem || request.Network != runner.NetworkRequired {
		t.Fatalf("package request = %#v", request)
	}
	if got, want := request.Argv, append([]string{"-S", "--needed", "--noconfirm"}, wantPackages...); !reflect.DeepEqual(got, want) {
		t.Fatalf("package argv = %#v, want %#v", got, want)
	}

	wantOperations := []string{
		DesktopPackagesOperation,
		"desktop.file.hyprwhspr",
		"desktop.file.niri",
		"desktop.file.noctalia",
		"desktop.polkit.model",
		"desktop.polkit.shell",
		DesktopSessionOperation,
	}
	if got := desktopStepIDs(plan.Steps); !reflect.DeepEqual(got, wantOperations) {
		t.Fatalf("step IDs = %#v, want %#v", got, wantOperations)
	}
	for _, step := range plan.Steps {
		if step.ID == DesktopPackagesOperation {
			continue
		}
		if step.Scope != planner.ScopeUser || step.Network != planner.NetworkNone || step.Operation != planner.Operation(DesktopManagedFileOperation) {
			t.Fatalf("file/session step %q = %#v", step.ID, step)
		}
	}
	for _, file := range plan.Files {
		if !strings.HasPrefix(file.File.Path, home+string(filepath.Separator)) {
			t.Fatalf("file escaped runtime home: %#v", file)
		}
		if strings.Contains(file.File.Path, "/home/alex") || strings.Contains(string(file.File.Content), "templates/hosts/galaxy") {
			t.Fatalf("portable file leaked Galaxy data: %#v", file)
		}
	}
}

func TestDesktopRequestPlanRequiresRoleOwnedAssetsAndKeepsRiskyStepsDefaultDenied(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	policy := portableDesktopPolicy()
	policy.Desired.Templates = policy.Desired.Templates[:2]
	if _, err := BuildDesktopRequestPlan(policy, DesktopObservation{HomeRoot: home, UserName: "fixture"}); err == nil || !strings.Contains(err.Error(), "workstation") {
		t.Fatalf("missing role asset error = %v", err)
	}

	policy = portableDesktopPolicy()
	evidence := PlatformEvidence{Desktop: DesktopObservation{HomeRoot: home, UserName: "fixture"}, Capabilities: map[catalog.RiskCapability]CapabilityEvidence{}}
	module, err := buildDesktopModule(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{desktopFixedDisplaysStep, desktopFixedInputDevicesStep, desktopLiteralHomePathsStep, desktopCosmicPruneStep} {
		if hasStep(module, forbidden) {
			t.Fatalf("portable module contains denied step %q: %#v", forbidden, module.Steps)
		}
	}
	for _, step := range module.Steps {
		encoded := string(step.Desired)
		if strings.Contains(encoded, "templates/hosts/galaxy") || strings.Contains(encoded, "/home/alex") {
			t.Fatalf("portable module leaked fixed data in %q: %s", step.ID, encoded)
		}
	}
}

func TestDesktopRequestPlanConvergesPackageAndFileObservations(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	policy := portableDesktopPolicy()
	initial, err := BuildDesktopRequestPlan(policy, DesktopObservation{HomeRoot: home, UserName: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	observation := DesktopObservation{HomeRoot: home, UserName: "fixture", InstalledPackages: map[string]string{}, Files: map[string]DesktopFileObservation{}}
	for _, name := range initial.Packages {
		observation.InstalledPackages[name] = "1.0"
	}
	for _, item := range initial.Files {
		if err := os.MkdirAll(filepath.Dir(item.File.Path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(item.File.Path, item.File.Content, os.FileMode(item.File.Mode)); err != nil {
			t.Fatal(err)
		}
		observed, err := ObserveDesktopFile(item.File.Path)
		if err != nil {
			t.Fatal(err)
		}
		observation.Files[item.File.Path] = observed
	}
	converged, err := BuildDesktopRequestPlan(policy, observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(converged.Requests) != 0 {
		t.Fatalf("converged requests = %#v", converged.Requests)
	}
	for _, step := range converged.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("step %q disposition = %q", step.ID, step.Disposition)
		}
	}
}

func portableDesktopPolicy() catalog.ResolvedHostPolicy {
	return catalog.ResolvedHostPolicy{
		Name:  "portable-fixture",
		Roles: []string{"workstation"},
		Desired: catalog.Catalog{
			CatalogVersion: 1,
			Kind:           catalog.KindHost,
			Modules:        catalog.ModuleSet{"desktop": true},
			Templates:      append([]string(nil), workstationAssets...),
		},
	}
}

func desktopStepIDs(steps []planner.Step) []string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = step.ID
	}
	return ids
}
