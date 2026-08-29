package gitx

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestObserveUpdateReportsNewerCommitWithoutChangingRepositoryState(t *testing.T) {
	remote := newUpdateRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	beforeHEAD := gitBytes(t, target, "rev-parse", "HEAD")
	beforeIndex, err := os.ReadFile(filepath.Join(target, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCatalog, err := os.ReadFile(filepath.Join(target, "catalog.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	beforeManifest, err := os.ReadFile(filepath.Join(target, "internal", "assets", "data", "source-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	candidate := pushUpdate(t, remote, "remote update\n")
	recorder := &recordingRunner{base: ExecRunner{}}
	got, err := New(NewSafeRunner(recorder)).ObserveUpdate(context.Background(), UpdateSpec{
		Path: target, Remote: remote,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentCommit != string(bytes.TrimSpace(beforeHEAD)) || got.CandidateCommit != candidate || !got.Newer {
		t.Fatalf("update result = %#v, want current %s candidate %s newer", got, bytes.TrimSpace(beforeHEAD), candidate)
	}
	if after := gitBytes(t, target, "rev-parse", "HEAD"); !bytes.Equal(beforeHEAD, after) {
		t.Fatalf("HEAD changed from %q to %q", beforeHEAD, after)
	}
	if after, err := os.ReadFile(filepath.Join(target, ".git", "index")); err != nil || !bytes.Equal(beforeIndex, after) {
		t.Fatalf("index changed: %v", err)
	}
	if after, err := os.ReadFile(filepath.Join(target, "catalog.yaml")); err != nil || !bytes.Equal(beforeCatalog, after) {
		t.Fatalf("catalog bytes changed: %v", err)
	}
	if after, err := os.ReadFile(filepath.Join(target, "internal", "assets", "data", "source-manifest.json")); err != nil || !bytes.Equal(beforeManifest, after) {
		t.Fatalf("source manifest changed: %v", err)
	}
	if len(recorder.requests) < 2 || !slices.Equal(recorder.requests[1].Argv, []string{"fetch", "--no-tags", "origin", "main"}) {
		t.Fatalf("update commands = %#v", recorder.requests)
	}
	for _, request := range recorder.requests {
		if len(request.Argv) != 0 && (request.Argv[0] == "checkout" || request.Argv[0] == "merge" || request.Argv[0] == "tag") {
			t.Fatalf("report-only update issued mutating command: %#v", request)
		}
	}
}

func TestObserveUpdateReportsNoNewerCandidateWhenRefsMatch(t *testing.T) {
	remote := newUpdateRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	current := git(t, target, "rev-parse", "HEAD")

	recorder := &recordingRunner{base: ExecRunner{}}
	got, err := New(NewSafeRunner(recorder)).ObserveUpdate(context.Background(), UpdateSpec{Path: target, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentCommit != current || got.CandidateCommit != current || got.Newer {
		t.Fatalf("matching update result = %#v", got)
	}
	for _, request := range recorder.requests {
		if len(request.Argv) > 0 && request.Argv[0] == "merge-base" {
			t.Fatalf("matching refs ran reachability check: %#v", recorder.requests)
		}
	}
}

func TestObserveUpdateRejectsDivergedRefsWithoutCheckoutMutation(t *testing.T) {
	remote := newUpdateRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	git(t, target, "config", "user.email", "test@example.invalid")
	git(t, target, "config", "user.name", "Gitx Test")
	if err := os.WriteFile(filepath.Join(target, "tracked.txt"), []byte("local branch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, target, "add", "tracked.txt")
	git(t, target, "commit", "-m", "local branch")
	before := git(t, target, "rev-parse", "HEAD")
	candidate := pushUpdate(t, remote, "remote branch\n")

	recorder := &recordingRunner{base: ExecRunner{}}
	got, err := New(NewSafeRunner(recorder)).ObserveUpdate(context.Background(), UpdateSpec{Path: target, Remote: remote})
	if err == nil || !strings.Contains(err.Error(), "diverged") {
		t.Fatalf("divergence result = %#v, err = %v", got, err)
	}
	if got.CurrentCommit != before || got.CandidateCommit != candidate || got.Newer {
		t.Fatalf("divergence update result = %#v", got)
	}
	if after := git(t, target, "rev-parse", "HEAD"); after != before {
		t.Fatalf("HEAD changed from %s to %s", before, after)
	}
	for _, request := range recorder.requests {
		if len(request.Argv) > 0 && (request.Argv[0] == "checkout" || request.Argv[0] == "merge" || request.Argv[0] == "tag") {
			t.Fatalf("divergence issued mutating command: %#v", request)
		}
	}
}

func TestObserveUpdateAcceptsLocalAheadAsNotNewer(t *testing.T) {
	remote := newUpdateRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	git(t, target, "config", "user.email", "test@example.invalid")
	git(t, target, "config", "user.name", "Gitx Test")
	if err := os.WriteFile(filepath.Join(target, "tracked.txt"), []byte("local ahead\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, target, "add", "tracked.txt")
	git(t, target, "commit", "-m", "local ahead")
	current := git(t, target, "rev-parse", "HEAD")
	candidate := git(t, target, "rev-parse", "HEAD~1")

	got, err := New(NewSafeRunner(&recordingRunner{base: ExecRunner{}})).ObserveUpdate(context.Background(), UpdateSpec{Path: target, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentCommit != current || got.CandidateCommit != candidate || got.Newer {
		t.Fatalf("local-ahead update result = %#v", got)
	}
}

func TestObserveUpdateRejectsCanonicalOriginMismatchBeforeFetch(t *testing.T) {
	remote := newUpdateRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	other := filepath.Join(t.TempDir(), "other.git")
	git(t, target, "remote", "set-url", "origin", other)
	recorder := &recordingRunner{base: ExecRunner{}}
	if _, err := New(NewSafeRunner(recorder)).ObserveUpdate(context.Background(), UpdateSpec{Path: target, Remote: remote}); err == nil {
		t.Fatal("mismatched origin was accepted")
	}
	if len(recorder.requests) != 1 || !slices.Equal(recorder.requests[0].Argv, []string{"remote", "get-url", "origin"}) {
		t.Fatalf("mismatch commands = %#v", recorder.requests)
	}
}

func newUpdateRemote(t *testing.T) string {
	t.Helper()
	remote, _ := newRemote(t)
	seed := filepath.Join(t.TempDir(), "seed")
	git(t, filepath.Dir(seed), "clone", "--branch", "main", remote, seed)
	if err := os.MkdirAll(filepath.Join(seed, "internal", "assets", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "catalog.yaml"), []byte("catalog: stable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, "internal", "assets", "data", "source-manifest.json"), []byte("{\"schema\":\"test\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "catalog.yaml", "internal/assets/data/source-manifest.json")
	git(t, seed, "commit", "-m", "seed update fixtures")
	git(t, seed, "push", "origin", "main")
	return remote
}
