package pi

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"alex-cachyos/internal/catalog"
)

func TestBuildPackagePlanUsesCompleteExactFixtureCatalog(t *testing.T) {
	fixture := loadPackageCatalog(t)
	home := filepath.Join(t.TempDir(), "home")
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	checkout := filepath.Join(home, "Projects", "gentle-pi")
	plan, err := BuildPackagePlan(PackagePlanInput{
		Pins: fixture.Pins, SettingsPath: settings, IntendedLayout: LayoutAgent,
		Checkout: CheckoutReference{Name: "gentle-pi", Path: checkout, ReadOnly: true, Pin: fixture.CheckoutPins["gentle-pi"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := packageNames(plan.Desired), RequiredPackageNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("package names = %#v, want %#v", got, want)
	}
	if len(plan.Installs) != len(plan.Desired) || plan.SettingsPath != settings || plan.IntendedLayout != LayoutAgent {
		t.Fatalf("plan = %#v", plan)
	}
	for _, install := range plan.Installs {
		if install.Mode != InstallExactCatalogPin || install.Name == "" || install.Spec == "" {
			t.Fatalf("install is not catalog-driven: %#v", install)
		}
		if install.Source == PackageLocalPath && install.ResolvedPath != checkout {
			t.Fatalf("local install copied or changed checkout authority: %#v", install)
		}
	}
	for _, desired := range plan.Desired {
		switch desired.Source {
		case PackageNPM:
			if desired.Spec != "npm:"+desired.Name+"@"+desired.DesiredVersion || desired.ResolvedPath != "" || desired.CheckoutCommit != "" {
				t.Fatalf("npm desired = %#v", desired)
			}
		case PackageLocalPath:
			if desired.Name != "gentle-pi" || desired.Spec != catalog.LocalPiPackagePath || desired.ResolvedPath != checkout || desired.CheckoutCommit != fixture.CheckoutPins["gentle-pi"].Commit {
				t.Fatalf("local desired = %#v", desired)
			}
		default:
			t.Fatalf("unknown source in %#v", desired)
		}
	}
	plan.Desired[0].Spec = "changed"
	if fixture.Pins.NPM["@juicesharp/rpiv-ask-user-question"] != "0.0.0-fixture.3" {
		t.Fatal("package plan aliases catalog input")
	}
}

func TestBuildPackagePlanFailsClosedOnIncompleteOrUntrustedAuthority(t *testing.T) {
	fixture := loadPackageCatalog(t)
	home := filepath.Join(t.TempDir(), "home")
	settings := filepath.Join(home, ".pi", "agent", "settings.json")
	checkout := filepath.Join(home, "Projects", "gentle-pi")
	base := PackagePlanInput{Pins: fixture.Pins, SettingsPath: settings, IntendedLayout: LayoutAgent, Checkout: CheckoutReference{Name: "gentle-pi", Path: checkout, ReadOnly: true, Pin: fixture.CheckoutPins["gentle-pi"]}}

	tests := []struct {
		name   string
		mutate func(*PackagePlanInput)
	}{
		{"missing npm package", func(input *PackagePlanInput) { delete(input.Pins.NPM, "pi-web-access") }},
		{"range version", func(input *PackagePlanInput) { input.Pins.NPM["pi-web-access"] = "^1.0.0" }},
		{"unknown npm package", func(input *PackagePlanInput) { input.Pins.NPM["fixture-extra"] = "1.0.0" }},
		{"wrong local reference", func(input *PackagePlanInput) { input.Pins.LocalPathPackages["gentle-pi"] = "../gentle-pi" }},
		{"wrong checkout path", func(input *PackagePlanInput) { input.Checkout.Path = filepath.Join(home, "elsewhere") }},
		{"writable checkout", func(input *PackagePlanInput) { input.Checkout.ReadOnly = false }},
		{"unnamed checkout", func(input *PackagePlanInput) { input.Checkout.Name = "" }},
		{"root settings", func(input *PackagePlanInput) { input.SettingsPath = "/settings.json" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := clonePackageInput(base)
			test.mutate(&input)
			if _, err := BuildPackagePlan(input); err == nil {
				t.Fatal("unsafe package authority was accepted")
			}
		})
	}
}

func TestObservePackagesProbesBothLayoutsAndReportsDesiredVersionDrift(t *testing.T) {
	fixture := loadPackageCatalog(t)
	home := filepath.Join(t.TempDir(), "home")
	plan, err := BuildPackagePlan(PackagePlanInput{
		Pins: fixture.Pins, SettingsPath: filepath.Join(home, ".pi", "agent", "settings.json"), IntendedLayout: LayoutAgent,
		Checkout: CheckoutReference{Name: "gentle-pi", Path: filepath.Join(home, "Projects", "gentle-pi"), ReadOnly: true, Pin: fixture.CheckoutPins["gentle-pi"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantVersions := desiredVersions(plan)
	wantVersions["pi-web-access"] = "9.9.9-fixture"
	probe := &fixturePackageProbe{byLayout: map[PackageLayout]map[string]string{LayoutAgent: wantVersions, LayoutLegacy: {}}}
	observation, err := ObservePackages(context.Background(), plan, probe)
	if err != nil {
		t.Fatal(err)
	}
	wantRoots := []string{filepath.Join(home, ".pi", "agent", "npm"), filepath.Join(home, ".pi", "npm")}
	if !reflect.DeepEqual(probe.roots, wantRoots) || observation.ActiveLayout != LayoutAgent {
		t.Fatalf("probe roots/observation = %#v / %#v", probe.roots, observation)
	}
	report := CheckPackageDrift(plan, observation)
	if !report.Drift || len(report.Packages) != 1 {
		t.Fatalf("report = %#v", report)
	}
	drift := report.Packages[0]
	if drift.Name != "pi-web-access" || drift.Desired != fixture.Pins.NPM["pi-web-access"] || drift.Resolved != "9.9.9-fixture" || drift.Layout != LayoutAgent {
		t.Fatalf("drift = %#v", drift)
	}
	if plan.Desired[packageIndex(t, plan, "pi-web-access")].DesiredVersion != fixture.Pins.NPM["pi-web-access"] {
		t.Fatal("resolved version redefined the desired pin")
	}
}

func TestObservePackagesFailsClosedWhenBothLayoutsContainDesiredPackages(t *testing.T) {
	fixture := loadPackageCatalog(t)
	home := filepath.Join(t.TempDir(), "home")
	plan, err := BuildPackagePlan(PackagePlanInput{
		Pins: fixture.Pins, SettingsPath: filepath.Join(home, ".pi", "agent", "settings.json"), IntendedLayout: LayoutAgent,
		Checkout: CheckoutReference{Name: "gentle-pi", Path: filepath.Join(home, "Projects", "gentle-pi"), ReadOnly: true, Pin: fixture.CheckoutPins["gentle-pi"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	probe := &fixturePackageProbe{byLayout: map[PackageLayout]map[string]string{
		LayoutAgent:  {"pi-web-access": "0.0.0-fixture.4"},
		LayoutLegacy: {"pi-web-access": "0.0.0-fixture.4"},
	}}
	observation, err := ObservePackages(context.Background(), plan, probe)
	if err != nil {
		t.Fatal(err)
	}
	if observation.ActiveLayout != LayoutMultiple || !CheckPackageDrift(plan, observation).Drift {
		t.Fatalf("ambiguous observation = %#v", observation)
	}
}

type fixturePackageProbe struct {
	byLayout map[PackageLayout]map[string]string
	roots    []string
}

func (probe *fixturePackageProbe) Probe(_ context.Context, request PackageProbeRequest) (map[string]string, error) {
	probe.roots = append(probe.roots, request.Root)
	versions := probe.byLayout[request.Layout]
	result := make(map[string]string, len(versions))
	for name, version := range versions {
		result[name] = version
	}
	return result, nil
}

func loadPackageCatalog(t *testing.T) catalog.Catalog {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "package-catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := catalog.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return fixture
}

func clonePackageInput(input PackagePlanInput) PackagePlanInput {
	cloned := input
	cloned.Pins = &catalog.Pins{NPM: map[string]string{}, LocalPathPackages: map[string]string{}}
	for name, version := range input.Pins.NPM {
		cloned.Pins.NPM[name] = version
	}
	for name, path := range input.Pins.LocalPathPackages {
		cloned.Pins.LocalPathPackages[name] = path
	}
	return cloned
}

func packageNames(desired []DesiredPackage) []string {
	result := make([]string, len(desired))
	for i, item := range desired {
		result[i] = item.Name
	}
	return result
}

func desiredVersions(plan PackagePlan) map[string]string {
	result := make(map[string]string, len(plan.Desired))
	for _, desired := range plan.Desired {
		result[desired.Name] = desired.DesiredVersion
	}
	return result
}

func packageIndex(t *testing.T, plan PackagePlan, name string) int {
	t.Helper()
	for i, desired := range plan.Desired {
		if desired.Name == name {
			return i
		}
	}
	t.Fatalf("package %q is missing", name)
	return -1
}
