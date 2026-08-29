package gitx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// UpdateSpec identifies an existing main checkout whose origin/main should be
// observed for a possible catalog-authoring update.
type UpdateSpec struct {
	Path   string
	Remote string
	Branch string
}

// UpdateResult is the read-only comparison between the checkout HEAD and the
// fetched origin/main commit.
type UpdateResult struct {
	CurrentCommit   string
	CandidateCommit string
	Newer           bool
}

// ObserveUpdate fetches origin/main and reports a descendant candidate without
// changing the checkout branch, index, or worktree.
func (c *Client) ObserveUpdate(ctx context.Context, spec UpdateSpec) (UpdateResult, error) {
	var result UpdateResult
	if c == nil || c.runner == nil {
		return result, errors.New("nil git runner")
	}
	if err := validateUpdateSpec(&spec); err != nil {
		return result, err
	}
	info, err := os.Lstat(spec.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, errors.New("update checkout path does not exist")
		}
		return result, errors.New("inspect update checkout path failed")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return result, errors.New("update checkout path is not a directory")
	}

	remote, err := c.run(ctx, spec.Path, "remote", "remote", "get-url", "origin")
	if err != nil {
		return result, err
	}
	if CanonicalRemote(string(remote.Stdout)) != CanonicalRemote(spec.Remote) {
		return result, errors.New("checkout origin does not match expected canonical URL")
	}
	if _, err := c.run(ctx, spec.Path, "fetch", "fetch", "--no-tags", "origin", spec.Branch); err != nil {
		return result, err
	}

	result.CurrentCommit, err = c.resolveUpdateCommit(ctx, spec.Path, "head", "HEAD", "current HEAD")
	if err != nil {
		return result, err
	}
	result.CandidateCommit, err = c.resolveUpdateCommit(ctx, spec.Path, "candidate", "origin/"+spec.Branch, "origin/main")
	if err != nil {
		return result, err
	}
	if result.CurrentCommit == result.CandidateCommit {
		return result, nil
	}

	ancestry, err := c.run(ctx, spec.Path, "reachability", "merge-base", "--is-ancestor", result.CurrentCommit, result.CandidateCommit)
	if err == nil {
		result.Newer = true
		return result, nil
	}
	if ancestry.ExitCode != 1 {
		return result, err
	}

	reverse, err := c.run(ctx, spec.Path, "divergence", "merge-base", "--is-ancestor", result.CandidateCommit, result.CurrentCommit)
	if err == nil {
		return result, nil
	}
	if reverse.ExitCode == 1 {
		return result, errors.New("checkout HEAD and origin/main have diverged")
	}
	return result, err
}

func validateUpdateSpec(spec *UpdateSpec) error {
	if spec.Path == "" || !filepath.IsAbs(spec.Path) {
		return errors.New("update checkout path must be absolute")
	}
	if spec.Remote == "" || CanonicalRemote(spec.Remote) == "" {
		return errors.New("update checkout remote is required")
	}
	if spec.Branch == "" {
		spec.Branch = "main"
	}
	if spec.Branch != "main" {
		return errors.New("update checkout branch must be main")
	}
	spec.Path = filepath.Clean(spec.Path)
	return nil
}

func (c *Client) resolveUpdateCommit(ctx context.Context, cwd, operation, revision, label string) (string, error) {
	resolved, err := c.run(ctx, cwd, operation, "rev-parse", revision)
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(resolved.Stdout))
	if !isCommitID(commit) {
		return "", errors.New(label + " was not a 40-character hexadecimal commit")
	}
	return commit, nil
}
