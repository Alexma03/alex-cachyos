package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
	"alex-cachyos/templates"
)

const (
	DesktopModuleName            = "desktop"
	DesktopPackagesOperation     = "desktop.packages.install"
	DesktopManagedFileOperation  = "managed-file.publish"
	DesktopSessionOperation      = "desktop.session.dmrc"
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

var workstationFileOperations = map[string]struct {
	operation string
	relative  string
	kind      string
}{
	"templates/roles/workstation/hyprwhspr/config.json":  {"desktop.file.hyprwhspr", ".config/hyprwhspr/config.json", "hyprwhspr-config"},
	"templates/roles/workstation/niri/config.kdl":        {"desktop.file.niri", ".config/niri/config.kdl", "niri-config"},
	"templates/roles/workstation/noctalia/settings.toml": {"desktop.file.noctalia", ".local/state/noctalia/settings.toml", "noctalia-config"},
}

// DesktopFileObservation is a bounded desired-file observation. Content is
// represented only by SHA-256; live bytes never enter planner or receipt data.
type DesktopFileObservation struct {
	Path       string
	Exists     bool
	SHA256     string
	Mode       uint32
	Ownership  FileOwnershipKind
	BackupPath string
}

// DesktopObservation contains only values read by a caller-owned OS adapter.
// HomeRoot and UserName are runtime identity, never catalog authority.
type DesktopObservation struct {
	HomeRoot          string
	UserName          string
	InstalledPackages map[string]string
	Files             map[string]DesktopFileObservation
}

// DesktopFilePlan binds one stable planner operation to its desired file.
type DesktopFilePlan struct {
	Operation string
	File      ManagedFileDescriptor
}

// DesktopRequestPlan is the complete executable workstation bundle. Callers
// can pass Requests and Files to production ports without recovering bytes or
// argv from planner metadata.
type DesktopRequestPlan struct {
	Packages []string
	Requests []runner.CommandRequest
	Files    []DesktopFilePlan
	Steps    []planner.Step
}

// BuildDesktopRequestPlan maps the workstation role to typed package and file
// operations. Host-owned fragments remain in the separately gated factories.
func BuildDesktopRequestPlan(policy catalog.ResolvedHostPolicy, observation DesktopObservation) (DesktopRequestPlan, error) {
	home, err := validateDesktopIdentity(observation.HomeRoot, observation.UserName)
	if err != nil {
		return DesktopRequestPlan{}, err
	}
	if !declaresRole(policy, "workstation") {
		return DesktopRequestPlan{}, fmt.Errorf("host %q does not declare workstation role", policy.Name)
	}

	packages, err := embeddedPackageList("desktop/packages.pacman")
	if err != nil {
		return DesktopRequestPlan{}, err
	}
	request, err := makeRequest(DesktopPackagesOperation, "/usr/bin/pacman", append([]string{"-S", "--needed", "--noconfirm"}, packages...), runner.ScopeSystem, runner.NetworkRequired, nil)
	if err != nil {
		return DesktopRequestPlan{}, err
	}
	missing := missingDesktopPackages(packages, observation.InstalledPackages)
	packageStep := moduleRequestStep(DesktopModuleName, request, convergedDisposition(len(missing) == 0),
		map[string]any{"packages": packages, "transactionPolicy": "repository-name-presence"},
		map[string]any{"installedVersions": versionSubset(observation.InstalledPackages, packages)},
		"desktop.packages.retain")
	result := DesktopRequestPlan{Packages: append([]string(nil), packages...), Steps: []planner.Step{packageStep}}
	if len(missing) != 0 {
		result.Requests = append(result.Requests, request)
	}

	roleAssets := ownedRoleTemplates(policy, "workstation")
	seen := make(map[string]bool, len(roleAssets))
	for _, asset := range roleAssets {
		definition, ok := workstationFileOperations[asset]
		if !ok {
			return DesktopRequestPlan{}, fmt.Errorf("host %q workstation role contains unsupported asset %q", policy.Name, asset)
		}
		data, readErr := fs.ReadFile(templates.FS, strings.TrimPrefix(asset, "templates/"))
		if readErr != nil {
			return DesktopRequestPlan{}, fmt.Errorf("read workstation asset %q: %w", asset, readErr)
		}
		result.Files = append(result.Files, DesktopFilePlan{Operation: definition.operation, File: newManagedFile(filepath.Join(home, filepath.FromSlash(definition.relative)), data, 0o644, definition.kind)})
		seen[asset] = true
	}
	for _, required := range workstationAssets {
		if !seen[required] {
			return DesktopRequestPlan{}, fmt.Errorf("host %q workstation role is missing required asset %q", policy.Name, required)
		}
	}
	for _, generic := range []struct {
		asset, operation, relative, kind string
	}{
		{"quickshell-polkit/PolkitModel.js", "desktop.polkit.model", ".config/quickshell/polkit/PolkitModel.js", "polkit-model"},
		{"quickshell-polkit/shell.qml", "desktop.polkit.shell", ".config/quickshell/polkit/shell.qml", "polkit-shell"},
	} {
		data, readErr := fs.ReadFile(templates.FS, generic.asset)
		if readErr != nil {
			return DesktopRequestPlan{}, fmt.Errorf("read desktop asset %q: %w", generic.asset, readErr)
		}
		result.Files = append(result.Files, DesktopFilePlan{Operation: generic.operation, File: newManagedFile(filepath.Join(home, filepath.FromSlash(generic.relative)), data, 0o644, generic.kind)})
	}
	result.Files = append(result.Files, DesktopFilePlan{Operation: DesktopSessionOperation, File: newManagedFile(filepath.Join(home, ".dmrc"), []byte("[Desktop]\nSession=niri\n"), 0o600, "default-session")})
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Operation < result.Files[j].Operation })

	fileSteps := make([]planner.Step, 0, len(result.Files))
	for _, item := range result.Files {
		observed := observation.Files[item.File.Path]
		step, stepErr := managedFileStep(DesktopModuleName, item.Operation, item.File, DevtoolsFileObservation{
			Path: observed.Path, Exists: observed.Exists, SHA256: observed.SHA256, Mode: observed.Mode,
			Ownership: observed.Ownership, BackupPath: observed.BackupPath,
		}, []string{DesktopPackagesOperation})
		if stepErr != nil {
			return DesktopRequestPlan{}, stepErr
		}
		step.Operation = planner.Operation(DesktopManagedFileOperation)
		fileSteps = append(fileSteps, step)
	}
	// The user-session selector is the final base operation. Risky host-specific
	// desktop steps depend on it and therefore cannot precede the usable target.
	for i := range fileSteps {
		if fileSteps[i].ID != DesktopSessionOperation {
			continue
		}
		for _, other := range fileSteps {
			if other.ID != DesktopSessionOperation {
				fileSteps[i].DependsOn = append(fileSteps[i].DependsOn, other.ID)
			}
		}
		sort.Strings(fileSteps[i].DependsOn)
	}
	result.Steps = append(result.Steps, fileSteps...)
	return cloneDesktopRequestPlan(result), nil
}

func buildDesktopModule(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) (planner.Module, error) {
	module := planner.Module{Name: DesktopModuleName, Enabled: moduleEnabled(policy, DesktopModuleName)}
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
	workstation, err := BuildDesktopRequestPlan(policy, evidence.Desktop)
	if err != nil {
		return planner.Module{}, err
	}
	module.Steps = append(module.Steps, workstation.Steps...)

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
			DependsOn:   []string{DesktopSessionOperation},
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
				DependsOn:   []string{DesktopSessionOperation},
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

func validateDesktopIdentity(home, user string) (string, error) {
	if !canonicalDesktopRoot(home) {
		return "", fmt.Errorf("invalid desktop observation: home root must be canonical and absolute")
	}
	if user == "" || strings.TrimSpace(user) != user || strings.ContainsAny(user, " /\\\x00\r\n\t") {
		return "", fmt.Errorf("invalid desktop observation: user name is required")
	}
	return home, nil
}

func missingDesktopPackages(packages []string, installed map[string]string) []string {
	missing := make([]string, 0, len(packages))
	for _, name := range packages {
		if installed[name] == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func desktopObservationForDescriptor(file ManagedFileDescriptor) DesktopFileObservation {
	digest := sha256.Sum256(file.Content)
	return DesktopFileObservation{Path: file.Path, Exists: true, SHA256: hex.EncodeToString(digest[:]), Mode: file.Mode, Ownership: OwnershipCreated}
}

func cloneDesktopRequestPlan(source DesktopRequestPlan) DesktopRequestPlan {
	result := DesktopRequestPlan{Packages: append([]string(nil), source.Packages...), Requests: cloneVicinaeRequests(source.Requests), Steps: cloneVicinaeSteps(source.Steps)}
	result.Files = make([]DesktopFilePlan, len(source.Files))
	for i, item := range source.Files {
		result.Files[i] = DesktopFilePlan{Operation: item.Operation, File: item.File.Clone()}
	}
	return result
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
