package cachyos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"alex-cachyos/internal/adopt"
	"alex-cachyos/internal/executor"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
	"alex-cachyos/internal/safefile"
)

var (
	ErrUnsafeDesktopFile       = errors.New("unsafe desktop managed file")
	ErrDesktopRuntimeOperation = errors.New("desktop runtime operation unavailable")
)

const maxDesktopManagedFileBytes = int64(32 << 20)

// DesktopPackagePort returns typed package identities without exposing command
// output to planner or receipt values.
type DesktopPackagePort interface {
	Installed(context.Context, []string) (map[string]string, error)
}

// DesktopFilePort is the bounded publication seam for role-owned user files.
type DesktopFilePort interface {
	Observe(context.Context, string) (DesktopFileObservation, error)
	Publish(context.Context, ManagedFileDescriptor) error
}

// DesktopRuntime dispatches only operations declared by one immutable request
// plan. Command and file mutation remain separate behind this small interface.
type DesktopRuntime struct {
	steps       map[string]planner.Step
	files       map[string]ManagedFileDescriptor
	packages    []string
	commands    *executor.CommandMutator
	packagePort DesktopPackagePort
	filePort    DesktopFilePort
}

func NewDesktopRuntime(plan DesktopRequestPlan, commandRunner runner.Runner, network executor.NetworkPort, packages DesktopPackagePort, files DesktopFilePort) (*DesktopRuntime, error) {
	if packages == nil || files == nil {
		return nil, fmt.Errorf("%w: package and file ports are required", ErrDesktopRuntimeOperation)
	}
	if !canonicalDesktopPackages(plan.Packages) {
		return nil, fmt.Errorf("%w: package inventory is not canonical", ErrDesktopRuntimeOperation)
	}
	steps := make(map[string]planner.Step, len(plan.Steps))
	for _, step := range plan.Steps {
		if step.ID == "" || steps[step.ID].ID != "" {
			return nil, fmt.Errorf("%w: duplicate or empty step", ErrDesktopRuntimeOperation)
		}
		steps[step.ID] = step
	}
	fileIndex := make(map[string]ManagedFileDescriptor, len(plan.Files))
	for _, item := range plan.Files {
		if item.Operation == "" || fileIndex[item.Operation].Path != "" || steps[item.Operation].ID == "" || validateManagedFilePath(item.File.Path) != nil {
			return nil, fmt.Errorf("%w: invalid file operation", ErrDesktopRuntimeOperation)
		}
		fileIndex[item.Operation] = item.File.Clone()
	}
	var commands *executor.CommandMutator
	if len(plan.Requests) != 0 {
		var err error
		commands, err = executor.NewCommandMutator(commandRunner, plan.Requests, network)
		if err != nil {
			return nil, err
		}
	}
	return &DesktopRuntime{
		steps: steps, files: fileIndex, packages: append([]string(nil), plan.Packages...),
		commands: commands, packagePort: packages, filePort: files,
	}, nil
}

func (runtime *DesktopRuntime) Observe(ctx context.Context, step planner.Step) ([]byte, error) {
	if runtime == nil {
		return nil, ErrDesktopRuntimeOperation
	}
	want, ok := runtime.steps[step.ID]
	if !ok || !desktopStepMatches(step, want) {
		return nil, ErrDesktopRuntimeOperation
	}
	if step.ID == DesktopPackagesOperation {
		installed, err := runtime.packagePort.Installed(ctx, append([]string(nil), runtime.packages...))
		if err != nil {
			return nil, err
		}
		if len(missingDesktopPackages(runtime.packages, installed)) == 0 {
			return append([]byte(nil), want.Desired...), nil
		}
		return mustJSON(map[string]any{"installedVersions": versionSubset(installed, runtime.packages)}), nil
	}
	file, ok := runtime.files[step.ID]
	if !ok {
		return nil, ErrDesktopRuntimeOperation
	}
	observed, err := runtime.filePort.Observe(ctx, file.Path)
	if err != nil {
		return nil, err
	}
	desired := desktopObservationForDescriptor(file)
	if observed.Exists && observed.SHA256 == desired.SHA256 && observed.Mode == desired.Mode {
		return append([]byte(nil), want.Desired...), nil
	}
	return mustJSON(map[string]any{"exists": observed.Exists, "sha256": observed.SHA256, "mode": observed.Mode}), nil
}

func (runtime *DesktopRuntime) Mutate(ctx context.Context, step planner.Step) error {
	if runtime == nil || !desktopStepMatches(step, runtime.steps[step.ID]) {
		return ErrDesktopRuntimeOperation
	}
	if step.ID == DesktopPackagesOperation {
		if runtime.commands == nil {
			return ErrDesktopRuntimeOperation
		}
		return runtime.commands.Mutate(ctx, step)
	}
	file, ok := runtime.files[step.ID]
	if !ok {
		return ErrDesktopRuntimeOperation
	}
	return runtime.filePort.Publish(ctx, file.Clone())
}

func desktopStepMatches(got, want planner.Step) bool {
	return want.ID != "" && got.ID == want.ID && got.Module == want.Module && got.Scope == want.Scope && got.Network == want.Network && got.Operation == want.Operation && bytes.Equal(got.Desired, want.Desired)
}

// RunnerDesktopPackagePort is the production read-only Pacman observation
// adapter. Non-zero pacman -Q still yields any typed identities printed before
// missing packages; other failures fail closed.
type RunnerDesktopPackagePort struct{ Runner runner.Runner }

func (port RunnerDesktopPackagePort) Installed(ctx context.Context, names []string) (map[string]string, error) {
	if port.Runner == nil {
		return nil, errors.New("desktop package observer unavailable")
	}
	if !canonicalDesktopPackages(names) {
		return nil, errors.New("desktop package observation inventory is not canonical")
	}
	request, err := makeRequest("desktop.packages.observe", "/usr/bin/pacman", append([]string{"-Q"}, names...), runner.ScopeUser, runner.NetworkNone, nil)
	if err != nil {
		return nil, err
	}
	result, runErr := port.Runner.Run(ctx, request)
	if runErr != nil && result.ExitCode != 1 {
		return nil, errors.New("desktop package observation failed")
	}
	installed := make(map[string]string, len(names))
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(result.Stdout)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || !wanted[fields[0]] || installed[fields[0]] != "" {
			return nil, errors.New("desktop package observation was not canonical")
		}
		installed[fields[0]] = fields[1]
	}
	return installed, nil
}

func canonicalDesktopPackages(names []string) bool {
	if len(names) == 0 {
		return false
	}
	for i, name := range names {
		if !validPackageName(name) || (i != 0 && names[i-1] >= name) {
			return false
		}
	}
	return true
}

// OSDesktopFilePort publishes user-owned files through same-directory atomic
// rename. Existing regular files are adopted exactly once before replacement.
type OSDesktopFilePort struct {
	stateRoot string
	homeRoot  string
	receiptID string
}

func NewOSDesktopFilePort(stateRoot, homeRoot, receiptID string) (*OSDesktopFilePort, error) {
	if !canonicalDesktopRoot(stateRoot) || !canonicalDesktopRoot(homeRoot) || strings.TrimSpace(receiptID) == "" {
		return nil, fmt.Errorf("%w: state root, home root, and receipt identity are required", ErrUnsafeDesktopFile)
	}
	return &OSDesktopFilePort{stateRoot: stateRoot, homeRoot: homeRoot, receiptID: receiptID}, nil
}

func (port *OSDesktopFilePort) Observe(_ context.Context, path string) (DesktopFileObservation, error) {
	if port == nil || !desktopPathBelow(port.homeRoot, path) {
		return DesktopFileObservation{}, ErrUnsafeDesktopFile
	}
	return ObserveDesktopFile(path)
}

func ObserveDesktopFile(path string) (DesktopFileObservation, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return DesktopFileObservation{}, ErrUnsafeDesktopFile
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return DesktopFileObservation{Path: path}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return DesktopFileObservation{}, fmt.Errorf("%w: target is not a regular file", ErrUnsafeDesktopFile)
	}
	root, err := safefile.OpenRoot(filepath.Dir(path))
	if err != nil {
		return DesktopFileObservation{}, fmt.Errorf("%w: open parent", ErrUnsafeDesktopFile)
	}
	defer root.Close()
	result, err := root.ReadRegularFileWithMetadata(filepath.Base(path), maxDesktopManagedFileBytes)
	if err != nil {
		return DesktopFileObservation{}, fmt.Errorf("%w: read target", ErrUnsafeDesktopFile)
	}
	digest := sha256.Sum256(result.Data)
	return DesktopFileObservation{Path: path, Exists: true, SHA256: hex.EncodeToString(digest[:]), Mode: uint32(result.Metadata.Mode.Perm())}, nil
}

func (port *OSDesktopFilePort) Publish(ctx context.Context, file ManagedFileDescriptor) error {
	if port == nil || ctx == nil {
		return ErrUnsafeDesktopFile
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !desktopPathBelow(port.homeRoot, file.Path) || file.Mode == 0 || file.Mode&^uint32(0o777) != 0 || int64(len(file.Content)) > maxDesktopManagedFileBytes {
		return ErrUnsafeDesktopFile
	}
	if err := ensureDesktopParent(filepath.Dir(file.Path)); err != nil {
		return err
	}
	info, err := os.Lstat(file.Path)
	switch {
	case err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0):
		return fmt.Errorf("%w: target is not a regular file", ErrUnsafeDesktopFile)
	case err == nil:
		if _, err := adopt.NewStore(port.stateRoot).Adopt(file.Path, port.receiptID); err != nil {
			return fmt.Errorf("adopt desktop file: %w", err)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("%w: inspect target", ErrUnsafeDesktopFile)
	}
	return atomicPublishDesktopFile(file)
}

func canonicalDesktopRoot(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path && path != string(filepath.Separator) && !strings.ContainsAny(path, "\x00\r\n")
}

func desktopPathBelow(root, path string) bool {
	if !canonicalDesktopRoot(root) || path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return false
	}
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func ensureDesktopParent(parent string) error {
	if !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return ErrUnsafeDesktopFile
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create desktop parent: %w", err)
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(parent, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: unsafe parent", ErrUnsafeDesktopFile)
		}
	}
	return nil
}

func atomicPublishDesktopFile(file ManagedFileDescriptor) (err error) {
	parent := filepath.Dir(file.Path)
	temporary, err := os.CreateTemp(parent, ".alex-cachyos-*")
	if err != nil {
		return fmt.Errorf("create desktop candidate: %w", err)
	}
	name := temporary.Name()
	remove := true
	defer func() {
		_ = temporary.Close()
		if remove {
			_ = os.Remove(name)
		}
	}()
	if err := temporary.Chmod(os.FileMode(file.Mode)); err != nil {
		return err
	}
	written, err := io.Copy(temporary, bytes.NewReader(file.Content))
	if err != nil || written != int64(len(file.Content)) {
		return errors.New("write desktop candidate failed")
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if info, statErr := os.Lstat(file.Path); statErr == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return ErrUnsafeDesktopFile
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return ErrUnsafeDesktopFile
	}
	if err := os.Rename(name, file.Path); err != nil {
		return fmt.Errorf("publish desktop file: %w", err)
	}
	remove = false
	directory, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
