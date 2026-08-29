package gitx

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

var ErrUnsafeGitCommand = errors.New("unsafe git command")

type SafeRunner struct{ Runner Runner }

func NewSafeRunner(runner Runner) *SafeRunner { return &SafeRunner{Runner: runner} }

func (r *SafeRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if r == nil || r.Runner == nil {
		return CommandResult{ExitCode: -1}, errors.New("nil git runner")
	}
	if err := ValidateGitCommand(request); err != nil {
		return CommandResult{ExitCode: -1}, err
	}
	return r.Runner.Run(ctx, request)
}
func ValidateGitCommand(request CommandRequest) error {
	if !isGitExecutable(request.Executable) || request.Cwd == "" || !filepath.IsAbs(request.Cwd) {
		return ErrUnsafeGitCommand
	}
	if len(request.Argv) == 0 {
		return ErrUnsafeGitCommand
	}
	for _, arg := range request.Argv {
		if strings.IndexByte(arg, 0) >= 0 || isForceFlag(arg) || isShellOption(arg) || strings.Contains(arg, "ext::") || strings.ContainsAny(arg, ";|&`$()<>\r\n") {
			return ErrUnsafeGitCommand
		}
	}

	subcommand, index := gitSubcommand(request.Argv)
	switch subcommand {
	case "clean", "stash", "commit", "commit-tree", "reset", "shell", "difftool", "mergetool":
		return ErrUnsafeGitCommand
	}
	if subcommand == "fetch" || subcommand == "push" {
		if destructiveTagRefspec(request.Argv[index+1:]) {
			return ErrUnsafeGitCommand
		}
	}
	if subcommand == "checkout" && activeTagCheckout(request.Argv[index+1:]) {
		return ErrUnsafeGitCommand
	}
	return nil
}
func isGitExecutable(executable string) bool {
	return executable == "git" || (filepath.IsAbs(executable) && filepath.Base(executable) == "git")
}
func isForceFlag(arg string) bool {
	if strings.HasPrefix(arg, "--force") {
		return true
	}
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	return strings.ContainsRune(arg[1:], 'f')
}
func isShellOption(arg string) bool {
	if arg == "-c" || arg == "--command" || strings.HasPrefix(arg, "--command=") || arg == "--config" || strings.HasPrefix(arg, "--config=") || arg == "--config-env" || strings.HasPrefix(arg, "--config-env=") {
		return true
	}
	return arg == "--upload-pack" || strings.HasPrefix(arg, "--upload-pack=") || arg == "--receive-pack" || strings.HasPrefix(arg, "--receive-pack=") || arg == "-C" || strings.HasPrefix(arg, "-C") || arg == "--git-dir" || strings.HasPrefix(arg, "--git-dir=") || arg == "--work-tree" || strings.HasPrefix(arg, "--work-tree=")
}
func gitSubcommand(argv []string) (string, int) {
	for i, arg := range argv {
		if arg == "--" && i+1 < len(argv) {
			return argv[i+1], i+1
		}
		if !strings.HasPrefix(arg, "-") {
			return arg, i
		}
	}
	return "", len(argv)
}
func destructiveTagRefspec(args []string) bool {
	for _, arg := range args {
		if arg == "--prune-tags" || arg == "--delete" {
			return true
		}
		if arg == "--" || strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, ":") {
			return true
		}
		source, destination, hasColon := strings.Cut(arg, ":")
		if !hasColon {
			if strings.HasPrefix(arg, "refs/tags/") {
				return true
			}
			continue
		}
		if strings.HasPrefix(destination, "refs/tags/") || (destination == "" && strings.HasPrefix(source, "refs/tags/")) {
			return true
		}
	}
	return false
}
func activeTagCheckout(args []string) bool {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return true
	}
	ref := args[0]
	if strings.HasPrefix(ref, "refs/tags/") || strings.HasPrefix(ref, "catalog-v") {
		return true
	}
	return ref != "main" && !isCommitID(ref)
}
func isCommitID(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

type ObjectReader struct{ runner Runner }

func NewObjectReader(runner Runner) *ObjectReader {
	return &ObjectReader{runner: NewSafeRunner(runner)}
}
func (r *ObjectReader) Show(ctx context.Context, cwd, revision, objectPath string) ([]byte, error) {
	object, err := objectPathName(revision, objectPath)
	if err != nil {
		return nil, err
	}
	return r.read(ctx, cwd, "show", "show", object)
}
func (r *ObjectReader) CatFile(ctx context.Context, cwd, object string) ([]byte, error) {
	if err := validateObjectName(object); err != nil {
		return nil, err
	}
	return r.read(ctx, cwd, "cat-file", "cat-file", "-p", object)
}
func (r *ObjectReader) ReadObject(ctx context.Context, cwd string, parts ...string) ([]byte, error) {
	switch len(parts) {
	case 1:
		return r.CatFile(ctx, cwd, parts[0])
	case 2:
		return r.Show(ctx, cwd, parts[0], parts[1])
	default:
		return nil, errors.New("object read requires an object or revision and path")
	}
}
func (r *ObjectReader) read(ctx context.Context, cwd, operation string, argv ...string) ([]byte, error) {
	if r == nil || r.runner == nil {
		return nil, errors.New("nil git runner")
	}
	if cwd == "" || !filepath.IsAbs(cwd) {
		return nil, errors.New("object read directory must be absolute")
	}
	request := CommandRequest{Operation: operation, Executable: "git", Argv: append([]string(nil), argv...), Cwd: cwd}
	if err := ValidateGitCommand(request); err != nil {
		return nil, err
	}
	result, err := r.runner.Run(ctx, request)
	if err != nil {
		return nil, err
	}
	if result.ExitCode != 0 {
		return nil, errors.New("git object read failed")
	}
	return append([]byte(nil), result.Stdout...), nil
}
func objectPathName(revision, objectPath string) (string, error) {
	if err := validateRevision(revision); err != nil {
		return "", err
	}
	if err := validateObjectPath(objectPath); err != nil {
		return "", err
	}
	return revision + ":" + objectPath, nil
}
func validateRevision(revision string) error {
	if revision == "" || strings.TrimSpace(revision) != revision || strings.HasPrefix(revision, "-") || strings.IndexByte(revision, 0) >= 0 {
		return ErrUnsafeGitCommand
	}
	if strings.ContainsAny(revision, ";|&`$()<>\r\n") {
		return ErrUnsafeGitCommand
	}
	return nil
}
func validateObjectPath(objectPath string) error {
	if objectPath == "" || strings.IndexByte(objectPath, 0) >= 0 || filepath.IsAbs(objectPath) || strings.HasPrefix(objectPath, "-") {
		return ErrUnsafeGitCommand
	}
	for _, part := range strings.FieldsFunc(objectPath, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return ErrUnsafeGitCommand
		}
	}
	return nil
}
func validateObjectName(object string) error {
	if object == "" || strings.IndexByte(object, 0) >= 0 || strings.HasPrefix(object, "-") || strings.TrimSpace(object) != object {
		return ErrUnsafeGitCommand
	}
	if strings.ContainsAny(object, ";|&`$()<>\r\n") {
		return ErrUnsafeGitCommand
	}
	if i := strings.IndexByte(object, ':'); i >= 0 {
		if _, err := objectPathName(object[:i], object[i+1:]); err != nil {
			return err
		}
	}
	return nil
}
func (c *Client) objectReader() *ObjectReader {
	if c == nil {
		return NewObjectReader(nil)
	}
	return NewObjectReader(c.runner)
}
func (c *Client) Show(ctx context.Context, cwd, revision, objectPath string) ([]byte, error) {
	return c.objectReader().Show(ctx, cwd, revision, objectPath)
}
func (c *Client) CatFile(ctx context.Context, cwd, object string) ([]byte, error) {
	return c.objectReader().CatFile(ctx, cwd, object)
}
func (c *Client) ReadObject(ctx context.Context, cwd string, parts ...string) ([]byte, error) {
	return c.objectReader().ReadObject(ctx, cwd, parts...)
}
