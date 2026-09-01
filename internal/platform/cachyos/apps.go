package cachyos

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const AppsModuleName = "apps"

var ErrInvalidAppsObservation = errors.New("invalid apps observation")

type AppsFileObservation struct {
	Path       string
	Exists     bool
	SHA256     string
	Mode       uint32
	Ownership  FileOwnershipKind
	BackupPath string
}

type AppsAURSourceObservation struct {
	Exists      bool
	Remote      string
	Commit      string
	PatchSHA256 string
}

type AppsObservation struct {
	HomeRoot              string
	UserName              string
	InstalledPackages     map[string]string
	ExplicitPackages      map[string]bool
	Services              map[string]ServiceObservation
	Groups                map[string]bool
	DockerDesktopDisabled bool
	Files                 map[string]AppsFileObservation
	AURSources            map[string]AppsAURSourceObservation
	AURPins               map[string]catalog.AURLocalPin
	Catalog               *catalog.Catalog
}

type AppsRequestPlan struct {
	Requests       []runner.CommandRequest
	PacmanPackages []string
	AURPackages    []string
	AURPins        map[string]catalog.AURLocalPin
	WebApps        WebAppPlan
	Files          []ManagedFileDescriptor
	Steps          []planner.Step
}

func BuildAppsRequestPlan(observation AppsObservation, fetcher IconFetcher) (AppsRequestPlan, error) {
	return BuildAppsRequestPlanWithContext(context.Background(), observation, fetcher)
}

func BuildAppsRequestPlanWithContext(ctx context.Context, observation AppsObservation, fetcher IconFetcher) (AppsRequestPlan, error) {
	home, err := validateAppsObservation(observation)
	if err != nil {
		return AppsRequestPlan{}, err
	}
	pacmanPackages, err := embeddedPackageList("apps/packages.pacman")
	if err != nil {
		return AppsRequestPlan{}, err
	}
	aurPackages, err := embeddedPackageList("apps/packages.aur")
	if err != nil {
		return AppsRequestPlan{}, err
	}
	pins, err := resolveAppsPins(observation, aurPackages)
	if err != nil {
		return AppsRequestPlan{}, err
	}
	roots, err := NewWebAppRoots(home, filepath.Join(home, ".local"))
	if err != nil {
		return AppsRequestPlan{}, err
	}
	webApps, err := BuildWebAppPlanWithContextAndRoots(ctx, roots, fetcher)
	if err != nil {
		return AppsRequestPlan{}, err
	}
	result := AppsRequestPlan{
		PacmanPackages: append([]string(nil), pacmanPackages...),
		AURPackages:    append([]string(nil), aurPackages...),
		AURPins:        copyAURPins(pins),
		WebApps:        webApps.Clone(),
		Files:          webApps.Files(),
	}

	missingPacman := missingPackages(pacmanPackages, observation.InstalledPackages)
	if len(missingPacman) != 0 {
		request, err := makeRequest("apps.packages.pacman.install", "/usr/bin/pacman", append([]string{"-S", "--needed", "--noconfirm"}, missingPacman...), runner.ScopeSystem, runner.NetworkRequired, nil)
		if err != nil {
			return AppsRequestPlan{}, err
		}
		result.Requests = append(result.Requests, request)
		result.Steps = append(result.Steps, appsPackageStep(request, missingPacman, observation.InstalledPackages, nil))
	} else {
		result.Steps = append(result.Steps, appsSatisfiedPackageStep("apps.packages.pacman.install", runner.ScopeSystem, pacmanPackages, observation.InstalledPackages, nil))
	}

	missingAUR := missingPackages(aurPackages, observation.InstalledPackages)
	missingAURSet := make(map[string]bool, len(missingAUR))
	for _, name := range missingAUR {
		missingAURSet[name] = true
	}
	aurInstallSteps := make([]string, 0, len(aurPackages))
	for _, name := range aurPackages {
		installID := "apps.aur." + name + ".install"
		aurInstallSteps = append(aurInstallSteps, installID)
		if !missingAURSet[name] {
			result.Steps = append(result.Steps, appsSatisfiedAURStep(name, pins[name], observation.InstalledPackages[name]))
			continue
		}
		if err := addPinnedAURInstall(&result, observation, home, name, pins[name]); err != nil {
			return AppsRequestPlan{}, err
		}
	}

	allPackages := append(append([]string(nil), pacmanPackages...), aurPackages...)
	sort.Strings(allPackages)
	notExplicit := make([]string, 0)
	for _, name := range allPackages {
		if !observation.ExplicitPackages[name] {
			notExplicit = append(notExplicit, name)
		}
	}
	if len(notExplicit) != 0 {
		request, err := makeRequest("apps.packages.explicit", "/usr/bin/pacman", append([]string{"-D", "--asexplicit"}, notExplicit...), runner.ScopeSystem, runner.NetworkNone, nil)
		if err != nil {
			return AppsRequestPlan{}, err
		}
		result.Requests = append(result.Requests, request)
		step := moduleRequestStep(AppsModuleName, request, planner.DispositionApply, map[string]any{"requestedNames": notExplicit, "ownership": "explicit"}, map[string]any{"explicit": explicitSubset(observation.ExplicitPackages, allPackages)}, "apps.packages.explicit.restore")
		step.DependsOn = append(existingAppsDependencies(result.Steps, "apps.packages.pacman.install"), aurInstallSteps...)
		result.Steps = append(result.Steps, step)
	} else {
		result.Steps = append(result.Steps, planner.Step{ID: "apps.packages.explicit", Module: AppsModuleName, Description: "mark declared application packages explicit", Scope: planner.ScopeSystem, Network: planner.NetworkNone, Operation: "apps.packages.explicit", Disposition: planner.DispositionSatisfied, Desired: mustJSON(map[string]any{"requestedNames": allPackages, "ownership": "explicit"}), Observed: mustJSON(map[string]any{"explicit": allPackages}), Inverse: &planner.InverseDescriptor{Operation: "apps.packages.explicit.restore", Value: mustJSON(map[string]any{"before": allPackages})}})
	}

	for _, service := range []struct {
		unit       string
		dependency string
	}{
		{"docker.service", "apps.packages.pacman.install"},
		{"tailscaled.service", "apps.packages.pacman.install"},
		{"nordvpnd.service", "apps.aur.nordvpn-bin.install"},
	} {
		if err := addAppsService(&result, observation, service.unit, service.dependency); err != nil {
			return AppsRequestPlan{}, err
		}
	}
	for _, group := range []struct {
		name       string
		dependency string
	}{
		{"docker", "apps.service.docker.enable"},
		{"nordvpn", "apps.service.nordvpnd.enable"},
	} {
		if err := addAppsGroup(&result, observation, group.name, group.dependency); err != nil {
			return AppsRequestPlan{}, err
		}
	}
	if err := addDockerDesktopDisable(&result, observation); err != nil {
		return AppsRequestPlan{}, err
	}

	for _, file := range result.Files {
		observed := observation.Files[file.Path]
		step, err := managedFileStep(AppsModuleName, appsFileStepID(result.WebApps, file), file, DevtoolsFileObservation{
			Path: observed.Path, Exists: observed.Exists, SHA256: observed.SHA256, Mode: observed.Mode, Ownership: observed.Ownership, BackupPath: observed.BackupPath,
		}, []string{"apps.packages.pacman.install"})
		if err != nil {
			return AppsRequestPlan{}, err
		}
		if file.Kind == "icon" {
			step.Network = planner.NetworkRequired
			step.Desired = mustJSON(map[string]any{"path": file.Path, "sha256": fileSHA256(file.Content), "mode": file.Mode, "kind": file.Kind, "iconOutcome": file.Outcome, "sourceURL": file.SourceURL})
		}
		result.Steps = append(result.Steps, step)
	}

	sort.SliceStable(result.Steps, func(i, j int) bool {
		return appsStepRank(result.Steps[i].ID) < appsStepRank(result.Steps[j].ID) || appsStepRank(result.Steps[i].ID) == appsStepRank(result.Steps[j].ID) && result.Steps[i].ID < result.Steps[j].ID
	})
	result.Requests = cloneVicinaeRequests(result.Requests)
	result.Files = cloneVicinaeFiles(result.Files)
	result.Steps = cloneVicinaeSteps(result.Steps)
	return result, nil
}

func BuildAppsModule(observation AppsObservation, fetcher IconFetcher) (planner.Module, error) {
	return BuildAppsModuleWithContext(context.Background(), observation, fetcher)
}

func BuildAppsModuleWithContext(ctx context.Context, observation AppsObservation, fetcher IconFetcher) (planner.Module, error) {
	requestPlan, err := BuildAppsRequestPlanWithContext(ctx, observation, fetcher)
	if err != nil {
		return planner.Module{}, err
	}
	return planner.Module{Name: AppsModuleName, Enabled: true, Steps: requestPlan.Steps}, nil
}

func BuildAppsPlan(observation AppsObservation, fetcher IconFetcher) (planner.Plan, error) {
	module, err := BuildAppsModule(observation, fetcher)
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.BuildPlan([]planner.Module{module}, planner.Selection{Only: []string{AppsModuleName}})
}

func validateAppsObservation(observation AppsObservation) (string, error) {
	home := observation.HomeRoot
	if home == "" || !filepath.IsAbs(home) || filepath.Clean(home) != home || !utf8.ValidString(home) || hasControl(home) {
		return "", fmt.Errorf("%w: home root must be a clean absolute path", ErrInvalidAppsObservation)
	}
	if observation.UserName == "" || strings.TrimSpace(observation.UserName) != observation.UserName || strings.ContainsAny(observation.UserName, " /\\\x00\r\n\t") {
		return "", fmt.Errorf("%w: user name is required", ErrInvalidAppsObservation)
	}
	return home, nil
}

func resolveAppsPins(observation AppsObservation, packages []string) (map[string]catalog.AURLocalPin, error) {
	pins := observation.AURPins
	if pins == nil && observation.Catalog != nil && observation.Catalog.Pins != nil {
		pins = observation.Catalog.Pins.AURLocal
	}
	result := make(map[string]catalog.AURLocalPin, len(packages))
	for _, name := range packages {
		pin, ok := pins[name]
		if !ok {
			return nil, fmt.Errorf("%w: missing AUR/local pin for %q", ErrInvalidAppsObservation, name)
		}
		if err := catalog.ValidateAURLocalPin(name, pin); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidAppsObservation, err)
		}
		result[name] = pin
	}
	return result, nil
}

func missingPackages(want []string, installed map[string]string) []string {
	missing := make([]string, 0)
	for _, name := range want {
		if !hasPackage(installed, name) {
			missing = append(missing, name)
		}
	}
	return missing
}

func addPinnedAURInstall(result *AppsRequestPlan, observation AppsObservation, home, name string, pin catalog.AURLocalPin) error {
	prefix := "apps.aur." + name
	sourceDir := filepath.Join(home, ".cache", "alex-cachyos", "aur", name)
	patchName := ".alex-cachyos-source.patch"
	patchPath := filepath.Join(sourceDir, patchName)
	remote := "https://aur.archlinux.org/" + name + ".git"
	observed := observation.AURSources[name]
	if observed.Exists && observed.Remote != "" && observed.Remote != remote {
		return fmt.Errorf("%w: AUR source %q has unexpected remote", ErrInvalidAppsObservation, name)
	}

	var fetch runner.CommandRequest
	var err error
	if observed.Exists {
		fetch, err = makeAppsRequestAt(prefix+".fetch", "/usr/bin/git", []string{"-C", sourceDir, "fetch", "origin", pin.SourceCommit}, home, runner.NetworkRequired, nil)
	} else {
		fetch, err = makeAppsRequestAt(prefix+".fetch", "/usr/bin/git", []string{"clone", "--filter=blob:none", "--no-checkout", remote, sourceDir}, home, runner.NetworkRequired, nil)
	}
	if err != nil {
		return err
	}
	result.Requests = append(result.Requests, fetch)
	result.Steps = append(result.Steps, moduleRequestStep(AppsModuleName, fetch, planner.DispositionApply,
		map[string]any{"package": name, "remote": remote, "sourceCommit": pin.SourceCommit, "sourceDir": sourceDir},
		map[string]any{"exists": observed.Exists, "remote": observed.Remote, "commit": observed.Commit}, prefix+".source.restore"))

	checkout, err := makeAppsRequestAt(prefix+".checkout", "/usr/bin/git", []string{"-C", sourceDir, "checkout", "--detach", pin.SourceCommit}, home, runner.NetworkNone, nil)
	if err != nil {
		return err
	}
	checkoutStep := moduleRequestStep(AppsModuleName, checkout, convergedDisposition(observed.Commit == pin.SourceCommit),
		map[string]any{"package": name, "sourceCommit": pin.SourceCommit, "sourceDir": sourceDir},
		map[string]any{"commit": observed.Commit}, prefix+".checkout.restore")
	checkoutStep.DependsOn = []string{prefix + ".fetch"}
	result.Steps = append(result.Steps, checkoutStep)
	if observed.Commit != pin.SourceCommit {
		result.Requests = append(result.Requests, checkout)
	}

	materialize, err := makeAppsRequestAt(prefix+".patch.materialize", "/usr/bin/git", []string{
		"-C", sourceDir, "diff-tree", "--root", "--no-commit-id", "--binary", "-p", "--output=" + patchPath, pin.SourceCommit,
	}, home, runner.NetworkNone, nil)
	if err != nil {
		return err
	}
	patchConverged := observed.Commit == pin.SourceCommit && strings.EqualFold(observed.PatchSHA256, pin.PatchSHA256)
	materializeStep := moduleRequestStep(AppsModuleName, materialize, convergedDisposition(patchConverged),
		map[string]any{"package": name, "sourceCommit": pin.SourceCommit, "patchPath": patchPath, "patchSHA256": pin.PatchSHA256},
		map[string]any{"patchSHA256": observed.PatchSHA256}, prefix+".patch.remove")
	materializeStep.DependsOn = []string{prefix + ".checkout"}
	result.Steps = append(result.Steps, materializeStep)
	if !patchConverged {
		result.Requests = append(result.Requests, materialize)
	}

	verify, err := makeAppsRequestAt(prefix+".patch.verify", "/usr/bin/sha256sum", []string{"--check", "--strict", "-"}, sourceDir, runner.NetworkNone, []byte(pin.PatchSHA256+"  "+patchName+"\n"))
	if err != nil {
		return err
	}
	verifyStep := moduleRequestStep(AppsModuleName, verify, convergedDisposition(patchConverged),
		map[string]any{"package": name, "patchPath": patchPath, "patchSHA256": pin.PatchSHA256},
		map[string]any{"patchSHA256": observed.PatchSHA256}, prefix+".patch.verify")
	verifyStep.DependsOn = []string{prefix + ".patch.materialize"}
	result.Steps = append(result.Steps, verifyStep)
	if !patchConverged {
		result.Requests = append(result.Requests, verify)
	}

	install, err := makeAppsRequestAt(prefix+".install", "/usr/bin/paru", []string{"-B", "--install", "--needed", "--noconfirm", sourceDir}, home, runner.NetworkRequired, nil)
	if err != nil {
		return err
	}
	installStep := moduleRequestStep(AppsModuleName, install, planner.DispositionApply,
		map[string]any{"package": name, "sourceDir": sourceDir, "sourceCommit": pin.SourceCommit, "patchSHA256": pin.PatchSHA256, "packageManager": "paru-local-build"},
		map[string]any{"beforeVersion": observation.InstalledPackages[name]}, prefix+".remove")
	installStep.DependsOn = []string{prefix + ".patch.verify"}
	result.Steps = append(result.Steps, installStep)
	result.Requests = append(result.Requests, install)
	return nil
}

func makeAppsRequestAt(operation, executable string, argv []string, cwd string, network runner.NetworkPolicy, stdin []byte) (runner.CommandRequest, error) {
	request, err := makeRequest(operation, executable, argv, runner.ScopeUser, network, stdin)
	if err != nil {
		return runner.CommandRequest{}, err
	}
	request.Cwd = cwd
	if err := runner.ValidateCommandRequest(request); err != nil {
		return runner.CommandRequest{}, err
	}
	return request, nil
}

func appsSatisfiedAURStep(name string, pin catalog.AURLocalPin, version string) planner.Step {
	id := "apps.aur." + name + ".install"
	return planner.Step{
		ID: id, Module: AppsModuleName, Description: "install pinned AUR/local package " + name,
		Scope: planner.ScopeUser, Network: planner.NetworkRequired, Operation: planner.Operation(id), Disposition: planner.DispositionSatisfied,
		Desired:  mustJSON(map[string]any{"package": name, "sourceCommit": pin.SourceCommit, "patchSHA256": pin.PatchSHA256, "packageManager": "paru-local-build"}),
		Observed: mustJSON(map[string]any{"version": version}),
		Inverse:  &planner.InverseDescriptor{Operation: planner.Operation("apps.aur." + name + ".remove"), Value: mustJSON(map[string]any{"beforeVersion": version})},
	}
}

func appsPackageStep(request runner.CommandRequest, names []string, installed map[string]string, pins map[string]catalog.AURLocalPin) planner.Step {
	desired := map[string]any{"requestedNames": append([]string(nil), names...), "repositoryPolicy": BootstrapPacmanRepositoryPolicy, "transactionPolicy": "name-presence"}
	if pins != nil {
		desired["aurLocalPins"] = pins
		desired["packageManager"] = "paru"
	}
	return moduleRequestStep(AppsModuleName, request, planner.DispositionApply, desired, map[string]any{"beforeVersions": versionSubset(installed, names)}, request.Operation+".restore")
}

func appsSatisfiedPackageStep(id string, scope runner.Scope, names []string, installed map[string]string, pins map[string]catalog.AURLocalPin) planner.Step {
	desired := map[string]any{"requestedNames": append([]string(nil), names...), "repositoryPolicy": BootstrapPacmanRepositoryPolicy, "transactionPolicy": "name-presence"}
	if pins != nil {
		desired["aurLocalPins"] = pins
		desired["packageManager"] = "paru"
	}
	return planner.Step{ID: id, Module: AppsModuleName, Description: id, Scope: planner.Scope(scope), Network: planner.NetworkRequired, Operation: planner.Operation(id), Disposition: planner.DispositionSatisfied, Desired: mustJSON(desired), Observed: mustJSON(map[string]any{"versions": versionSubset(installed, names)}), Inverse: &planner.InverseDescriptor{Operation: planner.Operation(id + ".restore"), Value: mustJSON(map[string]any{"beforeVersions": versionSubset(installed, names)})}}
}

func addAppsService(result *AppsRequestPlan, observation AppsObservation, unit, dependency string) error {
	state := observation.Services[unit]
	id := "apps.service." + strings.TrimSuffix(unit, ".service") + ".enable"
	converged := state.Installed && state.Enabled && state.Active
	request, err := makeRequest(id, "/usr/bin/systemctl", []string{"enable", "--now", unit}, runner.ScopeSystem, runner.NetworkNone, nil)
	if err != nil {
		return err
	}
	step := moduleRequestStep(AppsModuleName, request, convergedDisposition(converged), map[string]any{"unit": unit, "enabled": true, "active": true}, map[string]any{"installed": state.Installed, "enabled": state.Enabled, "active": state.Active}, "apps.service.restore")
	step.DependsOn = []string{dependency}
	result.Steps = append(result.Steps, step)
	if !converged {
		result.Requests = append(result.Requests, request)
	}
	return nil
}

func addAppsGroup(result *AppsRequestPlan, observation AppsObservation, group, dependency string) error {
	id := "apps.group." + group + ".add"
	converged := observation.Groups[group]
	request, err := makeRequest(id, "/usr/bin/usermod", []string{"-aG", group, observation.UserName}, runner.ScopeSystem, runner.NetworkNone, nil)
	if err != nil {
		return err
	}
	step := moduleRequestStep(AppsModuleName, request, convergedDisposition(converged), map[string]any{"group": group, "user": observation.UserName, "member": true}, map[string]any{"member": converged}, "apps.group.restore")
	step.DependsOn = []string{dependency}
	result.Steps = append(result.Steps, step)
	if !converged {
		result.Requests = append(result.Requests, request)
	}
	return nil
}

func addDockerDesktopDisable(result *AppsRequestPlan, observation AppsObservation) error {
	id := "apps.service.docker-desktop.disable"
	request, err := makeRequest(id, "/usr/bin/systemctl", []string{"--user", "disable", "--now", "docker-desktop.service"}, runner.ScopeUser, runner.NetworkNone, nil)
	if err != nil {
		return err
	}
	step := moduleRequestStep(AppsModuleName, request, convergedDisposition(observation.DockerDesktopDisabled), map[string]any{"unit": "docker-desktop.service", "enabled": false, "active": false}, map[string]any{"disabled": observation.DockerDesktopDisabled}, "apps.service.restore")
	step.DependsOn = []string{"apps.aur.docker-desktop.install"}
	result.Steps = append(result.Steps, step)
	if !observation.DockerDesktopDisabled {
		result.Requests = append(result.Requests, request)
	}
	return nil
}

func appsFileStepID(webApps WebAppPlan, file ManagedFileDescriptor) string {
	if file.Path == webApps.Launcher.Path {
		return "apps.webapp.launcher"
	}
	for _, install := range webApps.Installs {
		switch file.Path {
		case install.Desktop.Path:
			return "apps.webapp." + install.BaseName + ".desktop"
		case install.Icon.Path:
			return "apps.webapp." + install.BaseName + ".icon"
		}
	}
	return "apps.webapp.unknown"
}

func appsStepRank(id string) int {
	switch {
	case id == "apps.packages.pacman.install":
		return 0
	case strings.HasPrefix(id, "apps.aur.") && strings.HasSuffix(id, ".fetch"):
		return 1
	case strings.HasPrefix(id, "apps.aur.") && strings.HasSuffix(id, ".checkout"):
		return 2
	case strings.HasPrefix(id, "apps.aur.") && strings.HasSuffix(id, ".patch.materialize"):
		return 3
	case strings.HasPrefix(id, "apps.aur.") && strings.HasSuffix(id, ".patch.verify"):
		return 4
	case strings.HasPrefix(id, "apps.aur.") && strings.HasSuffix(id, ".install"):
		return 5
	case id == "apps.packages.explicit":
		return 6
	case strings.HasPrefix(id, "apps.service."):
		return 7
	case strings.HasPrefix(id, "apps.group."):
		return 8
	case id == "apps.webapp.launcher":
		return 9
	case strings.HasSuffix(id, ".desktop"):
		return 10
	case strings.HasSuffix(id, ".icon"):
		return 11
	default:
		return 12
	}
}

func existingAppsDependencies(steps []planner.Step, ids ...string) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		for _, step := range steps {
			if step.ID == id {
				result = append(result, id)
				break
			}
		}
	}
	return result
}

func explicitSubset(values map[string]bool, names []string) []string {
	result := make([]string, 0)
	for _, name := range names {
		if values[name] {
			result = append(result, name)
		}
	}
	return result
}

func pinSubset(values map[string]catalog.AURLocalPin, names []string) map[string]catalog.AURLocalPin {
	result := make(map[string]catalog.AURLocalPin, len(names))
	for _, name := range names {
		result[name] = values[name]
	}
	return result
}

func copyAURPins(values map[string]catalog.AURLocalPin) map[string]catalog.AURLocalPin {
	result := make(map[string]catalog.AURLocalPin, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func fileSHA256(content []byte) string {
	return sha256Hex(content)
}
