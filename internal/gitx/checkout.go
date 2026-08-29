package gitx

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type CheckoutSpec struct{ Path, Remote, Branch, DesiredCommit string }
type DirtyCounts struct {
	Entries, Modified, Added, Deleted, Renamed, Copied, Untracked, Unmerged, Ignored, Other int
}

func (c DirtyCounts) Total() int { return c.Entries }

type CheckoutResult struct {
	BeforeCommit, ResolvedCommit    string
	Adopted, Cloned, Dirty, Skipped bool
	DirtyCounts                     DirtyCounts
}
type Client struct{ runner Runner }

func New(runner Runner) *Client { return &Client{runner: runner} }

func (c *Client) EnsureCheckout(ctx context.Context, spec CheckoutSpec) (CheckoutResult, error) {
	var result CheckoutResult
	if c == nil || c.runner == nil {
		return result, errors.New("nil git runner")
	}
	if err := validateSpec(&spec); err != nil {
		return result, err
	}
	info, err := os.Lstat(spec.Path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return result, errors.New("checkout path is not a directory")
		}
		remote, err := c.run(ctx, spec.Path, "remote", "remote", "get-url", "origin")
		if err != nil {
			return result, err
		}
		if CanonicalRemote(string(remote.Stdout)) != CanonicalRemote(spec.Remote) {
			return result, errors.New("checkout origin does not match expected canonical URL")
		}
		result.Adopted = true
	} else if os.IsNotExist(err) {
		result.Cloned = true
		if _, err := c.run(ctx, filepath.Dir(spec.Path), "clone", "clone", "--branch", spec.Branch, spec.Remote, spec.Path); err != nil {
			return result, err
		}
	} else {
		return result, errors.New("inspect checkout path failed")
	}
	if _, err := c.run(ctx, spec.Path, "fetch", "fetch", "--no-tags", "origin", spec.Branch); err != nil {
		return result, err
	}
	head, err := c.run(ctx, spec.Path, "head", "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	result.ResolvedCommit = strings.TrimSpace(string(head.Stdout))
	if result.ResolvedCommit == "" {
		return result, errors.New("checkout HEAD was empty")
	}
	if result.Adopted {
		result.BeforeCommit = result.ResolvedCommit
	}
	status, err := c.run(ctx, spec.Path, "status", "status", "--porcelain=v1", "-z")
	if err != nil {
		return result, err
	}
	result.DirtyCounts = ParseStatusPorcelain(status.Stdout)
	result.Dirty = result.DirtyCounts.Total() != 0
	if _, err := c.run(ctx, spec.Path, "reachability", "merge-base", "--is-ancestor", spec.DesiredCommit, "origin/main"); err != nil {
		return result, errors.New("desired commit is not reachable from origin/main")
	}
	return result, nil
}
func (c *Client) AdvanceCheckout(ctx context.Context, spec CheckoutSpec) (CheckoutResult, error) {
	var result CheckoutResult
	if c == nil || c.runner == nil {
		return result, errors.New("nil git runner")
	}
	if err := validateAdvanceSpec(&spec); err != nil {
		return result, err
	}
	info, err := os.Lstat(spec.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return result, errors.New("checkout path does not exist")
		}
		return result, errors.New("inspect checkout path failed")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return result, errors.New("checkout path is not a directory")
	}
	remote, err := c.run(ctx, spec.Path, "remote", "remote", "get-url", "origin")
	if err != nil {
		return result, err
	}
	if CanonicalRemote(string(remote.Stdout)) != CanonicalRemote(spec.Remote) {
		return result, errors.New("checkout origin does not match expected canonical URL")
	}
	result.Adopted = true
	branch, err := c.run(ctx, spec.Path, "branch", "branch", "--show-current")
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(string(branch.Stdout)) != "main" {
		return result, errors.New("checkout must be on main")
	}
	if _, err := c.run(ctx, spec.Path, "fetch", "fetch", "--no-tags", "origin", spec.Branch); err != nil {
		return result, err
	}
	head, err := c.run(ctx, spec.Path, "head", "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	result.BeforeCommit = strings.TrimSpace(string(head.Stdout))
	if result.BeforeCommit == "" {
		return result, errors.New("checkout HEAD was empty")
	}
	result.ResolvedCommit = result.BeforeCommit
	status, err := c.run(ctx, spec.Path, "status", "status", "--porcelain=v1", "-z")
	if err != nil {
		return result, err
	}
	result.DirtyCounts = ParseStatusPorcelain(status.Stdout)
	result.Dirty = result.DirtyCounts.Total() != 0
	localPaths := parseStatusPaths(status.Stdout)
	if _, err := c.run(ctx, spec.Path, "reachability", "merge-base", "--is-ancestor", spec.DesiredCommit, "origin/main"); err != nil {
		return result, errors.New("desired commit is not reachable from origin/main")
	}
	if result.BeforeCommit == spec.DesiredCommit {
		return result, nil
	}
	ancestry, err := c.run(ctx, spec.Path, "divergence", "merge-base", "--is-ancestor", "HEAD", spec.DesiredCommit)
	if err != nil {
		if ancestry.ExitCode == 1 {
			return result, errors.New("checkout main has diverged from desired commit")
		}
		return result, err
	}
	changed, err := c.run(ctx, spec.Path, "changed-paths", "diff", "--name-only", "-z", "HEAD", spec.DesiredCommit)
	if err != nil {
		return result, err
	}
	if pathsOverlap(localPaths, parseNULPaths(changed.Stdout)) {
		result.Skipped = true
		return result, errors.New("checkout advancement skipped because local changes overlap the requested commit; save those changes before retrying")
	}
	if _, err := c.run(ctx, spec.Path, "checkout", "checkout", "main"); err != nil {
		return result, err
	}
	if _, err := c.run(ctx, spec.Path, "merge", "merge", "--ff-only", spec.DesiredCommit); err != nil {
		return result, err
	}
	after, err := c.run(ctx, spec.Path, "head", "rev-parse", "HEAD")
	if err != nil {
		return result, err
	}
	result.ResolvedCommit = strings.TrimSpace(string(after.Stdout))
	if result.ResolvedCommit == "" {
		return result, errors.New("checkout HEAD was empty after advancement")
	}
	afterStatus, err := c.run(ctx, spec.Path, "status", "status", "--porcelain=v1", "-z")
	if err != nil {
		return result, err
	}
	result.DirtyCounts = ParseStatusPorcelain(afterStatus.Stdout)
	result.Dirty = result.DirtyCounts.Total() != 0
	return result, nil
}

func validateAdvanceSpec(spec *CheckoutSpec) error {
	if err := validateSpec(spec); err != nil {
		return err
	}
	if !isCommitID(spec.DesiredCommit) {
		return errors.New("desired commit must be a 40-character hexadecimal commit")
	}
	return nil
}

func parseStatusPaths(data []byte) map[string]struct{} {
	paths := make(map[string]struct{})
	for len(data) != 0 {
		record, rest := nextNULRecord(data)
		data = rest
		if len(record) < 3 || record[2] != ' ' {
			continue
		}
		if path := string(record[3:]); path != "" {
			paths[path] = struct{}{}
		}
		if record[0] != 'R' && record[1] != 'R' && record[0] != 'C' && record[1] != 'C' {
			continue
		}
		if len(data) == 0 {
			continue
		}
		oldPath, rest := nextNULRecord(data)
		data = rest
		if len(oldPath) != 0 {
			paths[string(oldPath)] = struct{}{}
		}
	}
	return paths
}

func parseNULPaths(data []byte) map[string]struct{} {
	paths := make(map[string]struct{})
	for len(data) != 0 {
		record, rest := nextNULRecord(data)
		data = rest
		if len(record) != 0 {
			paths[string(record)] = struct{}{}
		}
	}
	return paths
}

func nextNULRecord(data []byte) ([]byte, []byte) {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		return data[:end], data[end+1:]
	}
	return data, nil
}

func pathsOverlap(left, right map[string]struct{}) bool {
	for leftPath := range left {
		for rightPath := range right {
			if gitPathsOverlap(leftPath, rightPath) {
				return true
			}
		}
	}
	return false
}

func gitPathsOverlap(left, right string) bool {
	left = strings.TrimSuffix(left, "/")
	right = strings.TrimSuffix(right, "/")
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func validateSpec(spec *CheckoutSpec) error {
	if spec.Path == "" || !filepath.IsAbs(spec.Path) {
		return errors.New("checkout path must be absolute")
	}
	if spec.Remote == "" || CanonicalRemote(spec.Remote) == "" {
		return errors.New("checkout remote is required")
	}
	if spec.Branch == "" {
		spec.Branch = "main"
	}
	if spec.Branch != "main" {
		return errors.New("checkout branch must be main")
	}
	if spec.DesiredCommit == "" {
		return errors.New("desired commit is required")
	}
	spec.Path = filepath.Clean(spec.Path)
	return nil
}
func (c *Client) run(ctx context.Context, cwd, operation string, argv ...string) (CommandResult, error) {
	request := CommandRequest{Operation: operation, Executable: "git", Argv: append([]string(nil), argv...), Cwd: cwd}
	result, err := c.runner.Run(ctx, request)
	if err != nil || result.ExitCode != 0 {
		return result, errors.New("git command failed")
	}
	return result, nil
}

func ParseStatusPorcelain(data []byte) DirtyCounts {
	var counts DirtyCounts
	renamePath := false
	for len(data) != 0 {
		end := bytes.IndexByte(data, 0)
		record := data
		if end < 0 {
			data = nil
		} else {
			record, data = data[:end], data[end+1:]
		}
		if renamePath {
			renamePath = false
			continue
		}
		if len(record) < 3 || record[2] != ' ' {
			continue
		}
		x, y := record[0], record[1]
		counts.Entries++
		switch {
		case x == '?' && y == '?':
			counts.Untracked++
		case x == '!' && y == '!':
			counts.Ignored++
		case x == 'U' || y == 'U' || (x == y && (x == 'A' || x == 'D')):
			counts.Unmerged++
		case x == 'R' || y == 'R':
			counts.Renamed++
			renamePath = true
		case x == 'C' || y == 'C':
			counts.Copied++
			renamePath = true
		case x == 'A' || y == 'A':
			counts.Added++
		case x == 'D' || y == 'D':
			counts.Deleted++
		case x == 'M' || y == 'M' || x == 'T' || y == 'T':
			counts.Modified++
		default:
			counts.Other++
		}
	}
	return counts
}

func CanonicalRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ':'); i > 0 && !strings.Contains(raw[:i], "/") && !strings.Contains(raw, "://") {
		raw = "ssh://" + raw[:i] + "/" + strings.TrimPrefix(path.Clean("/"+raw[i+1:]), "/")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return filepath.Clean(raw)
	}
	u.Scheme, u.Host, u.Path = strings.ToLower(u.Scheme), strings.ToLower(u.Host), path.Clean(u.Path)
	if u.Scheme == "file" {
		if p, err := url.PathUnescape(u.Path); err == nil {
			if u.Host != "" && u.Host != "localhost" {
				p = "//" + u.Host + p
			}
			if absolute, err := filepath.Abs(filepath.FromSlash(p)); err == nil {
				return filepath.Clean(absolute)
			}
		}
	}
	return strings.TrimSuffix(u.String(), "/")
}
