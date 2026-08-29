package gitx

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

type safetyCaptureRunner struct {
	base     Runner
	requests []CommandRequest
}

func (r *safetyCaptureRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	r.requests = append(r.requests, CommandRequest{Operation: request.Operation, Executable: request.Executable, Argv: append([]string(nil), request.Argv...), Cwd: request.Cwd})
	if r.base == nil {
		return CommandResult{}, nil
	}
	return r.base.Run(ctx, request)
}
func TestSafeRunnerRejectsUnsafeCommandsBeforeExecution(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		exec string
	}{
		{name: "force", argv: []string{"checkout", "--force", "main"}, exec: "git"},
		{name: "hard reset", argv: []string{"reset", "--hard", "HEAD"}, exec: "git"},
		{name: "clean", argv: []string{"clean", "-fd"}, exec: "git"},
		{name: "stash", argv: []string{"stash", "push"}, exec: "git"},
		{name: "commit", argv: []string{"commit", "-m", "nope"}, exec: "git"},
		{name: "shell executable", argv: []string{"-c", "git show HEAD:file"}, exec: "sh"},
		{name: "git shell config", argv: []string{"-c", "alias.show=!sh -c evil", "show", "HEAD:file"}, exec: "git"},
		{name: "forced tag refspec", argv: []string{"fetch", "origin", "+refs/heads/main:refs/tags/catalog-v1.0.0"}, exec: "git"},
		{name: "deleted tag refspec", argv: []string{"push", "origin", ":refs/tags/catalog-v1.0.0"}, exec: "git"},
		{name: "active tag checkout", argv: []string{"checkout", "catalog-v1.0.0"}, exec: "git"},
		{name: "tag ref checkout", argv: []string{"checkout", "refs/tags/catalog-v1.0.0"}, exec: "git"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &safetyCaptureRunner{}
			_, err := NewSafeRunner(capture).Run(context.Background(), CommandRequest{
				Operation: "test", Executable: tc.exec, Argv: tc.argv, Cwd: t.TempDir(),
			})
			if err == nil {
				t.Fatalf("unsafe command was accepted: %s %v", tc.exec, tc.argv)
			}
			if len(capture.requests) != 0 {
				t.Fatalf("unsafe command reached runner: %#v", capture.requests)
			}
		})
	}
}
func TestWU9aCommandsPassCentralSafetyValidation(t *testing.T) {
	remote, pin := newRemote(t)
	capture := &safetyCaptureRunner{base: ExecRunner{}}
	target := filepath.Join(t.TempDir(), "checkout")
	_, err := New(NewSafeRunner(capture)).EnsureCheckout(context.Background(), CheckoutSpec{
		Path: target, Remote: remote, DesiredCommit: pin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.requests) != 5 {
		t.Fatalf("captured commands = %#v, want five commands", capture.requests)
	}
	for i, request := range capture.requests {
		if err := ValidateGitCommand(request); err != nil {
			t.Fatalf("WU-9a command %d rejected: %v", i, err)
		}
		if request.Executable != "git" || !filepath.IsAbs(request.Cwd) {
			t.Fatalf("command %d = %#v, want an argv-only git request", i, request)
		}
	}
}
func TestObjectReadsReturnBytesWithoutChangingRepositoryState(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "--initial-branch=main")
	git(t, repo, "config", "user.email", "test@example.invalid")
	git(t, repo, "config", "user.name", "Gitx Test")
	catalog := []byte("catalog: one\n")
	if err := os.WriteFile(filepath.Join(repo, "catalog.yaml"), catalog, 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "catalog.yaml")
	git(t, repo, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(repo, "catalog.yaml"), []byte("user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "user-only.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	beforeHEAD := gitBytes(t, repo, "rev-parse", "HEAD")
	beforeIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	beforeStatus := gitBytes(t, repo, "status", "--porcelain=v1", "-z")

	capture := &safetyCaptureRunner{base: ExecRunner{}}
	reader := NewObjectReader(NewSafeRunner(capture))
	show, err := reader.Show(context.Background(), repo, "HEAD", "catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := reader.CatFile(context.Background(), repo, "HEAD:catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(show, catalog) || !bytes.Equal(cat, catalog) {
		t.Fatalf("object bytes = %q and %q, want %q", show, cat, catalog)
	}
	afterHEAD := gitBytes(t, repo, "rev-parse", "HEAD")
	afterIndex, err := os.ReadFile(filepath.Join(repo, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	afterStatus := gitBytes(t, repo, "status", "--porcelain=v1", "-z")
	if !bytes.Equal(beforeHEAD, afterHEAD) || !bytes.Equal(beforeIndex, afterIndex) || !bytes.Equal(beforeStatus, afterStatus) {
		t.Fatalf("object read changed repository state: HEAD %q/%q index %t status %q/%q", beforeHEAD, afterHEAD, bytes.Equal(beforeIndex, afterIndex), beforeStatus, afterStatus)
	}
	want := [][]string{{"show", "HEAD:catalog.yaml"}, {"cat-file", "-p", "HEAD:catalog.yaml"}}
	if len(capture.requests) != len(want) {
		t.Fatalf("object commands = %#v", capture.requests)
	}
	for i, request := range capture.requests {
		if err := ValidateGitCommand(request); err != nil || !slices.Equal(request.Argv, want[i]) {
			t.Fatalf("object command %d = %#v, want %v (%v)", i, request.Argv, want[i], err)
		}
	}
}
func TestObjectReadsRejectUnsafeReferencesWithoutRunningGit(t *testing.T) {
	capture := &safetyCaptureRunner{base: ExecRunner{}}
	reader := NewObjectReader(NewSafeRunner(capture))
	calls := []func() ([]byte, error){
		func() ([]byte, error) {
			return reader.Show(context.Background(), t.TempDir(), "HEAD; touch escaped", "catalog.yaml")
		},
		func() ([]byte, error) { return reader.Show(context.Background(), t.TempDir(), "HEAD", "../outside") },
		func() ([]byte, error) {
			return reader.CatFile(context.Background(), t.TempDir(), "HEAD; touch escaped")
		},
	}
	for _, call := range calls {
		if _, err := call(); err == nil {
			t.Fatal("unsafe object reference was accepted")
		}
	}
	if len(capture.requests) != 0 {
		t.Fatalf("unsafe object read reached runner: %#v", capture.requests)
	}
}
func gitBytes(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return output
}
