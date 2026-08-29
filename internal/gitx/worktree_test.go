package gitx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDetachedWorktreeIsStateOwnedAndMarked(t *testing.T) {
	repo, pin := localRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	recorder := &recordingRunner{base: ExecRunner{}}
	manager := NewWorktreeManager(recorder, state)
	destination := filepath.Join(state, "worktrees", "candidate")
	got, err := manager.Create(context.Background(), WorktreeSpec{Repo: repo, Destination: destination, Commit: pin})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || !got.Detached || got.Path != destination || got.Commit != pin {
		t.Fatalf("create result = %#v", got)
	}
	marker, err := os.ReadFile(manager.MarkerPath(got.Path))
	if err != nil || string(marker) != WorktreeMarkerContents || len(marker) > 128 {
		t.Fatalf("marker = %q, %v", marker, err)
	}
	if head := git(t, got.Path, "rev-parse", "--abbrev-ref", "HEAD"); head != "HEAD" {
		t.Fatalf("worktree is attached to %q", head)
	}
	assertSafeCommands(t, recorder.requests)
	assertCommands(t, recorder.requests, [][]string{{"worktree", "add", "--detach", destination, pin}})
}
func TestCreateRejectsUnownedExistingDestinationsAndSymlinks(t *testing.T) {
	repo, pin := localRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	manager := NewWorktreeManager(ExecRunner{}, state)
	unowned := filepath.Join(state, "unowned")
	if err := os.MkdirAll(unowned, 0700); err != nil {
		t.Fatal(err)
	}
	before := []byte("user bytes")
	if err := os.WriteFile(filepath.Join(unowned, "keep"), before, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), WorktreeSpec{Repo: repo, Destination: unowned, Commit: pin}); !errors.Is(err, ErrUnownedWorktree) {
		t.Fatalf("unowned destination error = %v", err)
	}
	if after, err := os.ReadFile(filepath.Join(unowned, "keep")); err != nil || string(after) != string(before) {
		t.Fatalf("unowned bytes changed: %q, %v", after, err)
	}
	real := filepath.Join(state, "real")
	if err := os.Mkdir(real, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(state, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(context.Background(), WorktreeSpec{Repo: repo, Destination: link, Commit: pin}); !errors.Is(err, ErrUnsafeWorktreeDestination) {
		t.Fatalf("symlink destination error = %v", err)
	}
}
func TestCleanupSkipsDirtyOrUnownedAndRemovesOnlyOwnedCleanWorktrees(t *testing.T) {
	repo, pin := localRepo(t)
	state := filepath.Join(t.TempDir(), "state")
	recorder := &recordingRunner{base: ExecRunner{}}
	manager := NewWorktreeManager(recorder, state)
	destination := filepath.Join(state, "owned")
	if _, err := manager.Create(context.Background(), WorktreeSpec{Repo: repo, Destination: destination, Commit: pin}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "user-only"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	dirty, err := manager.Cleanup(context.Background(), destination)
	if err != nil || !dirty.Skipped || dirty.Reason != CleanupDirty {
		t.Fatalf("dirty cleanup = %#v, %v", dirty, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "user-only")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(destination, "user-only")); err != nil {
		t.Fatal(err)
	}
	removed, err := manager.Cleanup(context.Background(), destination)
	if err != nil || !removed.Removed || removed.Skipped {
		t.Fatalf("clean cleanup = %#v, %v", removed, err)
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination remains: %v", err)
	}
	unowned := filepath.Join(state, "unowned")
	if err := os.Mkdir(unowned, 0700); err != nil {
		t.Fatal(err)
	}
	skipped, err := manager.Cleanup(context.Background(), unowned)
	if err != nil || !skipped.Skipped || skipped.Reason != CleanupUnowned {
		t.Fatalf("unowned cleanup = %#v, %v", skipped, err)
	}
}
func localRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "--initial-branch=main")
	git(t, repo, "config", "user.email", "test@example.invalid")
	git(t, repo, "config", "user.name", "Gitx Test")
	if err := os.WriteFile(filepath.Join(repo, "tracked"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "tracked")
	git(t, repo, "commit", "-m", "initial")
	return repo, git(t, repo, "rev-parse", "HEAD")
}
