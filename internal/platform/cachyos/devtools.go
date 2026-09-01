package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
	"alex-cachyos/templates"
)

const (
	DevtoolsModuleName  = "devtools"
	DevtoolsMarkerBegin = "# >>> alex-cachyos/devtools >>>"
	DevtoolsMarkerEnd   = "# <<< alex-cachyos/devtools <<<"
)

var ErrInvalidDevtoolsObservation = errors.New("invalid devtools observation")

type DevtoolsFileObservation struct {
	Path       string
	Exists     bool
	SHA256     string
	Mode       uint32
	Content    []byte
	Ownership  FileOwnershipKind
	BackupPath string
}

type DevtoolsObservation struct {
	HomeRoot           string
	InstalledPackages  map[string]string
	ToolchainConverged bool
	Files              map[string]DevtoolsFileObservation
	ShellFiles         map[string]DevtoolsFileObservation
}

type DevtoolsRequestPlan struct {
	Requests []runner.CommandRequest
	Files    []ManagedFileDescriptor
	Steps    []planner.Step
}

func BuildDevtoolsRequestPlan(observation DevtoolsObservation) (DevtoolsRequestPlan, error) {
	home, err := validateDevtoolsHome(observation.HomeRoot)
	if err != nil {
		return DevtoolsRequestPlan{}, err
	}
	files, err := buildDevtoolsFiles(home, observation.ShellFiles)
	if err != nil {
		return DevtoolsRequestPlan{}, err
	}
	result := DevtoolsRequestPlan{Files: cloneVicinaeFiles(files)}

	miseInstalled := hasPackage(observation.InstalledPackages, "mise")
	miseRequest, err := makeRequest("devtools.mise.install", "/usr/bin/pacman", []string{"-S", "--needed", "--noconfirm", "mise"}, runner.ScopeSystem, runner.NetworkRequired, nil)
	if err != nil {
		return DevtoolsRequestPlan{}, err
	}
	result.Steps = append(result.Steps, moduleRequestStep(DevtoolsModuleName, miseRequest, convergedDisposition(miseInstalled), map[string]any{
		"requestedNames":           []string{"mise"},
		"pacmanRepositoryPolicy":   BootstrapPacmanRepositoryPolicy,
		"pacmanTransactionPolicy":  "name-presence",
		"explicitPackageOwnership": true,
	}, map[string]any{"beforeVersions": versionSubset(observation.InstalledPackages, []string{"mise"})}, "devtools.mise.remove"))
	if !miseInstalled {
		result.Requests = append(result.Requests, miseRequest)
	}

	for _, file := range files {
		observed := devtoolsFileObservation(file.Path, observation)
		step, err := managedFileStep(DevtoolsModuleName, "devtools.file."+devtoolsFileID(home, file.Path), file, observed, []string{"devtools.mise.install"})
		if err != nil {
			return DevtoolsRequestPlan{}, err
		}
		result.Steps = append(result.Steps, step)
	}

	toolchainRequest, err := makeRequest("devtools.toolchain.install", "/usr/bin/mise", []string{"install"}, runner.ScopeUser, runner.NetworkRequired, nil)
	if err != nil {
		return DevtoolsRequestPlan{}, err
	}
	toolchainStep := moduleRequestStep(DevtoolsModuleName, toolchainRequest, convergedDisposition(observation.ToolchainConverged), map[string]any{
		"tools": []string{"node@lts", "npm@latest", "pnpm@latest"},
	}, map[string]any{"converged": observation.ToolchainConverged}, "devtools.toolchain.restore")
	for _, file := range files {
		toolchainStep.DependsOn = append(toolchainStep.DependsOn, "devtools.file."+devtoolsFileID(home, file.Path))
	}
	if !observation.ToolchainConverged {
		result.Requests = append(result.Requests, toolchainRequest)
	}
	result.Steps = append(result.Steps, toolchainStep)
	result.Requests = cloneVicinaeRequests(result.Requests)
	result.Steps = cloneVicinaeSteps(result.Steps)
	return result, nil
}

func BuildDevtoolsModule(observation DevtoolsObservation) (planner.Module, error) {
	requestPlan, err := BuildDevtoolsRequestPlan(observation)
	if err != nil {
		return planner.Module{}, err
	}
	return planner.Module{Name: DevtoolsModuleName, Enabled: true, Steps: requestPlan.Steps}, nil
}

func BuildDevtoolsPlan(observation DevtoolsObservation) (planner.Plan, error) {
	module, err := BuildDevtoolsModule(observation)
	if err != nil {
		return planner.Plan{}, err
	}
	return planner.BuildPlan([]planner.Module{module}, planner.Selection{Only: []string{DevtoolsModuleName}})
}

func buildDevtoolsFiles(home string, shells map[string]DevtoolsFileObservation) ([]ManagedFileDescriptor, error) {
	read := func(name string) ([]byte, error) {
		value, err := fs.ReadFile(templates.FS, name)
		if err != nil {
			return nil, fmt.Errorf("read embedded devtools asset %q: %w", name, err)
		}
		return append([]byte(nil), value...), nil
	}
	mise, err := read("devtools/mise.config.toml")
	if err != nil {
		return nil, err
	}
	pnpm, err := read("devtools/pnpm.config.yaml")
	if err != nil {
		return nil, err
	}
	npm, err := read("devtools/npmrc")
	if err != nil {
		return nil, err
	}
	pnpm = []byte(strings.ReplaceAll(string(pnpm), "@HOME@", filepath.ToSlash(home)))
	files := []ManagedFileDescriptor{
		newManagedFile(filepath.Join(home, ".config", "mise", "config.toml"), mise, 0o644, "mise-config"),
		newManagedFile(filepath.Join(home, ".config", "pnpm", "config.yaml"), pnpm, 0o644, "pnpm-config"),
		newManagedFile(filepath.Join(home, ".npmrc"), npm, 0o644, "npm-config"),
	}
	for _, shell := range []struct {
		path string
		body string
	}{
		{filepath.Join(home, ".zshrc"), `eval "$(mise activate zsh)"
# After mise: global CLIs; mise keeps winning for node/pnpm
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$PATH:$HOME/.local/bin" ;;
esac`},
		{filepath.Join(home, ".bashrc"), `eval "$(mise activate bash)"
# After mise: global CLIs; mise keeps winning for node/pnpm
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$PATH:$HOME/.local/bin" ;;
esac`},
		{filepath.Join(home, ".config", "fish", "config.fish"), `mise activate fish | source
# After mise: global CLIs (pnpm add -g → here), mise keeps winning for node/pnpm
fish_add_path --append --path "$HOME/.local/bin"`},
	} {
		observed := shells[shell.path]
		content, err := upsertDevtoolsMarker(observed.Content, shell.body)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrInvalidDevtoolsObservation, shell.path, err)
		}
		files = append(files, newManagedFile(shell.path, content, 0o644, "shell-marker"))
	}
	return files, nil
}

func upsertDevtoolsMarker(existing []byte, body string) ([]byte, error) {
	if !utf8.Valid(existing) {
		return nil, errors.New("shell file is not UTF-8")
	}
	text := string(existing)
	if strings.Count(text, DevtoolsMarkerBegin) > 1 || strings.Count(text, DevtoolsMarkerEnd) > 1 {
		return nil, errors.New("duplicate marker block")
	}
	begin := strings.Index(text, DevtoolsMarkerBegin)
	end := strings.Index(text, DevtoolsMarkerEnd)
	if (begin >= 0) != (end >= 0) || begin >= 0 && end < begin {
		return nil, errors.New("incomplete marker block")
	}
	if begin >= 0 {
		after := end + len(DevtoolsMarkerEnd)
		text = strings.TrimRight(text[:begin], "\n") + strings.TrimLeft(text[after:], "\n")
	}
	block := DevtoolsMarkerBegin + "\n" + strings.TrimSpace(body) + "\n" + DevtoolsMarkerEnd + "\n"
	if strings.TrimSpace(text) == "" {
		return []byte(block), nil
	}
	return []byte(strings.TrimRight(text, "\n") + "\n\n" + block), nil
}

func validateDevtoolsHome(home string) (string, error) {
	if home == "" || !filepath.IsAbs(home) || filepath.Clean(home) != home || !utf8.ValidString(home) || hasControl(home) {
		return "", fmt.Errorf("%w: home root must be a clean absolute path", ErrInvalidDevtoolsObservation)
	}
	return home, nil
}

func devtoolsFileObservation(path string, observation DevtoolsObservation) DevtoolsFileObservation {
	if value, ok := observation.Files[path]; ok {
		return value
	}
	return observation.ShellFiles[path]
}

func devtoolsFileID(home, path string) string {
	relative, _ := filepath.Rel(home, path)
	replacer := strings.NewReplacer("/", "-", "\\", "-", ".", "-")
	return strings.Trim(replacer.Replace(relative), "-")
}

func managedFileStep(module, id string, file ManagedFileDescriptor, observation DevtoolsFileObservation, dependsOn []string) (planner.Step, error) {
	if observation.Path != "" && observation.Path != file.Path {
		return planner.Step{}, fmt.Errorf("%w: observed path differs from desired target", ErrInvalidDevtoolsObservation)
	}
	digest := sha256.Sum256(file.Content)
	wantSHA := hex.EncodeToString(digest[:])
	converged := observation.Exists && strings.EqualFold(observation.SHA256, wantSHA) && observation.Mode == file.Mode
	inverseOperation := "remove-file"
	inverse := map[string]any{"path": file.Path}
	if observation.Exists {
		inverseOperation = "restore-backup"
		inverse = map[string]any{"path": file.Path, "backup": observation.BackupPath}
	}
	return planner.Step{
		ID: id, Module: module, DependsOn: append([]string(nil), dependsOn...), Description: "write managed " + file.Kind,
		Scope: planner.ScopeUser, Network: planner.NetworkNone, Operation: planner.Operation("managed-file.publish"),
		Disposition: convergedDisposition(converged),
		Desired:     mustJSON(map[string]any{"path": file.Path, "sha256": wantSHA, "mode": file.Mode, "kind": file.Kind}),
		Observed:    mustJSON(map[string]any{"exists": observation.Exists, "sha256": observation.SHA256, "mode": observation.Mode, "ownership": observation.Ownership}),
		Inverse:     &planner.InverseDescriptor{Operation: planner.Operation(inverseOperation), Value: mustJSON(inverse)},
	}, nil
}

func moduleRequestStep(module string, request runner.CommandRequest, disposition planner.Disposition, desired, observed map[string]any, inverse string) planner.Step {
	desiredCopy := make(map[string]any, len(desired)+1)
	for key, value := range desired {
		desiredCopy[key] = value
	}
	desiredCopy["request"] = commandIdentity(request)
	return planner.Step{
		ID: request.Operation, Module: module, Description: request.Operation,
		Scope: planner.Scope(request.Scope), Network: planner.NetworkClass(request.Network), Operation: planner.Operation(request.Operation), Disposition: disposition,
		Desired: mustJSON(desiredCopy), Observed: mustJSON(observed),
		Inverse: &planner.InverseDescriptor{Operation: planner.Operation(inverse), Value: mustJSON(map[string]any{"before": observed})},
	}
}

func sortedStringMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
