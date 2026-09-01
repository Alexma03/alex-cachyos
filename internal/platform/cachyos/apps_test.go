package cachyos

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

func TestAppsPlanHasExactInventoryPinsServicesGroupsAndFallback(t *testing.T) {
	home := t.TempDir()
	entries, err := LoadEmbeddedWebApps()
	if err != nil {
		t.Fatal(err)
	}
	responses := map[string][]byte{}
	errorsByURL := map[string]error{entries[0].IconURL: errors.New("offline")}
	for _, entry := range entries {
		url := entry.IconURL
		if entry.Name == entries[0].Name {
			url, err = DeriveWebAppFaviconURL(entry.URL)
			if err != nil {
				t.Fatal(err)
			}
		}
		responses[url] = testPNG(t, 1, 1)
	}
	fetcher := &scriptedIconFetcher{responses: responses, errors: errorsByURL}
	observation := AppsObservation{
		HomeRoot: home,
		UserName: "alex",
		AURPins:  testAppsPins(),
	}

	requestPlan, err := BuildAppsRequestPlanWithContext(context.Background(), observation, fetcher)
	if err != nil {
		t.Fatalf("BuildAppsRequestPlanWithContext() error = %v", err)
	}
	if !reflect.DeepEqual(requestPlan.PacmanPackages, []string{"chatgpt-desktop-bin", "cursor-bin", "discord", "docker", "localsend", "tailscale"}) {
		t.Fatalf("pacman inventory = %#v", requestPlan.PacmanPackages)
	}
	if !reflect.DeepEqual(requestPlan.AURPackages, []string{"ai-usagebar-bin", "docker-desktop", "hyprwhspr", "nordvpn-bin", "slack-desktop", "warp-terminal-bin"}) {
		t.Fatalf("AUR inventory = %#v", requestPlan.AURPackages)
	}
	if len(requestPlan.AURPins) != len(requestPlan.AURPackages) {
		t.Fatalf("AUR pins = %#v", requestPlan.AURPins)
	}
	for _, name := range requestPlan.AURPackages {
		if err := catalog.ValidateAURLocalPin(name, requestPlan.AURPins[name]); err != nil {
			t.Errorf("pin %q invalid: %v", name, err)
		}
	}

	pacman := requestByOperation(t, requestPlan.Requests, "apps.packages.pacman.install")
	if pacman.Executable != "/usr/bin/pacman" || pacman.Scope != runner.ScopeSystem || pacman.Network != runner.NetworkRequired {
		t.Fatalf("pacman request = %#v", pacman)
	}
	for _, name := range requestPlan.AURPackages {
		paru := requestByOperation(t, requestPlan.Requests, "apps.aur."+name+".install")
		if paru.Executable != "/usr/bin/paru" || paru.Scope != runner.ScopeUser || paru.Network != runner.NetworkRequired {
			t.Fatalf("paru request = %#v", paru)
		}
		if strings.Contains(strings.Join(paru.Argv, "\x00"), "pkexec") || paru.Executable == "/usr/bin/pkexec" {
			t.Fatalf("paru was incorrectly elevated: %#v", paru)
		}
	}
	for _, operation := range []string{
		"apps.service.docker.enable", "apps.service.tailscaled.enable", "apps.service.nordvpnd.enable",
		"apps.group.docker.add", "apps.group.nordvpn.add",
	} {
		request := requestByOperation(t, requestPlan.Requests, operation)
		if request.Scope != runner.ScopeSystem || request.Executable == "/usr/bin/pkexec" {
			t.Errorf("system request %q = %#v", operation, request)
		}
	}
	if requestByOperation(t, requestPlan.Requests, "apps.service.docker-desktop.disable").Scope != runner.ScopeUser {
		t.Fatal("docker-desktop user unit request is not user scoped")
	}

	if len(requestPlan.WebApps.Installs) != len(entries) {
		t.Fatalf("webapps = %#v", requestPlan.WebApps.Installs)
	}
	if got := requestPlan.WebApps.Installs[0].Icon.Outcome; got != IconOutcomeFaviconFallback {
		t.Fatalf("fallback outcome = %q", got)
	}
	if got := requestPlan.WebApps.Launcher.Path; got != filepath.Join(home, ".local", "bin", "alex-cachyos-webapp-launch") {
		t.Fatalf("launcher path = %q", got)
	}
	if !strings.Contains(string(requestPlan.WebApps.Installs[0].Desktop.Content), "Chrome webapp") {
		t.Fatalf("desktop content = %q", requestPlan.WebApps.Installs[0].Desktop.Content)
	}
}

func TestAppsSecondPlanIsSatisfiedAndOrdersBeforeDesktop(t *testing.T) {
	home := t.TempDir()
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{}}
	entries, err := LoadEmbeddedWebApps()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		fetcher.responses[entry.IconURL] = testPNG(t, 1, 1)
	}
	initial := AppsObservation{HomeRoot: home, UserName: "alex", AURPins: testAppsPins()}
	first, err := BuildAppsRequestPlan(initial, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	converged := initial
	converged.InstalledPackages = map[string]string{}
	converged.ExplicitPackages = map[string]bool{}
	for _, name := range append(append([]string{}, first.PacmanPackages...), first.AURPackages...) {
		converged.InstalledPackages[name] = "1.0-1"
		converged.ExplicitPackages[name] = true
	}
	converged.Services = map[string]ServiceObservation{
		"docker.service":     {Unit: "docker.service", Installed: true, Enabled: true, Active: true},
		"tailscaled.service": {Unit: "tailscaled.service", Installed: true, Enabled: true, Active: true},
		"nordvpnd.service":   {Unit: "nordvpnd.service", Installed: true, Enabled: true, Active: true},
	}
	converged.Groups = map[string]bool{"docker": true, "nordvpn": true}
	converged.DockerDesktopDisabled = true
	converged.Files = map[string]AppsFileObservation{}
	for _, file := range first.WebApps.Files() {
		converged.Files[file.Path] = AppsFileObservation{Path: file.Path, Exists: true, SHA256: fileSHA(file.Content), Mode: file.Mode, Ownership: OwnershipCreated}
	}
	second, err := BuildAppsRequestPlan(converged, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Requests) != 0 {
		t.Fatalf("converged requests = %#v", second.Requests)
	}
	for _, step := range second.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("step %q disposition = %s", step.ID, step.Disposition)
		}
	}

	apps, err := BuildAppsModule(converged, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	desktop := planner.Module{Name: "desktop", Enabled: true, DependsOn: []string{"apps"}, Steps: []planner.Step{{ID: "desktop.install", Module: "desktop", Disposition: planner.DispositionApply}}}
	plan, err := planner.BuildPlan([]planner.Module{desktop, apps}, planner.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	lastApps := -1
	firstDesktop := len(plan.Steps)
	for i, step := range plan.Steps {
		if step.Module == "apps" {
			lastApps = i
		}
		if step.Module == "desktop" && firstDesktop == len(plan.Steps) {
			firstDesktop = i
		}
	}
	if lastApps < 0 || firstDesktop <= lastApps {
		t.Fatalf("module order = %#v", plan.Steps)
	}
}

func TestAppsAURInstallationsAreBoundToExactSourceAndChecksumPins(t *testing.T) {
	home := t.TempDir()
	fetcher := embeddedAppsIconFetcher(t)
	pins := testAppsPins()
	plan, err := BuildAppsRequestPlan(AppsObservation{HomeRoot: home, UserName: "alex", AURPins: pins}, fetcher)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range plan.Requests {
		if err := runner.ValidateCommandRequest(request); err != nil {
			t.Fatalf("request %q invalid: %v", request.Operation, err)
		}
		if request.Shell || request.Executable == "/usr/bin/pkexec" || request.Executable == "sudo" {
			t.Fatalf("request %q bypasses typed argv/scope: %#v", request.Operation, request)
		}
	}

	for _, name := range plan.AURPackages {
		prefix := "apps.aur." + name
		pin := pins[name]
		checkout := requestByOperation(t, plan.Requests, prefix+".checkout")
		if !containsArg(checkout.Argv, pin.SourceCommit) {
			t.Fatalf("checkout %q does not carry source commit %q: %#v", name, pin.SourceCommit, checkout)
		}
		verify := requestByOperation(t, plan.Requests, prefix+".patch.verify")
		if !strings.HasPrefix(string(verify.Stdin), pin.PatchSHA256+"  ") {
			t.Fatalf("verify %q does not carry checksum %q: %#v", name, pin.PatchSHA256, verify)
		}
		install := requestByOperation(t, plan.Requests, prefix+".install")
		if install.Executable != "/usr/bin/paru" || install.Scope != runner.ScopeUser || !containsArg(install.Argv, filepath.Join(home, ".cache", "alex-cachyos", "aur", name)) {
			t.Fatalf("pinned local install %q = %#v", name, install)
		}
		if len(install.Argv) == 0 || install.Argv[0] != "-B" || containsArg(install.Argv, "-S") || containsArg(install.Argv, name) {
			t.Fatalf("install %q is not bound to the materialized local source: %#v", name, install.Argv)
		}
		step := planStepByID(t, plan.Steps, prefix+".install")
		if !containsArg(step.DependsOn, prefix+".patch.verify") {
			t.Fatalf("install %q dependencies = %#v", name, step.DependsOn)
		}
		for _, request := range []runner.CommandRequest{checkout, verify, install} {
			if err := runner.ValidateCommandRequest(request); err != nil {
				t.Errorf("request %q invalid: %v", request.Operation, err)
			}
			if request.Shell || request.Executable == "/usr/bin/pkexec" || request.Executable == "sudo" {
				t.Errorf("request %q bypasses typed argv/scope: %#v", request.Operation, request)
			}
		}
	}

	changed := testAppsPins()
	changed[plan.AURPackages[0]] = catalog.AURLocalPin{SourceCommit: strings.Repeat("0", 40), PatchSHA256: strings.Repeat("f", 64)}
	changedPlan, err := BuildAppsRequestPlan(AppsObservation{HomeRoot: home, UserName: "alex", AURPins: changed}, embeddedAppsIconFetcher(t))
	if err != nil {
		t.Fatal(err)
	}
	first := plan.AURPackages[0]
	if reflect.DeepEqual(requestByOperation(t, plan.Requests, "apps.aur."+first+".checkout"), requestByOperation(t, changedPlan.Requests, "apps.aur."+first+".checkout")) {
		t.Fatal("changing the source commit did not change the checkout request")
	}
	if reflect.DeepEqual(requestByOperation(t, plan.Requests, "apps.aur."+first+".patch.verify"), requestByOperation(t, changedPlan.Requests, "apps.aur."+first+".patch.verify")) {
		t.Fatal("changing the checksum did not change the verification request")
	}
	if reflect.DeepEqual(planStepByID(t, plan.Steps, "apps.aur."+first+".install").Desired, planStepByID(t, changedPlan.Steps, "apps.aur."+first+".install").Desired) {
		t.Fatal("changing the pins did not change the installation step identity")
	}
}

func embeddedAppsIconFetcher(t *testing.T) *scriptedIconFetcher {
	t.Helper()
	entries, err := LoadEmbeddedWebApps()
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{}}
	for _, entry := range entries {
		fetcher.responses[entry.IconURL] = testPNG(t, 1, 1)
	}
	return fetcher
}

func containsArg(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func testAppsPins() map[string]catalog.AURLocalPin {
	packages := []string{"warp-terminal-bin", "slack-desktop", "docker-desktop", "hyprwhspr", "ai-usagebar-bin", "nordvpn-bin"}
	pins := make(map[string]catalog.AURLocalPin, len(packages))
	for i, name := range packages {
		pins[name] = catalog.AURLocalPin{
			SourceCommit: strings.Repeat(string(rune('a'+i)), 40),
			PatchSHA256:  strings.Repeat(string(rune('1'+i)), 64),
		}
	}
	return pins
}
