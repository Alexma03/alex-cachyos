package gitx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type recordingRunner struct {
	base     Runner
	requests []CommandRequest
}

func (r *recordingRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	request.Argv = append([]string(nil), request.Argv...)
	r.requests = append(r.requests, request)
	return r.base.Run(ctx, request)
}
func TestCloneMissingCheckoutUsesSafeObservationCommands(t *testing.T) {
	remote, pin := newRemote(t)
	recorder := &recordingRunner{base: ExecRunner{}}
	target := filepath.Join(t.TempDir(), "checkout")

	got, err := New(recorder).EnsureCheckout(context.Background(), CheckoutSpec{Path: target, Remote: remote, DesiredCommit: pin})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Cloned || got.Adopted || got.ResolvedCommit != pin || got.Dirty || got.DirtyCounts.Total() != 0 {
		t.Fatalf("clone result = %#v", got)
	}
	assertSafeCommands(t, recorder.requests)
	assertCommands(t, recorder.requests, [][]string{
		{"clone", "--branch", "main", remote, target},
		{"fetch", "--no-tags", "origin", "main"},
		{"rev-parse", "HEAD"},
		{"status", "--porcelain=v1", "-z"},
		{"merge-base", "--is-ancestor", pin, "origin/main"},
	})
	recorder.requests = nil
	badPin := strings.Repeat("0", 40)
	if _, err := New(recorder).EnsureCheckout(context.Background(), CheckoutSpec{Path: target, Remote: remote, DesiredCommit: badPin}); err == nil {
		t.Fatal("unreachable desired commit was accepted")
	}
	assertSafeCommands(t, recorder.requests)
	if len(recorder.requests) == 0 || !slices.Equal(recorder.requests[len(recorder.requests)-1].Argv, []string{"merge-base", "--is-ancestor", badPin, "origin/main"}) {
		t.Fatalf("reachability command missing: %#v", recorder.requests)
	}
}
func TestAdoptExistingCheckoutRequiresCanonicalOriginAndCountsDirtyState(t *testing.T) {
	remote, pin := newRemote(t)
	target := filepath.Join(t.TempDir(), "checkout")
	git(t, filepath.Dir(target), "clone", "--branch", "main", remote, target)
	tracked := filepath.Join(target, "tracked.txt")
	before, err := os.ReadFile(tracked)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tracked, append(before, '\n', 'u', 's', 'e', 'r'), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := filepath.Join(target, "user-only.txt")
	if err := os.WriteFile(untracked, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}

	recorder := &recordingRunner{base: ExecRunner{}}
	got, err := New(recorder).EnsureCheckout(context.Background(), CheckoutSpec{
		Path:          target,
		Remote:        "file://" + filepath.ToSlash(remote),
		Branch:        "main",
		DesiredCommit: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cloned || !got.Adopted || got.ResolvedCommit != pin || !got.Dirty || got.DirtyCounts.Modified != 1 || got.DirtyCounts.Untracked != 1 {
		t.Fatalf("adopt result = %#v", got)
	}
	if got.DirtyCounts.Total() != 2 {
		t.Fatalf("dirty counts = %#v", got.DirtyCounts)
	}
	if after, err := os.ReadFile(tracked); err != nil || string(after) != string(append(before, '\n', 'u', 's', 'e', 'r')) {
		t.Fatalf("tracked bytes changed: %q, %v", after, err)
	}
	assertSafeCommands(t, recorder.requests)
	assertCommands(t, recorder.requests, [][]string{
		{"remote", "get-url", "origin"},
		{"fetch", "--no-tags", "origin", "main"},
		{"rev-parse", "HEAD"},
		{"status", "--porcelain=v1", "-z"},
		{"merge-base", "--is-ancestor", pin, "origin/main"},
	})

	git(t, target, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "other.git"))
	mismatchRecorder := &recordingRunner{base: ExecRunner{}}
	if _, err := New(mismatchRecorder).EnsureCheckout(context.Background(), CheckoutSpec{
		Path:          target,
		Remote:        remote,
		Branch:        "main",
		DesiredCommit: pin,
	}); err == nil {
		t.Fatal("checkout with a mismatched origin was adopted")
	}
	if len(mismatchRecorder.requests) != 1 || !slices.Equal(mismatchRecorder.requests[0].Argv, []string{"remote", "get-url", "origin"}) {
		t.Fatalf("mismatch commands = %#v", mismatchRecorder.requests)
	}
}
func TestParsePorcelainCountsNeverRetainsPaths(t *testing.T) {
	got := ParseStatusPorcelain([]byte(" M tracked-secret.txt\x00?? untracked-secret.txt\x00R  old-secret.txt\x00new-secret.txt\x00UU conflict-secret.txt\x00"))
	if got.Entries != 4 || got.Modified != 1 || got.Untracked != 1 || got.Renamed != 1 || got.Unmerged != 1 || got.Total() != 4 {
		t.Fatalf("status counts = %#v", got)
	}
	if strings.Contains(fmt.Sprintf("%#v", got), "secret") {
		t.Fatalf("status counts retained path data: %#v", got)
	}
}
func assertCommands(t *testing.T, requests []CommandRequest, want [][]string) {
	t.Helper()
	if len(requests) != len(want) {
		t.Fatalf("commands = %#v, want %d commands", requests, len(want))
	}
	for i := range want {
		if requests[i].Executable != "git" || !slices.Equal(requests[i].Argv, want[i]) || !filepath.IsAbs(requests[i].Cwd) {
			t.Fatalf("command %d = %#v, want git %v", i, requests[i], want[i])
		}
	}
}
func assertSafeCommands(t *testing.T, requests []CommandRequest) {
	t.Helper()
	for _, request := range requests {
		command := strings.Join(append([]string{request.Executable}, request.Argv...), " ")
		for _, forbidden := range []string{"--force", "reset --hard", "clean", "stash", "commit"} {
			if strings.Contains(command, forbidden) {
				t.Fatalf("forbidden git command %q in %q", forbidden, command)
			}
		}
	}
}
func newRemote(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	git(t, root, "init", "--bare", "--initial-branch=main", remote)
	git(t, root, "init", "--initial-branch=main", source)
	git(t, source, "config", "user.email", "test@example.invalid")
	git(t, source, "config", "user.name", "Gitx Test")
	if err := os.WriteFile(filepath.Join(source, "tracked.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "tracked.txt")
	git(t, source, "commit", "-m", "initial")
	git(t, source, "remote", "add", "origin", remote)
	git(t, source, "push", "origin", "main")
	return remote, git(t, source, "rev-parse", "HEAD")
}
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
