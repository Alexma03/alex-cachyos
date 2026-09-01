package gitx

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
)

var ErrCheckpointPathNotCommitted = errors.New("checkpoint path is not committed at HEAD")

// ValidateCommittedPaths proves that every caller-declared path exists in HEAD
// and that neither the index nor working tree differs from HEAD for those
// paths. Unrelated dirty files are deliberately outside the pathspec.
func (c *Client) ValidateCommittedPaths(ctx context.Context, cwd string, paths []string) error {
	if c == nil || c.runner == nil || !filepath.IsAbs(cwd) || len(paths) == 0 {
		return ErrCheckpointPathNotCommitted
	}
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	runner := NewSafeRunner(c.runner)
	run := func(operation string, argv ...string) error {
		result, err := runner.Run(ctx, CommandRequest{Operation: operation, Executable: "git", Argv: argv, Cwd: cwd})
		if err != nil || result.ExitCode != 0 {
			return ErrCheckpointPathNotCommitted
		}
		return nil
	}
	for _, path := range paths {
		if err := validateObjectPath(path); err != nil {
			return fmt.Errorf("%w: invalid declared path", ErrCheckpointPathNotCommitted)
		}
		if err := run("checkpoint-head-path", "cat-file", "-e", "HEAD:"+path); err != nil {
			return err
		}
	}
	argv := append([]string{"diff", "--quiet", "HEAD", "--"}, paths...)
	if err := run("checkpoint-worktree", argv...); err != nil {
		return err
	}
	argv = append([]string{"diff", "--cached", "--quiet", "HEAD", "--"}, paths...)
	return run("checkpoint-index", argv...)
}
