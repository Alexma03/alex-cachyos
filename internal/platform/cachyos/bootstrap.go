package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const (
	BootstrapPacmanRepositoryPolicy  = "configured-cachyos-arch"
	BootstrapPacmanTransactionPolicy = "full-system"

	bootstrapModuleName       = "bootstrap"
	bootstrapPackageInstall   = "bootstrap.packages.install"
	bootstrapPackageRemove    = "bootstrap.packages.remove"
	bootstrapPackageExplicit  = "bootstrap.packages.explicit"
	bootstrapPackageRollback  = "bootstrap.packages.rollback.external-system"
	bootstrapBootPlymouthEdit = "bootstrap.boot.plymouth-edit"
	bootstrapBootMkinitcpio   = "bootstrap.boot.mkinitcpio"
	bootstrapBootGRUBEdit     = "bootstrap.boot.grub-edit"
	bootstrapBootGRUBGenerate = "bootstrap.boot.grub-generate"
	bootstrapBootGRUBPublish  = "bootstrap.boot.grub-publish"
	bootstrapChromeInstall    = "bootstrap.chrome.install"
	bootstrapZsh              = "bootstrap.zsh"
	bootstrapLTS              = "bootstrap.kernel.lts"
)

const (
	mkinitcpioConfigPath = "/etc/mkinitcpio.conf"
	grubDefaultPath      = "/etc/default/grub"
)

// BootstrapObservation is the read-only host state used to produce a bootstrap
// module. InstalledPackages is a package-name-to-version snapshot; ExplicitSet
// is the set of packages Pacman currently marks as explicit.
type BootstrapObservation struct {
	InstalledPackages map[string]string
	ExplicitSet       []string
	Services          []ServiceObservation
	Boot              BootObservation
	ChromeInstalled   bool
	ZshConverged      bool
}

// BuildBootstrapModule maps each actionable request-kernel identity to one
// planner step and adds typed steps that have no command request yet (LTS
// deferral, GRUB publication, and the zsh marker-block rewrite).
func BuildBootstrapModule(observation BootstrapObservation) (planner.Module, error) {
	input, installed, explicit, err := requestInput(observation)
	if err != nil {
		return planner.Module{}, err
	}

	requestPlan, err := BuildBootstrapRequestPlan(input)
	if err != nil {
		return planner.Module{}, err
	}

	steps := make([]planner.Step, 0, len(requestPlan.Requests)+3)
	seenRequests := make(map[string]bool, len(requestPlan.Requests))
	for _, request := range requestPlan.Requests {
		// The request kernel intentionally describes the complete package
		// transaction even when its requested-name list is empty. An empty
		// transaction is not an actionable planner step.
		if request.Operation == bootstrapPackageInstall && len(requestPlan.MissingWanted) == 0 {
			continue
		}
		if seenRequests[request.Operation] {
			return planner.Module{}, fmt.Errorf("duplicate bootstrap request identity: %q", request.Operation)
		}
		seenRequests[request.Operation] = true
		steps = append(steps, requestStep(request, requestPlan, installed, explicit, input))
	}

	if requestPlan.GRUBPublish != nil {
		steps = append(steps, grubPublishStep(*requestPlan.GRUBPublish, input.Boot))
	}
	steps = append(steps, ltsStep(installed), zshStep(observation.ZshConverged))

	wireBootstrapDependencies(steps)
	sort.SliceStable(steps, func(i, j int) bool {
		leftRank := bootstrapStepRank(steps[i].ID)
		rightRank := bootstrapStepRank(steps[j].ID)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return steps[i].ID < steps[j].ID
	})

	return planner.Module{Name: bootstrapModuleName, Enabled: true, Steps: steps}, nil
}

// BuildBootstrapPlan delegates final ordering and defensive copying to the
// shared planner while selecting only this module.
func BuildBootstrapPlan(observation BootstrapObservation) (planner.Plan, error) {
	module, err := BuildBootstrapModule(observation)
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.BuildPlan([]planner.Module{module}, planner.Selection{Only: []string{bootstrapModuleName}})
}

// requestInput translates the richer observation into the request kernel's
// committed input types without exposing any source maps or slices as output.
func requestInput(observation BootstrapObservation) (BootstrapInputs, map[string]string, map[string]bool, error) {
	installed := copyPackages(observation.InstalledPackages)
	explicit := make(map[string]bool, len(observation.ExplicitSet))
	for _, name := range observation.ExplicitSet {
		explicit[name] = true
	}

	input := BootstrapInputs{Boot: observation.Boot, ChromeInstalled: observation.ChromeInstalled || hasPackage(installed, "google-chrome")}
	for _, name := range sortedPackageNames(installed) {
		input.InstalledPackages = append(input.InstalledPackages, InstalledPackage{
			Name:     name,
			Explicit: explicit[name],
		})
		if strings.HasPrefix(name, "firefox-i18n-") {
			input.FirefoxI18N = append(input.FirefoxI18N, name)
		}
	}
	services, err := normalizedServices(observation.Services, installed)
	if err != nil {
		return BootstrapInputs{}, nil, nil, err
	}
	input.Services = services
	return input, installed, explicit, nil
}

func normalizedServices(observed []ServiceObservation, installed map[string]string) ([]ServiceObservation, error) {
	present := make(map[string]bool, len(installed))
	for name := range installed {
		present[name] = true
	}
	return normalizeServiceObservations(observed, present)
}

func requestStep(
	request runner.CommandRequest,
	requestPlan BootstrapRequestPlan,
	installed map[string]string,
	explicit map[string]bool,
	input BootstrapInputs,
) planner.Step {
	desired := map[string]any{
		"request": commandIdentity(request),
	}
	observed := make(map[string]any)
	inverse := make(map[string]any)
	inverseOperation := request.Operation + ".inverse"

	switch request.Operation {
	case bootstrapPackageInstall:
		names := append([]string(nil), requestPlan.MissingWanted...)
		desired["pacmanRepositoryPolicy"] = BootstrapPacmanRepositoryPolicy
		desired["pacmanTransactionPolicy"] = BootstrapPacmanTransactionPolicy
		desired["requestedNames"] = names
		desired["versionPolicy"] = map[string]any{
			"repository":  BootstrapPacmanRepositoryPolicy,
			"transaction": BootstrapPacmanTransactionPolicy,
		}
		observed["beforeVersions"] = versionSnapshot(installed)
		inverseOperation = bootstrapPackageRollback
		inverse["rollbackPolicy"] = map[string]any{
			"type":     "external-system",
			"provider": "snapper",
			"scope":    "system",
		}
		inverse["pacmanRepositoryPolicy"] = BootstrapPacmanRepositoryPolicy
		inverse["pacmanTransactionPolicy"] = BootstrapPacmanTransactionPolicy
	case bootstrapPackageRemove:
		names := append([]string(nil), requestPlan.RemovalDelta...)
		desired["pacmanRepositoryPolicy"] = BootstrapPacmanRepositoryPolicy
		desired["pacmanTransactionPolicy"] = "remove-installed"
		desired["requestedNames"] = names
		desired["versionPolicy"] = map[string]any{
			"repository":  BootstrapPacmanRepositoryPolicy,
			"transaction": "remove-installed",
		}
		observed["beforeVersions"] = versionSubset(installed, names)
		inverseOperation = bootstrapPackageInstall
		inverse["requestedNames"] = names
	case bootstrapPackageExplicit:
		names := append([]string(nil), requestPlan.ExplicitDelta...)
		desired["pacmanRepositoryPolicy"] = BootstrapPacmanRepositoryPolicy
		desired["pacmanTransactionPolicy"] = "explicit-mark"
		desired["requestedNames"] = names
		desired["explicitOwnership"] = "explicit"
		desired["versionPolicy"] = map[string]any{
			"repository":  BootstrapPacmanRepositoryPolicy,
			"transaction": "explicit-mark",
		}
		observed["beforeExplicit"] = explicitInstalled(installed, explicit)
		observed["beforeVersions"] = versionSubset(installed, names)
		inverseOperation = "bootstrap.packages.explicit-restore"
		inverse["requestedNames"] = names
	case bootstrapBootPlymouthEdit:
		desired["targetPath"] = mkinitcpioConfigPath
		desired["change"] = "remove-plymouth"
		observed["mkinitcpioHasPlymouth"] = input.Boot.MkinitcpioHasPlymouth
		inverseOperation = "bootstrap.boot.restore-mkinitcpio"
	case bootstrapBootMkinitcpio:
		desired["targetPath"] = mkinitcpioConfigPath
		desired["change"] = "regenerate-mkinitcpio"
		observed["mkinitcpioHasPlymouth"] = input.Boot.MkinitcpioHasPlymouth
		inverseOperation = "bootstrap.boot.restore-mkinitcpio"
	case bootstrapBootGRUBEdit:
		desired["targetPath"] = grubDefaultPath
		desired["change"] = "remove-splash"
		observed["grubHasSplash"] = input.Boot.GrubHasSplash
		inverseOperation = "bootstrap.boot.restore-grub-defaults"
	case bootstrapBootGRUBGenerate:
		desired["stagedPath"] = grubStagePath
		desired["change"] = "generate-staged-grub"
		observed["grubGeneratorAvailable"] = input.Boot.GrubGeneratorAvailable
		inverseOperation = "bootstrap.boot.grub-restore"
	case bootstrapChromeInstall:
		desired["requestedNames"] = []string{"google-chrome"}
		desired["packageManager"] = "paru"
		observed["beforeVersions"] = versionSubset(installed, []string{"google-chrome"})
		inverseOperation = "bootstrap.chrome.remove"
	default:
		if strings.HasPrefix(request.Operation, "bootstrap.service.") {
			unit := ""
			if len(request.Argv) != 0 {
				unit = request.Argv[len(request.Argv)-1]
			}
			state := serviceState(input.Services, unit)
			desired["unit"] = unit
			desired["packageName"] = servicePackageName(unit)
			desired["installed"] = true
			desired["enabled"] = true
			desired["active"] = true
			observed["unit"] = unit
			observed["installed"] = state.Installed
			observed["enabled"] = state.Enabled
			observed["active"] = state.Active
			inverseOperation = "bootstrap.service.restore"
		}
	}

	if before, ok := observed["beforeVersions"]; ok {
		inverse["beforeVersions"] = before
	}
	inverse["before"] = observed
	return makeStep(
		request.Operation,
		"bootstrap "+request.Operation,
		planner.Scope(request.Scope),
		planner.NetworkClass(request.Network),
		request.Operation,
		planner.DispositionApply,
		desired,
		observed,
		inverseOperation,
		inverse,
	)
}

func grubPublishStep(descriptor GRUBPublishDescriptor, boot BootObservation) planner.Step {
	desired := map[string]any{
		"sourcePath":        descriptor.SourcePath,
		"destinationPath":   descriptor.DestinationPath,
		"sameDirectory":     descriptor.SameDirectory,
		"noDirectOverwrite": descriptor.NoDirectOverwrite,
		"mode":              descriptor.Mode,
		"atomic":            true,
		"generationFailure": descriptor.GenerationFailure,
		"fallback":          descriptor.Fallback,
	}
	observed := map[string]any{
		"grubHasSplash":      boot.GrubHasSplash,
		"generatorAvailable": descriptor.GeneratorAvailable,
	}
	disposition := planner.DispositionApply
	if !descriptor.GeneratorAvailable {
		disposition = planner.DispositionBlocked
	}
	return makeStep(
		bootstrapBootGRUBPublish,
		"publish generated GRUB configuration atomically",
		planner.ScopeSystem,
		planner.NetworkNone,
		bootstrapBootGRUBPublish,
		disposition,
		desired,
		observed,
		"bootstrap.boot.grub-restore",
		map[string]any{
			"destinationPath": descriptor.DestinationPath,
			"fallback":        descriptor.Fallback,
		},
	)
}

func ltsStep(installed map[string]string) planner.Step {
	names := make([]string, 0, len(cachyosLTSNames))
	for _, name := range cachyosLTSNames {
		if _, ok := installed[name]; ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	disposition := planner.DispositionSatisfied
	if len(names) != 0 {
		disposition = planner.DispositionBlocked
	}
	return makeStep(
		bootstrapLTS,
		"defer installed CachyOS LTS kernel removal",
		planner.ScopeSystem,
		planner.NetworkNone,
		"bootstrap.kernel.lts-deferred",
		disposition,
		map[string]any{
			"packages": append([]string(nil), cachyosLTSNames...),
			"policy":   "defer-removal",
		},
		map[string]any{"beforeVersions": versionSubset(installed, names)},
		"bootstrap.kernel.lts-deferred",
		map[string]any{"policy": "leave-installed", "packages": append([]string(nil), names...)},
	)
}

func zshStep(converged bool) planner.Step {
	return makeStep(
		bootstrapZsh,
		"rewrite the managed zsh marker block",
		planner.ScopeUser,
		planner.NetworkNone,
		bootstrapZsh,
		convergedDisposition(converged),
		map[string]any{
			"path":        "~/.zshrc",
			"markerBegin": "# >>> alex-cachyos/devtools >>>",
			"markerEnd":   "# <<< alex-cachyos/devtools <<<",
			"converged":   true,
		},
		map[string]any{"converged": converged},
		"bootstrap.zsh.restore",
		map[string]any{
			"path":         "~/.zshrc",
			"precondition": "recorded-after-hash",
		},
	)
}

func makeStep(
	id string,
	description string,
	scope planner.Scope,
	network planner.NetworkClass,
	operation string,
	disposition planner.Disposition,
	desired any,
	observed any,
	inverseOperation string,
	inverse any,
) planner.Step {
	return planner.Step{
		ID:          id,
		Module:      bootstrapModuleName,
		Description: description,
		Scope:       scope,
		Network:     network,
		Operation:   planner.Operation(operation),
		Disposition: disposition,
		Desired:     mustJSON(desired),
		Observed:    mustJSON(observed),
		Inverse: &planner.InverseDescriptor{
			Operation: planner.Operation(inverseOperation),
			Value:     mustJSON(inverse),
		},
	}
}

func wireBootstrapDependencies(steps []planner.Step) {
	indices := make(map[string]int, len(steps))
	for i := range steps {
		indices[steps[i].ID] = i
	}

	add := func(id, dependency string) {
		index, ok := indices[id]
		if !ok || dependency == "" || id == dependency {
			return
		}
		if _, ok := indices[dependency]; !ok || containsString(steps[index].DependsOn, dependency) {
			return
		}
		steps[index].DependsOn = append(steps[index].DependsOn, dependency)
	}

	add(bootstrapPackageRemove, bootstrapPackageInstall)
	if _, ok := indices[bootstrapPackageRemove]; ok {
		add(bootstrapPackageExplicit, bootstrapPackageRemove)
	} else {
		add(bootstrapPackageExplicit, bootstrapPackageInstall)
	}

	add(bootstrapBootMkinitcpio, bootstrapBootPlymouthEdit)
	add(bootstrapBootGRUBGenerate, bootstrapBootGRUBEdit)
	if _, ok := indices[bootstrapBootGRUBGenerate]; ok {
		add(bootstrapBootGRUBPublish, bootstrapBootGRUBGenerate)
	} else {
		add(bootstrapBootGRUBPublish, bootstrapBootGRUBEdit)
	}

	lastPackageStep := ""
	for _, id := range []string{bootstrapPackageInstall, bootstrapPackageRemove, bootstrapPackageExplicit} {
		if _, ok := indices[id]; ok {
			lastPackageStep = id
		}
	}
	for _, id := range []string{"bootstrap.service.ananicy-cpp", "bootstrap.service.ufw", bootstrapChromeInstall} {
		add(id, lastPackageStep)
	}
	if _, ok := indices[bootstrapChromeInstall]; ok {
		add(bootstrapZsh, bootstrapChromeInstall)
	} else {
		add(bootstrapZsh, lastPackageStep)
	}
}

var bootstrapRanks = map[string]int{
	bootstrapLTS:              0,
	bootstrapPackageInstall:   1,
	bootstrapPackageRemove:    2,
	bootstrapPackageExplicit:  3,
	bootstrapBootPlymouthEdit: 4,
	bootstrapBootMkinitcpio:   5,
	bootstrapBootGRUBEdit:     6,
	bootstrapBootGRUBGenerate: 7,
	bootstrapBootGRUBPublish:  8,
}

func bootstrapStepRank(id string) int {
	if rank, ok := bootstrapRanks[id]; ok {
		return rank
	}
	if strings.HasPrefix(id, "bootstrap.service.") {
		return 20
	}
	if id == bootstrapChromeInstall {
		return 30
	}
	if id == bootstrapZsh {
		return 40
	}
	return 50
}

func normalizeUnit(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit != "" && !strings.HasSuffix(unit, ".service") {
		return unit + ".service"
	}
	return unit
}

func serviceState(services []ServiceObservation, unit string) ServiceObservation {
	for _, service := range services {
		if service.Unit == unit {
			return service
		}
	}
	return ServiceObservation{Unit: unit}
}

func servicePackageName(unit string) string {
	switch normalizeUnit(unit) {
	case "ananicy-cpp.service":
		return "ananicy-cpp"
	case "ufw.service":
		return "ufw"
	default:
		return ""
	}
}

func convergedDisposition(converged bool) planner.Disposition {
	if converged {
		return planner.DispositionSatisfied
	}
	return planner.DispositionApply
}

func commandIdentity(request runner.CommandRequest) map[string]any {
	hash := sha256.Sum256(request.Stdin)
	return map[string]any{
		"operation":    request.Operation,
		"executable":   request.Executable,
		"argv":         append([]string(nil), request.Argv...),
		"cwd":          request.Cwd,
		"network":      string(request.Network),
		"scope":        string(request.Scope),
		"timeout":      int64(request.Timeout),
		"outputPolicy": string(request.OutputPolicy),
		"outputLimit":  request.OutputLimit,
		"stdinSha256":  hex.EncodeToString(hash[:]),
	}
}

func versionSnapshot(installed map[string]string) map[string]string {
	return copyPackages(installed)
}

func versionSubset(installed map[string]string, names []string) map[string]string {
	out := make(map[string]string)
	for _, name := range names {
		if version, ok := installed[name]; ok {
			out[name] = version
		}
	}
	return out
}

func explicitInstalled(installed map[string]string, explicit map[string]bool) []string {
	out := make([]string, 0)
	for _, name := range sortedPackageNames(installed) {
		if explicit[name] {
			out = append(out, name)
		}
	}
	return out
}

func copyPackages(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for name, version := range values {
		out[name] = version
	}
	return out
}

func sortedPackageNames(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for name := range values {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func hasPackage(installed map[string]string, name string) bool {
	_, ok := installed[name]
	return ok
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mustJSON(value any) planner.JSONValue {
	data, _ := json.Marshal(value)
	return planner.JSONValue(data)
}
