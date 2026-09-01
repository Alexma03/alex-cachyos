package gitx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCommittedPathsIgnoresUnrelatedDirtyFilesAndRejectsDeclaredDrift(t *testing.T) {
	repo, _ := newTagRepo(t)
	assets := filepath.Join(repo, "managed-assets.json")
	if err := os.WriteFile(assets, []byte("assets\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "managed-assets.json")
	git(t, repo, "commit", "-m", "assets")
	if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := New(ExecRunner{})
	if err := client.ValidateCommittedPaths(context.Background(), repo, []string{"managed-assets.json", "catalog.yaml"}); err != nil {
		t.Fatalf("unrelated dirty file blocked checkpoint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "catalog.yaml"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := client.ValidateCommittedPaths(context.Background(), repo, []string{"catalog.yaml"}); !errors.Is(err, ErrCheckpointPathNotCommitted) {
		t.Fatalf("declared drift error = %v", err)
	}
	if err := client.ValidateCommittedPaths(context.Background(), repo, []string{"untracked.yaml"}); !errors.Is(err, ErrCheckpointPathNotCommitted) {
		t.Fatalf("untracked path error = %v", err)
	}
}
