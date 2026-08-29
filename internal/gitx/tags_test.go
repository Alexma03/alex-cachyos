package gitx

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateCatalogTagIsAnnotatedAndDescribeVisible(t *testing.T) {
	repo, head := newTagRepo(t)
	recorder := &recordingRunner{base: ExecRunner{}}
	client := New(NewSafeRunner(recorder))
	got, err := client.CreateTag(context.Background(), repo, TagSpec{
		Name:        "catalog-v1.2.3",
		Message:     "initial catalog",
		TaggerName:  "Gitx Test",
		TaggerEmail: "test@example.invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "catalog-v1.2.3" || got.Commit != head || !got.DescribeVisible {
		t.Fatalf("tag result = %#v", got)
	}
	if typ := git(t, repo, "cat-file", "-t", "refs/tags/catalog-v1.2.3"); typ != "tag" {
		t.Fatalf("tag object type = %q, want annotated tag", typ)
	}
	if target := git(t, repo, "rev-parse", "refs/tags/catalog-v1.2.3^{}"); target != head {
		t.Fatalf("tag target = %q, want HEAD %q", target, head)
	}
	if described := git(t, repo, "describe", "--exact-match", "--tags", "HEAD"); described != "catalog-v1.2.3" {
		t.Fatalf("git describe = %q", described)
	}
	object := git(t, repo, "cat-file", "-p", "refs/tags/catalog-v1.2.3")
	if !strings.Contains(object, "tagger Gitx Test <test@example.invalid> ") || !strings.HasSuffix(object, "initial catalog") {
		t.Fatalf("tag object lacks explicit identity/message: %q", object)
	}
	assertTagCommands(t, recorder.requests)
}

func TestCreateCatalogTagValidatesInputsAndRefusesExistingTag(t *testing.T) {
	repo, _ := newTagRepo(t)
	cases := []TagSpec{
		{Name: "catalog-v1.2", Message: "message", TaggerName: "Gitx Test", TaggerEmail: "test@example.invalid"},
		{Name: "catalog-v1.2.3", Message: "", TaggerName: "Gitx Test", TaggerEmail: "test@example.invalid"},
		{Name: "catalog-v1.2.3", Message: "message", TaggerName: "", TaggerEmail: "test@example.invalid"},
		{Name: "catalog-v1.2.3", Message: "message", TaggerName: "Gitx Test", TaggerEmail: ""},
	}
	for _, spec := range cases {
		t.Run(spec.Name+"/"+spec.Message+"/"+spec.TaggerName+"/"+spec.TaggerEmail, func(t *testing.T) {
			recorder := &recordingRunner{base: ExecRunner{}}
			if _, err := New(NewSafeRunner(recorder)).CreateTag(context.Background(), repo, spec); err == nil {
				t.Fatal("invalid tag specification was accepted")
			}
			if len(recorder.requests) != 0 {
				t.Fatalf("invalid specification reached git: %#v", recorder.requests)
			}
		})
	}

	spec := TagSpec{Name: "catalog-v1.2.3", Message: "message", TaggerName: "Gitx Test", TaggerEmail: "test@example.invalid"}
	recorder := &recordingRunner{base: ExecRunner{}}
	client := New(NewSafeRunner(recorder))
	if _, err := client.CreateTag(context.Background(), repo, spec); err != nil {
		t.Fatal(err)
	}
	recorder.requests = nil
	if _, err := client.CreateTag(context.Background(), repo, spec); err == nil {
		t.Fatal("existing tag was moved or accepted")
	}
	for _, request := range recorder.requests {
		if slices.Contains(request.Argv, "tag") {
			t.Fatalf("existing-tag refusal attempted tag mutation: %#v", request)
		}
		if strings.Contains(strings.Join(request.Argv, " "), "--force") || slices.Contains(request.Argv, "commit") {
			t.Fatalf("unsafe existing-tag command: %#v", request)
		}
	}
}

func assertTagCommands(t *testing.T, requests []CommandRequest) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("tag operation emitted no commands")
	}
	for _, request := range requests {
		if request.Executable != "git" || !filepath.IsAbs(request.Cwd) {
			t.Fatalf("request = %#v, want argv-only git request", request)
		}
		joined := strings.Join(request.Argv, " ")
		if strings.Contains(joined, "--force") || strings.Contains(joined, "reset --hard") || strings.Contains(joined, "clean") || strings.Contains(joined, "stash") || slices.Contains(request.Argv, "commit") {
			t.Fatalf("unsafe tag command = %#v", request)
		}
	}
}

func newTagRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "--initial-branch=main")
	git(t, repo, "config", "user.name", "Gitx Test")
	git(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "catalog.yaml"), []byte("catalog: one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "catalog.yaml")
	git(t, repo, "commit", "-m", "initial")
	return repo, git(t, repo, "rev-parse", "HEAD")
}
