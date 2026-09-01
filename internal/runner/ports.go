package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
	UserScope         = ScopeUser
	SystemScope       = ScopeSystem
)

type NetworkPolicy string

const (
	NetworkNone     NetworkPolicy = "none"
	NetworkRequired NetworkPolicy = "required"
)

type OutputPolicy string

const (
	OutputCaptureRedacted OutputPolicy = "capture-redacted"
	OutputDiscard         OutputPolicy = "discard"
	OutputStreamSafe       OutputPolicy = "stream-safe"
)

// CommandRequest is the complete, argv-only command boundary. Env and Stdin
// are bounded by ValidateCommandRequest; nil means no values are supplied.
type CommandRequest struct {
	Operation    string
	Executable   string
	Argv         []string
	Cwd          string
	Env          map[string]string
	Stdin        []byte
	Scope        Scope
	Network      NetworkPolicy
	OutputPolicy OutputPolicy
	Timeout      time.Duration
	OutputLimit  int64
	Shell        bool
}

type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

type Runner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

var (
	ErrInvalidCommandRequest = errors.New("invalid command request")
	ErrUnmatchedExpectation  = errors.New("unmatched runner expectation")
	ErrUnconsumedExpectations = errors.New("unconsumed runner expectations")
)

const (
	MaxEnvEntries = 32
	MaxEnvBytes   = 8 << 10
	MaxStdinBytes = 1 << 20
	MaxOutputLimit int64 = 16 << 20
	MaxTimeout = 10 * time.Minute
)

func ValidateCommandRequest(request CommandRequest) error {
	if request.Operation == "" {
		return invalidRequest("operation", "is empty")
	}
	if err := validateText("operation", request.Operation); err != nil { return err }
	if request.Executable == "" {
		return invalidRequest("executable", "is empty")
	}
	if strings.Contains(request.Executable, "/") && !filepath.IsAbs(request.Executable) {
		return invalidRequest("executable", "must be absolute when it contains a path")
	}
	if isShellExecutable(request.Executable) || isSudo(request.Executable) {
		return invalidRequest("executable", "invokes a forbidden shell or sudo")
	}
	if err := validateText("executable", request.Executable); err != nil { return err }
	if request.Cwd == "" || !filepath.IsAbs(request.Cwd) {
		return invalidRequest("cwd", "must be an absolute path")
	}
	if err := validateText("cwd", request.Cwd); err != nil { return err }
	if request.Shell {
		return invalidRequest("shell", "shell execution is forbidden")
	}
	for i, arg := range request.Argv {
		if isSudo(arg) || forbiddenShellForm(request.Executable, arg) {
			return invalidRequest(fmt.Sprintf("argv[%d]", i), "contains a forbidden shell or sudo form")
		}
		if err := validateText(fmt.Sprintf("argv[%d]", i), arg); err != nil { return err }
	}
	if bytes.IndexByte(request.Stdin, 0) >= 0 || len(request.Stdin) > MaxStdinBytes {
		return invalidRequest("stdin", "contains NUL or exceeds its bound")
	}
	if request.Scope != ScopeUser && request.Scope != ScopeSystem {
		return invalidRequest("scope", "must be user or system")
	}
	if request.Network != NetworkNone && request.Network != NetworkRequired {
		return invalidRequest("network", "must be none or required")
	}
	if request.OutputPolicy != OutputCaptureRedacted && request.OutputPolicy != OutputDiscard && request.OutputPolicy != OutputStreamSafe {
		return invalidRequest("outputPolicy", "is not supported")
	}
	if request.Timeout <= 0 || request.Timeout > MaxTimeout {
		return invalidRequest("timeout", "must be bounded and positive")
	}
	if request.OutputLimit <= 0 || request.OutputLimit > MaxOutputLimit {
		return invalidRequest("outputLimit", "must be bounded and positive")
	}
	if len(request.Env) > MaxEnvEntries {
		return invalidRequest("env", "has too many entries")
	}
	keys := make([]string, 0, len(request.Env))
	for key := range request.Env { keys = append(keys, key) }
	sort.Strings(keys)
	envBytes := 0
	for _, key := range keys {
		value := request.Env[key]
		if key == "" || strings.ContainsAny(key, "=\x00\r\n") || strings.ContainsAny(value, "\x00\r\n") {
			return invalidRequest("env", "contains an invalid name or value")
		}
		if secretLikeEnvironment(key, value) {
			return invalidRequest("env."+key, "contains a secret-like value")
		}
		envBytes += len(key) + len(value) + 2
		if envBytes > MaxEnvBytes { return invalidRequest("env", "exceeds its byte bound") }
	}
	return nil
}

func ValidateRequest(request CommandRequest) error { return ValidateCommandRequest(request) }

func invalidRequest(field, reason string) error {
	return fmt.Errorf("%w: %s %s", ErrInvalidCommandRequest, field, reason)
}

func validateText(field, value string) error {
	if strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, ";|&`$()<>\r\n") {
		return invalidRequest(field, "contains NUL or shell syntax")
	}
	return nil
}

func isShellExecutable(value string) bool {
	switch strings.ToLower(filepath.Base(value)) {
	case "sh", "bash", "dash", "zsh", "fish", "ksh", "csh", "tcsh", "ash", "rbash":
		return true
	default:
		return false
	}
}

func isShellForm(value string) bool {
	if isShellExecutable(value) || isSudo(value) { return true }
	switch value {
	case "-c", "-lc", "-cl", "-ic", "-ci", "-xc", "-cx", "-ec", "-ce", "-lec", "-lce":
		return true
	}
	return strings.HasPrefix(value, "--command=") || strings.HasPrefix(value, "-c") && len(value) > 2
}

func forbiddenShellForm(executable, arg string) bool {
	if filepath.Base(executable) == "pacman" && isShellExecutable(arg) {
		return false
	}
	return isShellForm(arg)
}

func isSudo(value string) bool { return filepath.Base(value) == "sudo" }

func secretLikeEnvironment(name, value string) bool {
	normalizedName := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(name))
	for _, marker := range []string{"password", "passwd", "secret", "token", "api_key", "auth", "credential", "cookie", "private_key"} {
		if strings.Contains(normalizedName, marker) { return true }
	}
	lowerValue := strings.ToLower(value)
	for _, marker := range []string{"secret", "password", "passwd", "api_key", "access_token", "auth_token", "bearer ", "private key", "credential", "-----begin"} {
		if strings.Contains(lowerValue, marker) { return true }
	}
	return false
}
