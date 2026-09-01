package cachyos

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"alex-cachyos/internal/assets"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const (
	// VicinaePackageName is the AUR package that owns the Vicinae executable.
	// The similarly named repository package "vicinae" is deliberately not an
	// acceptable substitute.
	VicinaePackageName = "vicinae-bin"
	// VicinaeServiceUnit is the per-user systemd unit managed by this module.
	VicinaeServiceUnit = "vicinae.service"
	// VicinaeEnvironmentPath is the system environment drop-in used by COSMIC's
	// clipboard integration.
	VicinaeEnvironmentPath = "/etc/environment.d/99-vicinae-cosmic.conf"
	// VicinaeStockSystemActionsPath is the read-only COSMIC system-actions source.
	VicinaeStockSystemActionsPath = "/usr/share/cosmic/com.system76.CosmicSettings.Shortcuts/v1/system_actions"
	// VicinaeShortcutRelativePath is the per-user COSMIC shortcut directory.
	VicinaeShortcutRelativePath = ".config/cosmic/com.system76.CosmicSettings.Shortcuts/v1"

	vicinaeEnvironmentAsset = "templates/vicinae/99-vicinae-cosmic.conf"
	vicinaeCustomAsset      = "templates/vicinae/cosmic-shortcuts-custom"

	vicinaeModuleName = "vicinae"

	vicinaePackageInstallOperation  = "vicinae.package.install"
	vicinaePackageFetchOperation    = "vicinae.package.fetch"
	vicinaePackageCheckoutOperation = "vicinae.package.checkout"
	vicinaePackagePatchOperation    = "vicinae.package.patch.materialize"
	vicinaePackageVerifyOperation   = "vicinae.package.patch.verify"
	vicinaePackageRemoveOperation   = "vicinae.package.remove"
	vicinaePackageRestoreOperation  = "vicinae.package.restore"

	vicinaeServiceEnableOperation  = "vicinae.service.enable"
	vicinaeServiceDisableOperation = "vicinae.service.disable"
	vicinaeServiceRestoreOperation = "vicinae.service.restore"

	vicinaeEnvironmentOperation    = "vicinae.file.environment"
	vicinaeCustomShortcutOperation = "vicinae.shortcuts.custom"
	vicinaeSystemActionsOperation  = "vicinae.shortcuts.system-actions"
	vicinaeFileReapplyOperation    = "vicinae.file.reapply"
)

const (
	// Public operation names keep plan consumers independent from string
	// literals while retaining the same naming convention as bootstrap.
	VicinaeModuleName              = vicinaeModuleName
	VicinaePackageInstallOperation = vicinaePackageInstallOperation
	VicinaePackageRemoveOperation  = vicinaePackageRemoveOperation
	VicinaePackageRestoreOperation = vicinaePackageRestoreOperation
	VicinaeServiceEnableOperation  = vicinaeServiceEnableOperation
	VicinaeServiceDisableOperation = vicinaeServiceDisableOperation
	VicinaeServiceRestoreOperation = vicinaeServiceRestoreOperation
	VicinaeEnvironmentOperation    = vicinaeEnvironmentOperation
	VicinaeCustomShortcutOperation = vicinaeCustomShortcutOperation
	VicinaeSystemActionsOperation  = vicinaeSystemActionsOperation
	VicinaeFileReapplyOperation    = vicinaeFileReapplyOperation
)

var (
	// ErrInvalidVicinaePaths identifies a path that cannot safely be used by the
	// module. Paths are caller-provided so tests and higher-level observation ports
	// never need to rely on the process HOME.
	ErrInvalidVicinaePaths = errors.New("invalid Vicinae paths")
	// ErrInvalidVicinaeObservation identifies incomplete or contradictory state.
	ErrInvalidVicinaeObservation = errors.New("invalid Vicinae observation")
	// ErrVicinaeLauncherMissing means that the stock action file has no active
	// Launcher field that this module can safely rewrite.
	ErrVicinaeLauncherMissing = errors.New("COSMIC Launcher action is missing")
	// ErrVicinaeLauncherAmbiguous means that more than one active Launcher field
	// was found. Rewriting an ambiguous source is fail-closed.
	ErrVicinaeLauncherAmbiguous = errors.New("COSMIC Launcher action is ambiguous")
	// ErrVicinaeLauncherInvalid identifies a malformed or unsupported Launcher
	// field in an otherwise present stock action file.
	ErrVicinaeLauncherInvalid = errors.New("invalid COSMIC Launcher action")
)

// VicinaePaths contains the absolute roots used to render one module plan.
// HomeRoot is required even though the system paths are fixed: it prevents a
// planner from silently expanding a tilde or using an ambient HOME value.
type VicinaePaths struct {
	HomeRoot           string
	ShortcutDir        string
	StockSystemActions string
	EnvironmentFile    string
}

// VicinaePathConfig and VicinaePathRoots are descriptive aliases for callers
// that keep path configuration separate from host observations.
type VicinaePathConfig = VicinaePaths
type VicinaePathRoots = VicinaePaths

// NewVicinaePaths derives the fixed target paths from an explicitly supplied,
// clean absolute home directory.
func NewVicinaePaths(homeRoot string) (VicinaePaths, error) {
	return validateVicinaePaths(VicinaePaths{HomeRoot: homeRoot})
}

// DefaultVicinaePaths is the named constructor spelling used by plan callers.
func DefaultVicinaePaths(homeRoot string) (VicinaePaths, error) {
	return NewVicinaePaths(homeRoot)
}

// Validate checks all path invariants without touching the filesystem.
func (p VicinaePaths) Validate() error {
	_, err := validateVicinaePaths(p)
	return err
}

// VicinaePackageObservation is the package/version observation consumed by the
// factory. Installed, Exists, and Present are equivalent presence signals so a
// package port can use the vocabulary already exposed by its inventory API.
type VicinaePackageObservation struct {
	Name      string
	Version   string
	Installed bool
	Exists    bool
	Present   bool
}

// PackageObservation and VicinaePackageState are compatibility aliases for the
// same typed package observation.
type PackageObservation = VicinaePackageObservation
type VicinaePackageState = VicinaePackageObservation

// VicinaeServiceObservation is a typed summary of the user unit. Exists is
// separate from Enabled and Active because a package can be present while the
// unit is absent or not yet loaded.
type VicinaeServiceObservation struct {
	Unit      string
	Exists    bool
	Installed bool
	Enabled   bool
	Active    bool
}

// VicinaeServiceState is a descriptive alias for the service observation.
type VicinaeServiceState = VicinaeServiceObservation

// VicinaeFileObservation is the metadata-only observation for a managed target.
// Content is intentionally absent: hashes, modes, ownership, and the one-time
// backup path are sufficient for planning and inverse binding.
type VicinaeFileObservation struct {
	Path       string
	Exists     bool
	SHA256     string
	Hash       string
	Mode       uint32
	Ownership  FileOwnershipKind
	Kind       FileOwnershipKind
	BackupPath string
}

// FileObservation and VicinaeTargetObservation are concise aliases for the
// metadata-only target observation.
type FileObservation = VicinaeFileObservation
type VicinaeTargetObservation = VicinaeFileObservation

// VicinaeStockSystemActionsObservation contains the read-only bytes needed to
// produce the user override. Stock content is copied defensively and never
// becomes a mutation target.
type VicinaeStockSystemActionsObservation struct {
	Path    string
	Exists  bool
	SHA256  string
	Hash    string
	Mode    uint32
	Content []byte
}

// StockSystemActionsObservation is the short form used by observation ports.
type StockSystemActionsObservation = VicinaeStockSystemActionsObservation

// VicinaeShortcutObservation groups the two user-owned COSMIC shortcut targets.
// Direct fields on VicinaeObservation remain available for the common case.
type VicinaeShortcutObservation struct {
	SystemActions VicinaeFileObservation
	Custom        VicinaeFileObservation
}

// VicinaeObservation is the complete read-only input to the Vicinae factory.
// It contains package/version state, service existence/state, target metadata,
// the stock action bytes, and an explicit removal selection.
type VicinaeObservation struct {
	HomeRoot string
	Home     string
	Paths    VicinaePaths

	Package           VicinaePackageObservation
	InstalledPackages map[string]string
	Catalog           *catalog.Catalog
	AURSource         AppsAURSourceObservation

	Service VicinaeServiceObservation
	// Services is accepted as a compatibility input for the shared platform
	// observation port. The Vicinae factory still requires at most one matching
	// unit and normalizes it into Service before planning.
	Services []ServiceObservation

	Environment     VicinaeFileObservation
	EnvironmentFile VicinaeFileObservation
	SystemActions   VicinaeFileObservation
	CustomShortcut  VicinaeFileObservation
	Custom          VicinaeFileObservation
	Shortcuts       VicinaeShortcutObservation

	StockSystemActions VicinaeStockSystemActionsObservation
	StockActions       VicinaeStockSystemActionsObservation

	Remove bool
}

// VicinaeAssets are the two immutable embedded files consumed by this module.
// Every field returned by LoadVicinaeAssets owns an independent byte slice.
type VicinaeAssets struct {
	Environment    []byte
	CustomShortcut []byte
}

// VicinaeAssetBundle is a descriptive alias for VicinaeAssets.
type VicinaeAssetBundle = VicinaeAssets

// LoadVicinaeAssets reads only the embedded source tree and returns defensive
// copies. It never consults a repository-relative or live-host path.
func LoadVicinaeAssets() (VicinaeAssets, error) {
	environment, err := fs.ReadFile(assets.FS, vicinaeEnvironmentAsset)
	if err != nil {
		return VicinaeAssets{}, fmt.Errorf("read embedded Vicinae environment: %w", err)
	}
	custom, err := fs.ReadFile(assets.FS, vicinaeCustomAsset)
	if err != nil {
		return VicinaeAssets{}, fmt.Errorf("read embedded Vicinae custom shortcuts: %w", err)
	}
	return VicinaeAssets{
		Environment:    append([]byte(nil), environment...),
		CustomShortcut: append([]byte(nil), custom...),
	}, nil
}

// VicinaeRequestPlan is the factory's typed intermediate result. Requests are
// only the commands that still need execution; Steps retain complete projected
// metadata for both satisfied and actionable operations.
type VicinaeRequestPlan struct {
	Requests []runner.CommandRequest
	Files    []ManagedFileDescriptor

	Environment    ManagedFileDescriptor
	CustomShortcut ManagedFileDescriptor
	SystemActions  ManagedFileDescriptor

	Warnings []string
	Steps    []planner.Step
	Remove   bool
}

// VicinaePlanData is a descriptive alias for the intermediate request plan.
type VicinaePlanData = VicinaeRequestPlan

// BuildVicinaeRequestPlan builds the typed requests, desired files, warnings,
// and planner steps. An optional removal argument overrides observation.Remove
// so both CLI-oriented and observation-oriented callers share one factory.
func BuildVicinaeRequestPlan(observation VicinaeObservation, remove ...bool) (VicinaeRequestPlan, error) {
	removeRequested, err := requestedVicinaeRemoval(observation.Remove, remove)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	paths, err := resolveVicinaePaths(observation)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	assetsValue, err := LoadVicinaeAssets()
	if err != nil {
		return VicinaeRequestPlan{}, err
	}

	packageObservation, err := normalizeVicinaePackageObservation(observation)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	pin, err := resolveVicinaePin(observation, removeRequested)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	serviceObservation, err := preferredVicinaeServiceObservation(observation)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}

	environmentObservation, err := normalizeVicinaeFileObservation(
		"environment", preferredVicinaeFileObservation(observation.Environment, observation.EnvironmentFile), paths.EnvironmentFile,
	)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	customObservation, err := normalizeVicinaeFileObservation(
		"custom shortcuts", preferredVicinaeFileObservation(observation.CustomShortcut, preferredVicinaeFileObservation(observation.Custom, observation.Shortcuts.Custom)), filepath.Join(paths.ShortcutDir, "custom"),
	)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}

	stockObservation := preferredVicinaeStockObservation(observation.StockSystemActions, observation.StockActions)
	stockObservation, err = normalizeVicinaeStockObservation(stockObservation, paths.StockSystemActions)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}

	result := VicinaeRequestPlan{Remove: removeRequested}
	result.Environment = newVicinaeManagedFile(paths.EnvironmentFile, assetsValue.Environment, 0644, "environment")
	result.CustomShortcut = newVicinaeManagedFile(filepath.Join(paths.ShortcutDir, "custom"), assetsValue.CustomShortcut, 0644, "custom")
	result.Files = append(result.Files, result.Environment.Clone(), result.CustomShortcut.Clone())

	var systemObservation VicinaeFileObservation
	if stockObservation.Exists {
		rewritten, rewriteErr := RewriteCosmicSystemActions(stockObservation.Content)
		if rewriteErr != nil {
			return VicinaeRequestPlan{}, rewriteErr
		}
		result.SystemActions = newVicinaeManagedFile(filepath.Join(paths.ShortcutDir, "system_actions"), rewritten, vicinaeStockMode(stockObservation.Mode), "system-actions")
		result.Files = append(result.Files, result.SystemActions.Clone())
		systemObservation, err = normalizeVicinaeFileObservation(
			"system actions", preferredVicinaeFileObservation(observation.SystemActions, observation.Shortcuts.SystemActions), result.SystemActions.Path,
		)
		if err != nil {
			return VicinaeRequestPlan{}, err
		}
	} else {
		// The Bash module treats a missing stock file as a warning-only condition
		// during installation. Removal still restores an already-managed user
		// target, so retain a path-only descriptor for that inverse operation.
		result.Warnings = append(result.Warnings, "missing stock COSMIC system_actions; shortcut override skipped")
		if removeRequested {
			result.SystemActions = newVicinaeManagedFile(filepath.Join(paths.ShortcutDir, "system_actions"), nil, 0644, "system-actions")
			systemObservation, err = normalizeVicinaeFileObservation(
				"system actions", preferredVicinaeFileObservation(observation.SystemActions, observation.Shortcuts.SystemActions), result.SystemActions.Path,
			)
			if err != nil {
				return VicinaeRequestPlan{}, err
			}
			if systemObservation.Exists {
				result.Files = append(result.Files, result.SystemActions.Clone())
			}
		}
	}

	packageStep, packageRequest, err := vicinaePackageStep(packageObservation, pin, paths.HomeRoot, removeRequested)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	if packageRequest != nil && packageStep.Disposition == planner.DispositionApply {
		sourceSteps, sourceRequests, sourceErr := vicinaePinnedSourceSteps(observation.AURSource, pin, paths.HomeRoot)
		if sourceErr != nil {
			return VicinaeRequestPlan{}, sourceErr
		}
		result.Steps = append(result.Steps, sourceSteps...)
		result.Requests = append(result.Requests, sourceRequests...)
		result.Requests = append(result.Requests, *packageRequest)
	}
	result.Steps = append(result.Steps, packageStep)

	fileSteps := make([]planner.Step, 0, len(result.Files))
	var environmentStep planner.Step
	if removeRequested {
		environmentStep = vicinaeRetainedFileStep(
			vicinaeEnvironmentOperation,
			"retain the Vicinae COSMIC environment drop-in on module removal",
			result.Environment,
			environmentObservation,
			runner.ScopeSystem,
			[]string{vicinaePackageInstallOperation},
		)
	} else {
		environmentStep, err = vicinaeFileStep(
			vicinaeEnvironmentOperation,
			"write the Vicinae COSMIC environment drop-in",
			result.Environment,
			environmentObservation,
			runner.ScopeSystem,
			false,
			[]string{vicinaePackageInstallOperation},
		)
		if err != nil {
			return VicinaeRequestPlan{}, err
		}
	}
	fileSteps = append(fileSteps, environmentStep)

	customStep, err := vicinaeFileStep(
		vicinaeCustomShortcutOperation,
		"write the Vicinae custom COSMIC shortcuts",
		result.CustomShortcut,
		customObservation,
		runner.ScopeUser,
		removeRequested,
		[]string{vicinaePackageInstallOperation},
	)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	fileSteps = append(fileSteps, customStep)

	if stockObservation.Exists || (removeRequested && systemObservation.Exists) {
		systemStep, stepErr := vicinaeFileStep(
			vicinaeSystemActionsOperation,
			"rewrite the COSMIC Launcher system action",
			result.SystemActions,
			systemObservation,
			runner.ScopeUser,
			removeRequested,
			[]string{vicinaePackageInstallOperation},
		)
		if stepErr != nil {
			return VicinaeRequestPlan{}, stepErr
		}
		fileSteps = append(fileSteps, systemStep)
	}

	serviceStep, serviceRequest, err := vicinaeServiceStep(serviceObservation, removeRequested, fileSteps)
	if err != nil {
		return VicinaeRequestPlan{}, err
	}
	if serviceRequest != nil && serviceStep.Disposition != planner.DispositionSatisfied {
		result.Requests = append(result.Requests, *serviceRequest)
	}

	if removeRequested {
		// Stop/disable the unit before restoring user shortcut files. The package
		// and environment steps remain satisfied retention records, matching the
		// Bash module's deliberately non-destructive --remove behavior.
		result.Steps = append(result.Steps, serviceStep)
		for i := range fileSteps {
			if fileSteps[i].Disposition == planner.DispositionRemove {
				fileSteps[i].DependsOn = appendUniqueStrings(fileSteps[i].DependsOn, vicinaeServiceOperationForRemoval(serviceObservation))
			}
		}
		result.Steps = append(result.Steps, fileSteps...)
	} else {
		result.Steps = append(result.Steps, fileSteps...)
		result.Steps = append(result.Steps, serviceStep)
	}

	result.Requests = cloneVicinaeRequests(result.Requests)
	result.Files = cloneVicinaeFiles(result.Files)
	result.Warnings = append([]string(nil), result.Warnings...)
	result.Steps = cloneVicinaeSteps(result.Steps)
	return result, nil
}

// BuildVicinaeModule converts the intermediate result into the shared planner
// module interface. The module has no cross-module dependency: the canonical
// planner can place it independently alongside the other post-bootstrap units.
func BuildVicinaeModule(observation VicinaeObservation, remove ...bool) (planner.Module, error) {
	requestPlan, err := BuildVicinaeRequestPlan(observation, remove...)
	if err != nil {
		return planner.Module{}, err
	}
	return planner.Module{
		Name:    vicinaeModuleName,
		Enabled: true,
		Steps:   requestPlan.Steps,
	}, nil
}

// BuildVicinaePlan delegates dependency validation, stable ordering, and
// defensive copies to the shared planner while selecting only Vicinae.
func BuildVicinaePlan(observation VicinaeObservation, remove ...bool) (planner.Plan, error) {
	module, err := BuildVicinaeModule(observation, remove...)
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.BuildPlan([]planner.Module{module}, planner.Selection{Only: []string{vicinaeModuleName}})
}

// BuildVicinaeRemovalPlan is an explicit spelling for callers implementing the
// --remove command surface.
func BuildVicinaeRemovalPlan(observation VicinaeObservation) (planner.Plan, error) {
	return BuildVicinaePlan(observation, true)
}

// RewriteCosmicSystemActions replaces exactly one active stock Launcher field.
// It preserves every byte outside the quoted action value and is idempotent when
// the desired "vicinae toggle" value is already present.
func RewriteCosmicSystemActions(data []byte) ([]byte, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: input is not valid UTF-8", ErrVicinaeLauncherInvalid)
	}
	matches, err := parseCosmicLauncherFields(data)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: no active Launcher field", ErrVicinaeLauncherMissing)
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("%w: found %d active Launcher fields", ErrVicinaeLauncherAmbiguous, len(matches))
	}
	match := matches[0]
	if match.Value == "vicinae toggle" {
		return append([]byte(nil), data...), nil
	}
	if match.Value != "cosmic-launcher" {
		return nil, fmt.Errorf("%w: unsupported Launcher value", ErrVicinaeLauncherInvalid)
	}
	out := make([]byte, 0, len(data)+len("vicinae toggle")-len(match.Value))
	out = append(out, data[:match.ValueStart]...)
	out = append(out, "vicinae toggle"...)
	out = append(out, data[match.ValueEnd:]...)
	return out, nil
}

// RewriteVicinaeSystemActions is an alias for the module-oriented name.
func RewriteVicinaeSystemActions(data []byte) ([]byte, error) {
	return RewriteCosmicSystemActions(data)
}

// PatchCosmicSystemActions is a compatibility spelling for callers that call
// the transformation a patch rather than a rewrite.
func PatchCosmicSystemActions(data []byte) ([]byte, error) {
	return RewriteCosmicSystemActions(data)
}

// ValidateCosmicSystemActions validates the unique Launcher contract without
// returning or modifying any content.
func ValidateCosmicSystemActions(data []byte) error {
	_, err := RewriteCosmicSystemActions(data)
	return err
}

type cosmicLauncherMatch struct {
	Value      string
	ValueStart int
	ValueEnd   int
}

func parseCosmicLauncherFields(data []byte) ([]cosmicLauncherMatch, error) {
	matches := make([]cosmicLauncherMatch, 0, 1)
	lineStart := 0
	lineNumber := 1
	for lineStart <= len(data) {
		lineEnd := bytes.IndexByte(data[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(data)
		} else {
			lineEnd += lineStart
		}
		lineMatches, err := parseCosmicLauncherLine(data, lineStart, lineEnd, lineNumber)
		if err != nil {
			return nil, err
		}
		matches = append(matches, lineMatches...)
		if lineEnd == len(data) {
			break
		}
		lineStart, lineNumber = lineEnd+1, lineNumber+1
	}
	return matches, nil
}

func parseCosmicLauncherLine(data []byte, start, end, lineNumber int) ([]cosmicLauncherMatch, error) {
	matches := make([]cosmicLauncherMatch, 0, 1)
	for i := start; i < end; {
		switch data[i] {
		case '/':
			if i+1 < end && data[i+1] == '/' {
				return matches, nil
			}
			i++
		case '"':
			var err error
			i, err = skipCosmicString(data, i, end)
			if err != nil {
				return nil, fmt.Errorf("%w at line %d", ErrVicinaeLauncherInvalid, lineNumber)
			}
		default:
			if !hasCosmicIdentifierBoundary(data, i, end, "Launcher") {
				i++
				continue
			}
			colon := i + len("Launcher")
			colon = skipCosmicHorizontalSpace(data, colon, end)
			if colon >= end || data[colon] != ':' {
				// Bare Launcher is an enum/value in the surrounding syntax, not a
				// field. Only a field named Launcher is subject to this parser.
				i += len("Launcher")
				continue
			}
			valueStart := skipCosmicHorizontalSpace(data, colon+1, end)
			if valueStart >= end || data[valueStart] != '"' {
				return nil, fmt.Errorf("%w at line %d", ErrVicinaeLauncherInvalid, lineNumber)
			}
			valueEnd, next, value, err := parseCosmicQuotedValue(data, valueStart, end)
			if err != nil {
				return nil, fmt.Errorf("%w at line %d", ErrVicinaeLauncherInvalid, lineNumber)
			}
			next = skipCosmicHorizontalSpace(data, next, end)
			if next >= end || data[next] != ',' {
				return nil, fmt.Errorf("%w at line %d", ErrVicinaeLauncherInvalid, lineNumber)
			}
			matches = append(matches, cosmicLauncherMatch{Value: value, ValueStart: valueStart + 1, ValueEnd: valueEnd})
			i = next + 1
		}
	}
	return matches, nil
}

func hasCosmicIdentifierBoundary(data []byte, offset, end int, identifier string) bool {
	if offset+len(identifier) > end || string(data[offset:offset+len(identifier)]) != identifier {
		return false
	}
	if offset > 0 && isCosmicIdentifierByte(data[offset-1]) {
		return false
	}
	return offset+len(identifier) == end || !isCosmicIdentifierByte(data[offset+len(identifier)])
}

func isCosmicIdentifierByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '-'
}

func skipCosmicHorizontalSpace(data []byte, offset, end int) int {
	for offset < end && (data[offset] == ' ' || data[offset] == '\t' || data[offset] == '\r') {
		offset++
	}
	return offset
}

func skipCosmicString(data []byte, quote, end int) (int, error) {
	for i := quote + 1; i < end; i++ {
		if data[i] == '\\' {
			if i+1 >= end {
				return end, errors.New("unterminated quoted value")
			}
			i++
			continue
		}
		if data[i] == '"' {
			return i + 1, nil
		}
	}
	return end, errors.New("unterminated quoted value")
}

func parseCosmicQuotedValue(data []byte, quote, end int) (int, int, string, error) {
	for i := quote + 1; i < end; i++ {
		if data[i] == '\\' {
			return 0, 0, "", errors.New("escaped Launcher values are not accepted")
		}
		if data[i] == '"' {
			return i, i + 1, string(data[quote+1 : i]), nil
		}
	}
	return 0, 0, "", errors.New("unterminated Launcher value")
}

func requestedVicinaeRemoval(observation bool, values []bool) (bool, error) {
	if len(values) > 1 {
		return false, fmt.Errorf("%w: at most one removal flag is allowed", ErrInvalidVicinaeObservation)
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return observation, nil
}

func resolveVicinaePaths(observation VicinaeObservation) (VicinaePaths, error) {
	paths := observation.Paths
	if paths.HomeRoot == "" {
		paths.HomeRoot = observation.HomeRoot
	}
	if paths.HomeRoot == "" {
		paths.HomeRoot = observation.Home
	}
	if paths.ShortcutDir == "" && paths.HomeRoot != "" {
		paths.ShortcutDir = filepath.Join(paths.HomeRoot, VicinaeShortcutRelativePath)
	}
	if paths.StockSystemActions == "" {
		paths.StockSystemActions = VicinaeStockSystemActionsPath
	}
	if paths.EnvironmentFile == "" {
		paths.EnvironmentFile = VicinaeEnvironmentPath
	}
	return validateVicinaePaths(paths)
}

func validateVicinaePaths(paths VicinaePaths) (VicinaePaths, error) {
	for label, value := range map[string]string{
		"home root":            paths.HomeRoot,
		"shortcut directory":   paths.ShortcutDir,
		"stock system_actions": paths.StockSystemActions,
		"environment file":     paths.EnvironmentFile,
	} {
		if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return VicinaePaths{}, fmt.Errorf("%w: %s must be a clean absolute path", ErrInvalidVicinaePaths, label)
		}
	}
	if !vicinaePathContainedBy(paths.HomeRoot, paths.ShortcutDir) {
		return VicinaePaths{}, fmt.Errorf("%w: shortcut directory escapes home root", ErrInvalidVicinaePaths)
	}
	if paths.EnvironmentFile != VicinaeEnvironmentPath {
		return VicinaePaths{}, fmt.Errorf("%w: environment file must be %s", ErrInvalidVicinaePaths, VicinaeEnvironmentPath)
	}
	return VicinaePaths{
		HomeRoot:           filepath.ToSlash(paths.HomeRoot),
		ShortcutDir:        filepath.ToSlash(paths.ShortcutDir),
		StockSystemActions: filepath.ToSlash(paths.StockSystemActions),
		EnvironmentFile:    filepath.ToSlash(paths.EnvironmentFile),
	}, nil
}

func vicinaePathContainedBy(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func normalizeVicinaePackageObservation(observation VicinaeObservation) (VicinaePackageObservation, error) {
	value := observation.Package
	if value.Name == "" && len(observation.InstalledPackages) != 0 {
		if version, ok := observation.InstalledPackages[VicinaePackageName]; ok {
			value = VicinaePackageObservation{Name: VicinaePackageName, Version: version, Installed: true}
		}
	}
	if value.Name == "" {
		if value.Installed || value.Exists || value.Present || value.Version != "" {
			return VicinaePackageObservation{}, fmt.Errorf("%w: package presence has no name", ErrInvalidVicinaeObservation)
		}
		return value, nil
	}
	if strings.ContainsAny(value.Name, "\x00\r\n") || strings.TrimSpace(value.Name) != value.Name || !validPackageName(value.Name) {
		return VicinaePackageObservation{}, fmt.Errorf("%w: invalid package name", ErrInvalidVicinaeObservation)
	}
	if strings.ContainsAny(value.Version, "\x00\r\n") {
		return VicinaePackageObservation{}, fmt.Errorf("%w: invalid package version", ErrInvalidVicinaeObservation)
	}
	return value, nil
}

func preferredVicinaeServiceObservation(observation VicinaeObservation) (VicinaeServiceObservation, error) {
	value := observation.Service
	if !isZeroVicinaeServiceObservation(value) || len(observation.Services) == 0 {
		return normalizeVicinaeServiceObservation(value)
	}
	if len(observation.Services) != 1 {
		return VicinaeServiceObservation{}, fmt.Errorf("%w: expected one Vicinae service observation, got %d", ErrInvalidVicinaeObservation, len(observation.Services))
	}
	legacy := observation.Services[0]
	if legacy.Unit != "" && legacy.Unit != VicinaeServiceUnit {
		return VicinaeServiceObservation{}, fmt.Errorf("%w: expected service %s", ErrInvalidVicinaeObservation, VicinaeServiceUnit)
	}
	return normalizeVicinaeServiceObservation(VicinaeServiceObservation{
		Unit:      legacy.Unit,
		Exists:    legacy.Installed || legacy.Enabled || legacy.Active,
		Installed: legacy.Installed,
		Enabled:   legacy.Enabled,
		Active:    legacy.Active,
	})
}

func isZeroVicinaeServiceObservation(value VicinaeServiceObservation) bool {
	return value.Unit == "" && !value.Exists && !value.Installed && !value.Enabled && !value.Active
}

func normalizeVicinaeServiceObservation(value VicinaeServiceObservation) (VicinaeServiceObservation, error) {
	if value.Unit == "" {
		value.Unit = VicinaeServiceUnit
	}
	if value.Unit != VicinaeServiceUnit || strings.ContainsAny(value.Unit, "\x00\r\n") {
		return VicinaeServiceObservation{}, fmt.Errorf("%w: expected service %s", ErrInvalidVicinaeObservation, VicinaeServiceUnit)
	}
	value.Exists = value.Exists || value.Installed
	if !value.Exists && (value.Enabled || value.Active) {
		return VicinaeServiceObservation{}, fmt.Errorf("%w: service state is enabled or active but unit is absent", ErrInvalidVicinaeObservation)
	}
	return value, nil
}

func normalizeVicinaeFileObservation(label string, value VicinaeFileObservation, path string) (VicinaeFileObservation, error) {
	if value.Path == "" {
		value.Path = path
	}
	if value.Path != path || value.Path == "" || !utf8.ValidString(value.Path) || strings.ContainsAny(value.Path, "\x00\r\n") || !filepath.IsAbs(value.Path) || filepath.Clean(value.Path) != value.Path {
		return VicinaeFileObservation{}, fmt.Errorf("%w: %s has an invalid target path", ErrInvalidVicinaeObservation, label)
	}
	if value.SHA256 != "" && value.Hash != "" && !strings.EqualFold(value.SHA256, value.Hash) {
		return VicinaeFileObservation{}, fmt.Errorf("%w: %s has contradictory hashes", ErrInvalidVicinaeObservation, label)
	}
	if value.SHA256 == "" {
		value.SHA256 = value.Hash
	}
	if value.SHA256 != "" && !validSHA256(value.SHA256) {
		return VicinaeFileObservation{}, fmt.Errorf("%w: %s has an invalid hash", ErrInvalidVicinaeObservation, label)
	}
	value.SHA256 = strings.ToLower(value.SHA256)
	if !value.Exists {
		if value.SHA256 != "" || value.Mode != 0 || value.Ownership != "" || value.Kind != "" || value.BackupPath != "" {
			return VicinaeFileObservation{}, fmt.Errorf("%w: absent %s carries target metadata", ErrInvalidVicinaeObservation, label)
		}
		value.Ownership = OwnershipCreated
		return value, nil
	}
	if value.SHA256 == "" || value.Mode == 0 {
		return VicinaeFileObservation{}, fmt.Errorf("%w: existing %s needs hash and mode", ErrInvalidVicinaeObservation, label)
	}
	if value.Ownership != "" && value.Kind != "" && value.Ownership != value.Kind {
		return VicinaeFileObservation{}, fmt.Errorf("%w: %s has contradictory ownership", ErrInvalidVicinaeObservation, label)
	}
	if value.Ownership == "" {
		value.Ownership = value.Kind
	}
	if value.Ownership != OwnershipCreated && value.Ownership != OwnershipAdopted {
		return VicinaeFileObservation{}, fmt.Errorf("%w: %s has unknown ownership", ErrInvalidVicinaeObservation, label)
	}
	if value.Ownership == OwnershipCreated && value.BackupPath != "" {
		return VicinaeFileObservation{}, fmt.Errorf("%w: created %s carries a backup", ErrInvalidVicinaeObservation, label)
	}
	if value.Ownership == OwnershipAdopted {
		wantBackup := value.Path + ".bak.alex-cachyos"
		if value.BackupPath != wantBackup {
			return VicinaeFileObservation{}, fmt.Errorf("%w: adopted %s has an invalid backup", ErrInvalidVicinaeObservation, label)
		}
	}
	return value, nil
}

func normalizeVicinaeStockObservation(value VicinaeStockSystemActionsObservation, path string) (VicinaeStockSystemActionsObservation, error) {
	if value.Path == "" {
		value.Path = path
	}
	if value.Path != path || value.Path == "" || !utf8.ValidString(value.Path) || strings.ContainsAny(value.Path, "\x00\r\n") || !filepath.IsAbs(value.Path) || filepath.Clean(value.Path) != value.Path {
		return VicinaeStockSystemActionsObservation{}, fmt.Errorf("%w: stock system_actions path is invalid", ErrInvalidVicinaeObservation)
	}
	if value.SHA256 != "" && value.Hash != "" && !strings.EqualFold(value.SHA256, value.Hash) {
		return VicinaeStockSystemActionsObservation{}, fmt.Errorf("%w: stock system_actions has contradictory hashes", ErrInvalidVicinaeObservation)
	}
	if value.SHA256 == "" {
		value.SHA256 = value.Hash
	}
	if !value.Exists {
		if len(value.Content) != 0 || value.SHA256 != "" || value.Mode != 0 {
			return VicinaeStockSystemActionsObservation{}, fmt.Errorf("%w: absent stock system_actions carries content", ErrInvalidVicinaeObservation)
		}
		return value, nil
	}
	if value.Content == nil {
		return VicinaeStockSystemActionsObservation{}, fmt.Errorf("%w: existing stock system_actions has no content", ErrInvalidVicinaeObservation)
	}
	actual := sha256Hex(value.Content)
	if value.SHA256 != "" && !strings.EqualFold(value.SHA256, actual) {
		return VicinaeStockSystemActionsObservation{}, fmt.Errorf("%w: stock system_actions hash does not match content", ErrInvalidVicinaeObservation)
	}
	value.SHA256 = actual
	if value.Mode == 0 {
		value.Mode = 0644
	}
	value.Content = append([]byte(nil), value.Content...)
	return value, nil
}

func preferredVicinaeFileObservation(primary, fallback VicinaeFileObservation) VicinaeFileObservation {
	if !isZeroVicinaeFileObservation(primary) {
		return primary
	}
	return fallback
}

func preferredVicinaeStockObservation(primary, fallback VicinaeStockSystemActionsObservation) VicinaeStockSystemActionsObservation {
	if !isZeroVicinaeStockObservation(primary) {
		return primary
	}
	return fallback
}

func isZeroVicinaeFileObservation(value VicinaeFileObservation) bool {
	return value.Path == "" && !value.Exists && value.SHA256 == "" && value.Hash == "" && value.Mode == 0 && value.Ownership == "" && value.Kind == "" && value.BackupPath == ""
}

func isZeroVicinaeStockObservation(value VicinaeStockSystemActionsObservation) bool {
	return value.Path == "" && !value.Exists && value.SHA256 == "" && value.Hash == "" && value.Mode == 0 && value.Content == nil
}

func resolveVicinaePin(observation VicinaeObservation, remove bool) (catalog.AURLocalPin, error) {
	if remove {
		return catalog.AURLocalPin{}, nil
	}
	if observation.Catalog == nil || observation.Catalog.Pins == nil {
		return catalog.AURLocalPin{}, fmt.Errorf("%w: missing catalog AUR/local pin for %q", ErrInvalidVicinaeObservation, VicinaePackageName)
	}
	pin, ok := observation.Catalog.Pins.AURLocal[VicinaePackageName]
	if !ok {
		return catalog.AURLocalPin{}, fmt.Errorf("%w: missing catalog AUR/local pin for %q", ErrInvalidVicinaeObservation, VicinaePackageName)
	}
	if err := catalog.ValidateAURLocalPin(VicinaePackageName, pin); err != nil {
		return catalog.AURLocalPin{}, fmt.Errorf("%w: %v", ErrInvalidVicinaeObservation, err)
	}
	return pin, nil
}

func vicinaePinnedSourceSteps(observed AppsAURSourceObservation, pin catalog.AURLocalPin, home string) ([]planner.Step, []runner.CommandRequest, error) {
	sourceDir := filepath.Join(home, ".cache", "alex-cachyos", "aur", VicinaePackageName)
	patchName := ".alex-cachyos-source.patch"
	patchPath := filepath.Join(sourceDir, patchName)
	remote := "https://aur.archlinux.org/" + VicinaePackageName + ".git"
	if observed.Exists && observed.Remote != "" && observed.Remote != remote {
		return nil, nil, fmt.Errorf("%w: Vicinae AUR source has unexpected remote", ErrInvalidVicinaeObservation)
	}

	fetchArgv := []string{"clone", "--filter=blob:none", "--no-checkout", remote, sourceDir}
	if observed.Exists {
		fetchArgv = []string{"-C", sourceDir, "fetch", "origin", pin.SourceCommit}
	}
	fetch, err := makeVicinaeRequestAt(vicinaePackageFetchOperation, "/usr/bin/git", fetchArgv, home, runner.ScopeUser, runner.NetworkRequired, nil)
	if err != nil {
		return nil, nil, err
	}
	steps := []planner.Step{moduleRequestStep(vicinaeModuleName, fetch, planner.DispositionApply,
		map[string]any{"package": VicinaePackageName, "remote": remote, "sourceCommit": pin.SourceCommit, "sourceDir": sourceDir},
		map[string]any{"exists": observed.Exists, "remote": observed.Remote, "commit": observed.Commit}, vicinaePackageFetchOperation+".restore")}
	requests := []runner.CommandRequest{fetch}

	checkout, err := makeVicinaeRequestAt(vicinaePackageCheckoutOperation, "/usr/bin/git", []string{"-C", sourceDir, "checkout", "--detach", pin.SourceCommit}, home, runner.ScopeUser, runner.NetworkNone, nil)
	if err != nil {
		return nil, nil, err
	}
	checkoutStep := moduleRequestStep(vicinaeModuleName, checkout, convergedDisposition(observed.Commit == pin.SourceCommit),
		map[string]any{"package": VicinaePackageName, "sourceCommit": pin.SourceCommit, "sourceDir": sourceDir},
		map[string]any{"commit": observed.Commit}, vicinaePackageCheckoutOperation+".restore")
	checkoutStep.DependsOn = []string{vicinaePackageFetchOperation}
	steps = append(steps, checkoutStep)
	if observed.Commit != pin.SourceCommit {
		requests = append(requests, checkout)
	}

	materialize, err := makeVicinaeRequestAt(vicinaePackagePatchOperation, "/usr/bin/git", []string{
		"-C", sourceDir, "diff-tree", "--root", "--no-commit-id", "--binary", "-p", "--output=" + patchPath, pin.SourceCommit,
	}, home, runner.ScopeUser, runner.NetworkNone, nil)
	if err != nil {
		return nil, nil, err
	}
	patchConverged := observed.Commit == pin.SourceCommit && strings.EqualFold(observed.PatchSHA256, pin.PatchSHA256)
	materializeStep := moduleRequestStep(vicinaeModuleName, materialize, convergedDisposition(patchConverged),
		map[string]any{"package": VicinaePackageName, "sourceCommit": pin.SourceCommit, "patchPath": patchPath, "patchSHA256": pin.PatchSHA256},
		map[string]any{"patchSHA256": observed.PatchSHA256}, vicinaePackagePatchOperation+".remove")
	materializeStep.DependsOn = []string{vicinaePackageCheckoutOperation}
	steps = append(steps, materializeStep)
	if !patchConverged {
		requests = append(requests, materialize)
	}

	verify, err := makeVicinaeRequestAt(vicinaePackageVerifyOperation, "/usr/bin/sha256sum", []string{"--check", "--strict", "-"}, sourceDir, runner.ScopeUser, runner.NetworkNone, []byte(pin.PatchSHA256+"  "+patchName+"\n"))
	if err != nil {
		return nil, nil, err
	}
	verifyStep := moduleRequestStep(vicinaeModuleName, verify, convergedDisposition(patchConverged),
		map[string]any{"package": VicinaePackageName, "patchPath": patchPath, "patchSHA256": pin.PatchSHA256},
		map[string]any{"patchSHA256": observed.PatchSHA256}, vicinaePackageVerifyOperation)
	verifyStep.DependsOn = []string{vicinaePackagePatchOperation}
	steps = append(steps, verifyStep)
	if !patchConverged {
		requests = append(requests, verify)
	}
	return steps, requests, nil
}

func vicinaePackageStep(observation VicinaePackageObservation, pin catalog.AURLocalPin, home string, remove bool) (planner.Step, *runner.CommandRequest, error) {
	installed := observation.Name == VicinaePackageName && (observation.Installed || observation.Exists || observation.Present || observation.Version != "")
	sourceDir := filepath.Join(home, ".cache", "alex-cachyos", "aur", VicinaePackageName)
	request, err := makeVicinaeRequestAt(vicinaePackageInstallOperation, "/usr/bin/paru", []string{"-B", "--install", "--needed", "--noconfirm", sourceDir}, home, runner.ScopeUser, runner.NetworkRequired, nil)
	if err != nil {
		return planner.Step{}, nil, err
	}
	requestOperation := vicinaePackageInstallOperation
	desiredAction := "install"
	disposition := planner.DispositionApply
	network := planner.NetworkRequired
	inverse := &planner.InverseDescriptor{
		Operation: planner.Operation(vicinaePackageRestoreOperation),
		Value: mustJSON(map[string]any{
			"action":         "receipt-rollback",
			"packageName":    VicinaePackageName,
			"priorInstalled": installed,
			"priorVersion":   observation.Version,
			"exactName":      true,
		}),
	}
	if !installed {
		inverse.Operation = planner.Operation(vicinaePackageRemoveOperation)
	}
	if remove {
		desiredAction = "retain-on-module-remove"
		disposition = planner.DispositionSatisfied
		network = planner.NetworkNone
		requestOperation = "vicinae.package.retained"
		inverse = nil
	}
	if installed && !remove {
		disposition = planner.DispositionSatisfied
	}
	desired := map[string]any{
		"action": desiredAction,
		"package": map[string]any{
			"name":      VicinaePackageName,
			"exactName": true,
		},
		"requestOperation": requestOperation,
	}
	if !remove {
		desired["sourceCommit"] = pin.SourceCommit
		desired["patchSHA256"] = pin.PatchSHA256
		desired["sourceDir"] = sourceDir
		desired["packageManager"] = "paru-local-build"
		desired["request"] = commandIdentity(request)
	}
	observed := map[string]any{
		"name":      observation.Name,
		"version":   observation.Version,
		"installed": installed,
		"exactName": observation.Name == VicinaePackageName,
	}
	step := planner.Step{
		ID:          vicinaePackageInstallOperation,
		Module:      vicinaeModuleName,
		DependsOn:   map[bool][]string{false: {vicinaePackageVerifyOperation}, true: nil}[remove || installed],
		Description: "ensure the exact Vicinae package is installed",
		Scope:       planner.ScopeUser,
		Network:     network,
		Operation:   planner.Operation(vicinaePackageInstallOperation),
		Disposition: disposition,
		Desired:     mustJSON(desired),
		Observed:    mustJSON(observed),
		Inverse:     inverse,
	}
	return step, &request, nil
}

func vicinaeServiceStep(observation VicinaeServiceObservation, remove bool, fileSteps []planner.Step) (planner.Step, *runner.CommandRequest, error) {
	action := "enable-and-start"
	operation := vicinaeServiceEnableOperation
	needsMutation := !(observation.Exists && observation.Enabled && observation.Active)
	if remove {
		action = "stop-and-disable"
		operation = vicinaeServiceDisableOperation
		needsMutation = observation.Exists && (observation.Enabled || observation.Active)
	}
	request, err := makeVicinaeRequest(operation, "/usr/bin/systemctl", []string{"--user", map[bool]string{false: "enable", true: "disable"}[remove], "--now", VicinaeServiceUnit}, runner.ScopeUser, runner.NetworkNone)
	if err != nil {
		return planner.Step{}, nil, err
	}
	disposition := planner.DispositionApply
	if !needsMutation {
		disposition = planner.DispositionSatisfied
	}
	inverseValue := map[string]any{
		"action":  "receipt-rollback",
		"unit":    VicinaeServiceUnit,
		"exists":  observation.Exists,
		"enabled": observation.Enabled,
		"active":  observation.Active,
		"source":  "prior-service-state",
	}
	desired := map[string]any{
		"action":           action,
		"unit":             VicinaeServiceUnit,
		"exists":           true,
		"enabled":          !remove,
		"active":           !remove,
		"request":          commandIdentity(request),
		"priorStateStored": true,
	}
	if remove {
		desired["retainsPackage"] = true
		desired["retainsEnvironment"] = true
	}
	observed := map[string]any{
		"unit":    VicinaeServiceUnit,
		"exists":  observation.Exists,
		"enabled": observation.Enabled,
		"active":  observation.Active,
	}
	dependsOn := []string{vicinaePackageInstallOperation}
	if remove {
		// The service is deliberately before shortcut restoration. This is the
		// only ordering difference between normal installation and module removal.
		dependsOn = append(dependsOn, vicinaeEnvironmentOperation)
	}
	if !remove {
		for _, fileStep := range fileSteps {
			dependsOn = appendUniqueStrings(dependsOn, fileStep.ID)
		}
	}
	step := planner.Step{
		ID:          operation,
		Module:      vicinaeModuleName,
		DependsOn:   uniqueStrings(dependsOn),
		Description: "reconcile the Vicinae user service",
		Scope:       planner.ScopeUser,
		Network:     planner.NetworkNone,
		Operation:   planner.Operation(operation),
		Disposition: disposition,
		Desired:     mustJSON(desired),
		Observed:    mustJSON(observed),
		Inverse: &planner.InverseDescriptor{
			Operation: vicinaeServiceRestoreOperation,
			Value:     mustJSON(inverseValue),
		},
	}
	return step, &request, nil
}

func vicinaeRetainedFileStep(operation, description string, file ManagedFileDescriptor, observation VicinaeFileObservation, scope runner.Scope, dependsOn []string) planner.Step {
	return planner.Step{
		ID:          operation,
		Module:      vicinaeModuleName,
		DependsOn:   uniqueStrings(dependsOn),
		Description: description,
		Scope:       planner.Scope(scope),
		Network:     planner.NetworkNone,
		Operation:   planner.Operation(operation),
		Disposition: planner.DispositionSatisfied,
		Desired: mustJSON(map[string]any{
			"action": "retain-on-module-remove",
			"file":   vicinaeManagedFileMetadata(file),
		}),
		Observed: mustJSON(map[string]any{
			"target": vicinaeFileObservationMetadata(observation),
		}),
	}
}

func vicinaeFileStep(operation, description string, file ManagedFileDescriptor, observation VicinaeFileObservation, scope runner.Scope, remove bool, dependsOn []string) (planner.Step, error) {
	desired := map[string]any{
		"action": "ensure",
		"file":   vicinaeManagedFileMetadata(file),
	}
	observed := map[string]any{
		"target": vicinaeFileObservationMetadata(observation),
	}
	disposition := planner.DispositionApply
	var inverse *planner.InverseDescriptor
	if remove {
		desired["action"] = "module-remove"
		if !observation.Exists {
			desired["removalOperation"] = "none"
			disposition = planner.DispositionSatisfied
		} else {
			removalOperation := "remove-file"
			if observation.Ownership == OwnershipAdopted {
				removalOperation = "restore-backup"
			}
			desired["removalOperation"] = removalOperation
			disposition = planner.DispositionRemove
			inverse = &planner.InverseDescriptor{
				Operation: vicinaeFileReapplyOperation,
				Value: mustJSON(map[string]any{
					"action":          "receipt-rollback",
					"file":            vicinaeManagedFileMetadata(file),
					"priorOwnership":  string(observation.Ownership),
					"priorHash":       observation.SHA256,
					"priorMode":       observation.Mode,
					"priorBackupPath": observation.BackupPath,
				}),
			}
		}
	} else {
		if observation.Exists && observation.SHA256 == sha256Hex(file.Content) && observation.Mode == file.Mode {
			disposition = planner.DispositionSatisfied
		}
		bound, err := file.PlannerInverse(FileOwnershipObservation{Kind: observation.Ownership, BackupPath: observation.BackupPath})
		if err != nil {
			return planner.Step{}, fmt.Errorf("%w: bind %s inverse: %v", ErrInvalidVicinaeObservation, operation, err)
		}
		inverse = &bound
	}
	return planner.Step{
		ID:          operation,
		Module:      vicinaeModuleName,
		DependsOn:   uniqueStrings(dependsOn),
		Description: description,
		Scope:       planner.Scope(scope),
		Network:     planner.NetworkNone,
		Operation:   planner.Operation(operation),
		Disposition: disposition,
		Desired:     mustJSON(desired),
		Observed:    mustJSON(observed),
		Inverse:     inverse,
	}, nil
}

func newVicinaeManagedFile(path string, content []byte, mode uint32, kind string) ManagedFileDescriptor {
	return ManagedFileDescriptor{
		Path:    filepath.ToSlash(filepath.Clean(path)),
		Content: append([]byte(nil), content...),
		Mode:    mode,
		Kind:    kind,
	}
}

func vicinaeManagedFileMetadata(file ManagedFileDescriptor) map[string]any {
	return map[string]any{
		"path":       file.Path,
		"sha256":     sha256Hex(file.Content),
		"mode":       file.Mode,
		"kind":       file.Kind,
		"byteLength": len(file.Content),
	}
}

func vicinaeFileObservationMetadata(value VicinaeFileObservation) map[string]any {
	return map[string]any{
		"path":       value.Path,
		"exists":     value.Exists,
		"sha256":     value.SHA256,
		"mode":       value.Mode,
		"ownership":  string(value.Ownership),
		"backupPath": value.BackupPath,
	}
}

func vicinaeStockMode(mode uint32) uint32 {
	if mode == 0 {
		return 0644
	}
	return mode
}

func makeVicinaeRequest(operation, executable string, argv []string, scope runner.Scope, network runner.NetworkPolicy) (runner.CommandRequest, error) {
	return makeVicinaeRequestAt(operation, executable, argv, "/", scope, network, nil)
}

func makeVicinaeRequestAt(operation, executable string, argv []string, cwd string, scope runner.Scope, network runner.NetworkPolicy, stdin []byte) (runner.CommandRequest, error) {
	request := runner.CommandRequest{
		Operation:    operation,
		Executable:   executable,
		Argv:         append([]string(nil), argv...),
		Cwd:          cwd,
		Stdin:        append([]byte(nil), stdin...),
		Scope:        scope,
		Network:      network,
		OutputPolicy: runner.OutputCaptureRedacted,
		Timeout:      runner.MaxTimeout,
		OutputLimit:  1 << 20,
	}
	if err := runner.ValidateCommandRequest(request); err != nil {
		return runner.CommandRequest{}, err
	}
	return request, nil
}

func vicinaeServiceOperationForRemoval(_ VicinaeServiceObservation) string {
	return vicinaeServiceDisableOperation
}

func appendUniqueStrings(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = appendUniqueStrings(result, value)
	}
	return result
}

func cloneVicinaeRequests(values []runner.CommandRequest) []runner.CommandRequest {
	if values == nil {
		return nil
	}
	result := make([]runner.CommandRequest, len(values))
	for i, value := range values {
		result[i] = value
		result[i].Argv = append([]string(nil), value.Argv...)
		result[i].Stdin = append([]byte(nil), value.Stdin...)
		if value.Env != nil {
			result[i].Env = make(map[string]string, len(value.Env))
			for key, item := range value.Env {
				result[i].Env[key] = item
			}
		}
	}
	return result
}

func cloneVicinaeFiles(values []ManagedFileDescriptor) []ManagedFileDescriptor {
	if values == nil {
		return nil
	}
	result := make([]ManagedFileDescriptor, len(values))
	for i, value := range values {
		result[i] = value.Clone()
	}
	return result
}

func cloneVicinaeSteps(values []planner.Step) []planner.Step {
	if values == nil {
		return nil
	}
	result := make([]planner.Step, len(values))
	for i, value := range values {
		result[i] = value
		result[i].DependsOn = append([]string(nil), value.DependsOn...)
		result[i].Desired = append([]byte(nil), value.Desired...)
		result[i].Observed = append([]byte(nil), value.Observed...)
		if value.Inverse != nil {
			inverse := *value.Inverse
			inverse.Value = append([]byte(nil), value.Inverse.Value...)
			result[i].Inverse = &inverse
		}
	}
	return result
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

// SHA256Hex exposes the same deterministic digest helper used in plan metadata.
func SHA256Hex(value []byte) string { return sha256Hex(value) }

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
