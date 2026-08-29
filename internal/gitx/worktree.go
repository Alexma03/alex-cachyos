package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"alex-cachyos/internal/statepath"
)

const (
	WorktreeMarkerName     = ".alex-cachyos-worktree"
	WorktreeMarkerContents = "alex-cachyos-worktree/v1\n"
)

var (
	ErrUnownedWorktree           = errors.New("worktree destination is not configurator-owned")
	ErrUnsafeWorktreeDestination = errors.New("unsafe worktree destination")
)

type WorktreeSpec struct {
	Repo, Destination, Commit, StateHome string
}
type WorktreeResult struct {
	Path, Commit              string
	Created, Reused, Detached bool
}
type CleanupReason string

const (
	CleanupUnowned CleanupReason = "unowned"
	CleanupSymlink CleanupReason = "symlink"
	CleanupDirty   CleanupReason = "dirty"
	CleanupFailed  CleanupReason = "operation-failed"
)

type CleanupResult struct {
	Path             string
	Removed, Skipped bool
	Reason           CleanupReason
	DirtyCounts      DirtyCounts
}
type WorktreeManager struct {
	runner    Runner
	stateHome string
}

func NewWorktreeManager(runner Runner, stateHome string) *WorktreeManager {
	return &WorktreeManager{runner: runner, stateHome: stateHome}
}
func (m *WorktreeManager) MarkerPath(destination string) string {
	return WorktreeMarkerPath(m.stateHome, destination)
}
func WorktreeMarkerPath(_ string, destination string) string {
	return filepath.Clean(destination) + WorktreeMarkerName
}

func (m *WorktreeManager) Create(ctx context.Context, spec WorktreeSpec) (WorktreeResult, error) {
	var result WorktreeResult
	if m == nil || m.runner == nil {
		return result, errors.New("nil worktree manager")
	}
	root, err := m.resolveStateHome(spec.StateHome)
	if err != nil {
		return result, err
	}
	if err := validateLocalRepo(spec.Repo); err != nil {
		return result, err
	}
	if err := validateRevisionForWorktree(spec.Commit); err != nil {
		return result, err
	}
	destination := spec.Destination
	destination, err = validateDestination(root, destination)
	if err != nil {
		return result, err
	}
	if err := ensureStateHome(root); err != nil {
		return result, err
	}
	if err := ensureNoSymlinkPath(root, filepath.Dir(destination)); err != nil {
		return result, err
	}
	marker := WorktreeMarkerPath(root, destination)
	info, statErr := os.Lstat(destination)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return result, ErrUnsafeWorktreeDestination
		}
		if !validMarker(marker) {
			return result, ErrUnownedWorktree
		}
		return WorktreeResult{Path: destination, Commit: spec.Commit, Reused: true, Detached: true}, nil
	}
	if !os.IsNotExist(statErr) {
		return result, fmt.Errorf("inspect worktree destination: %w", statErr)
	}
	if _, err := os.Lstat(marker); err == nil {
		return result, ErrUnownedWorktree
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return result, fmt.Errorf("create worktree parent: %w", err)
	}
	if _, err := m.run(ctx, spec.Repo, "worktree-add", "worktree", "add", "--detach", destination, spec.Commit); err != nil {
		return result, err
	}
	if err := ensureMarker(marker); err != nil {
		return result, err
	}
	return WorktreeResult{Path: destination, Commit: spec.Commit, Created: true, Detached: true}, nil
}

func (m *WorktreeManager) Cleanup(ctx context.Context, destination string) (CleanupResult, error) {
	result := CleanupResult{Path: filepath.Clean(destination)}
	if m == nil || m.runner == nil {
		return result, errors.New("nil worktree manager")
	}
	root, err := m.resolveStateHome("")
	if err != nil {
		return result, err
	}
	if _, err := validateDestination(root, result.Path); err != nil {
		return result, err
	}
	if err := ensureNoSymlinkPath(root, filepath.Dir(result.Path)); err != nil {
		return result, err
	}
	info, err := os.Lstat(result.Path)
	if os.IsNotExist(err) {
		return skipped(result, CleanupUnowned), nil
	}
	if err != nil {
		return result, fmt.Errorf("inspect worktree destination: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return skipped(result, CleanupSymlink), nil
	}
	if !info.IsDir() || !validMarker(WorktreeMarkerPath(root, result.Path)) {
		return skipped(result, CleanupUnowned), nil
	}
	status, err := m.run(ctx, result.Path, "worktree-status", "status", "--porcelain=v1", "-z", "--ignored")
	if err != nil {
		return skipped(result, CleanupFailed), nil
	}
	result.DirtyCounts = ParseStatusPorcelain(status.Stdout)
	if result.DirtyCounts.Total() != 0 {
		return skipped(result, CleanupDirty), nil
	}
	if _, err := m.run(ctx, result.Path, "worktree-remove", "worktree", "remove", result.Path); err != nil {
		return skipped(result, CleanupFailed), nil
	}
	if _, err := os.Lstat(result.Path); err == nil || !os.IsNotExist(err) {
		return skipped(result, CleanupFailed), nil
	}
	if err := os.Remove(WorktreeMarkerPath(root, result.Path)); err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("remove worktree marker: %w", err)
	}
	result.Removed = true
	return result, nil
}

func skipped(result CleanupResult, reason CleanupReason) CleanupResult {
	result.Skipped, result.Reason = true, reason
	return result
}
func (m *WorktreeManager) resolveStateHome(explicit string) (string, error) {
	root := explicit
	if root == "" && m != nil {
		root = m.stateHome
	}
	if root == "" {
		paths, err := statepath.Resolve()
		if err != nil {
			return "", fmt.Errorf("resolve worktree state home: %w", err)
		}
		root = paths.StateHome
	}
	if !filepath.IsAbs(root) {
		return "", errors.New("worktree state home must be absolute")
	}
	root = filepath.Clean(root)
	if home, err := os.UserHomeDir(); err == nil && withinPath(root, filepath.Join(home, ".pi")) {
		return "", errors.New("worktree state home must not use Pi state")
	}
	return root, nil
}
func validateLocalRepo(repo string) error {
	if repo == "" || !filepath.IsAbs(repo) {
		return errors.New("worktree repository must be an absolute local path")
	}
	info, err := os.Lstat(filepath.Clean(repo))
	if err != nil {
		return fmt.Errorf("inspect worktree repository: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("worktree repository must be a local directory")
	}
	return nil
}
func validateRevisionForWorktree(revision string) error {
	if revision == "" || strings.TrimSpace(revision) != revision || strings.HasPrefix(revision, "-") || strings.IndexByte(revision, 0) >= 0 || strings.ContainsAny(revision, ";|&`$()<>\r\n") {
		return errors.New("unsafe worktree revision")
	}
	return nil
}
func validateDestination(root, destination string) (string, error) {
	if destination == "" || !filepath.IsAbs(destination) {
		return "", ErrUnsafeWorktreeDestination
	}
	destination = filepath.Clean(destination)
	if !withinPath(root, destination) || destination == root {
		return "", ErrUnsafeWorktreeDestination
	}
	return destination, nil
}
func ensureStateHome(root string) error {
	if err := os.MkdirAll(root, 0700); err != nil {
		return fmt.Errorf("create worktree state home: %w", err)
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("worktree state home must be a directory")
	}
	return os.Chmod(root, 0700)
}
func ensureNoSymlinkPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnsafeWorktreeDestination
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return fmt.Errorf("inspect worktree path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeWorktreeDestination
		}
	}
	return nil
}
func validMarker(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() != int64(len(WorktreeMarkerContents)) {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && string(data) == WorktreeMarkerContents
}
func ensureMarker(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create worktree marker directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create worktree marker: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(WorktreeMarkerContents); err != nil {
		return fmt.Errorf("write worktree marker: %w", err)
	}
	return file.Sync()
}
func (m *WorktreeManager) run(ctx context.Context, cwd, operation string, argv ...string) (CommandResult, error) {
	request := CommandRequest{Operation: operation, Executable: "git", Argv: append([]string(nil), argv...), Cwd: cwd}
	result, err := NewSafeRunner(m.runner).Run(ctx, request)
	if err != nil || result.ExitCode != 0 {
		return result, errors.New("git worktree command failed")
	}
	return result, nil
}
func withinPath(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
