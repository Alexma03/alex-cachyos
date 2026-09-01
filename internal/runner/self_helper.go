package runner

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	SelfHelperSchema           = "alex-cachyos.privileged-helper/v1"
	SelfHelperPublishFiles     = "publish-files"
	SelfHelperArgument         = "--alex-cachyos-privileged-helper"
	SelfHelperCommandOperation = "self-helper.publish-files"
	MaxSelfHelperRequestBytes  = 64 << 10
	MaxSelfHelperFileBytes     = 32 << 20
	maxSelfHelperFiles         = 64
)

var ErrInvalidSelfHelperRequest = errors.New("invalid privileged helper request")

// SelfHelperFile is a staged, content-addressed publication. Destination and
// mode are accepted only when they exactly match the root-side policy.
type SelfHelperFile struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	SHA256      string `json:"sha256"`
	Mode        uint32 `json:"mode"`
}

// SelfHelperRequest is the only payload accepted by the hidden helper mode.
// It deliberately has no executable, argv, environment, or shell fields.
type SelfHelperRequest struct {
	Schema    string           `json:"schema"`
	Operation string           `json:"operation"`
	Files     []SelfHelperFile `json:"files"`
}

// SelfHelperPolicy is supplied by trusted program code, never by request JSON.
// Every destination and mode must match an entry exactly.
type SelfHelperPolicy struct {
	SourceUID    uint32
	Destinations map[string]os.FileMode
}

// StageSelfHelperFile writes bytes into a private regular file owned by the
// invoking user and returns the identity the root helper will re-validate.
func StageSelfHelperFile(stageDir string, content []byte, destination string, mode os.FileMode) (SelfHelperFile, error) {
	if !filepath.IsAbs(stageDir) || filepath.Clean(stageDir) != stageDir {
		return SelfHelperFile{}, invalidSelfHelper("stage directory is not canonical and absolute")
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return SelfHelperFile{}, invalidSelfHelper("destination is not canonical and absolute")
	}
	if len(content) > MaxSelfHelperFileBytes {
		return SelfHelperFile{}, invalidSelfHelper("staged content exceeds its bound")
	}
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return SelfHelperFile{}, fmt.Errorf("create helper stage: %w", err)
	}
	info, err := os.Lstat(stageDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return SelfHelperFile{}, invalidSelfHelper("stage directory is not a private directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return SelfHelperFile{}, invalidSelfHelper("stage directory owner does not match the invoking user")
	}
	file, err := os.CreateTemp(stageDir, "publish-*")
	if err != nil {
		return SelfHelperFile{}, fmt.Errorf("create helper stage file: %w", err)
	}
	name := file.Name()
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(name)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return SelfHelperFile{}, fmt.Errorf("secure helper stage file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return SelfHelperFile{}, fmt.Errorf("write helper stage file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return SelfHelperFile{}, fmt.Errorf("sync helper stage file: %w", err)
	}
	if err := file.Close(); err != nil {
		return SelfHelperFile{}, fmt.Errorf("close helper stage file: %w", err)
	}
	remove = false
	hash := sha256.Sum256(content)
	return SelfHelperFile{Source: name, Destination: destination, SHA256: hex.EncodeToString(hash[:]), Mode: uint32(mode.Perm())}, nil
}

// BuildSelfHelperCommand creates the sole system-scoped self invocation. The
// elevation decorator turns it into pkexec <same-binary> <fixed-helper-flag>.
func BuildSelfHelperCommand(binary string, request SelfHelperRequest) (CommandRequest, error) {
	if !filepath.IsAbs(binary) || filepath.Clean(binary) != binary {
		return CommandRequest{}, invalidSelfHelper("helper executable is not canonical and absolute")
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > MaxSelfHelperRequestBytes {
		return CommandRequest{}, invalidSelfHelper("helper request cannot be encoded within its bound")
	}
	command := CommandRequest{
		Operation: SelfHelperCommandOperation, Executable: binary,
		Argv: []string{SelfHelperArgument}, Cwd: "/", Stdin: payload,
		Scope: ScopeSystem, Network: NetworkNone, OutputPolicy: OutputDiscard,
		Timeout: time.Minute, OutputLimit: 4096,
	}
	if err := ValidateCommandRequest(command); err != nil {
		return CommandRequest{}, err
	}
	return command, nil
}

func DecodeSelfHelperRequest(input io.Reader) (SelfHelperRequest, error) {
	if input == nil {
		return SelfHelperRequest{}, invalidSelfHelper("request input is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxSelfHelperRequestBytes+1))
	if err != nil || len(data) > MaxSelfHelperRequestBytes {
		return SelfHelperRequest{}, invalidSelfHelper("request input exceeds its bound")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var request SelfHelperRequest
	if err := decoder.Decode(&request); err != nil {
		return SelfHelperRequest{}, invalidSelfHelper("request is not valid typed JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SelfHelperRequest{}, invalidSelfHelper("request contains trailing data")
	}
	return request, nil
}

func ExecuteSelfHelper(request SelfHelperRequest, policy SelfHelperPolicy) error {
	return executeSelfHelper(request, policy, selfHelperHooks{})
}

func ExecuteSelfHelperInput(input io.Reader, policy SelfHelperPolicy) error {
	request, err := DecodeSelfHelperRequest(input)
	if err != nil {
		return err
	}
	return ExecuteSelfHelper(request, policy)
}

type selfHelperHooks struct{ afterSourcesOpened func() }

type openedSelfHelperFile struct {
	descriptor SelfHelperFile
	source     *os.File
	parent     *os.File
	temporary  string
}

func executeSelfHelper(request SelfHelperRequest, policy SelfHelperPolicy, hooks selfHelperHooks) (err error) {
	if err := validateSelfHelperRequest(request, policy); err != nil {
		return err
	}
	opened := make([]openedSelfHelperFile, 0, len(request.Files))
	defer func() {
		for i := range opened {
			if opened[i].temporary != "" && opened[i].parent != nil {
				_ = syscall.Unlinkat(int(opened[i].parent.Fd()), opened[i].temporary)
			}
			if opened[i].source != nil {
				_ = opened[i].source.Close()
			}
			if opened[i].parent != nil {
				_ = opened[i].parent.Close()
			}
		}
	}()

	for _, item := range request.Files {
		source, openErr := openStagedSource(item.Source, policy.SourceUID)
		if openErr != nil {
			return openErr
		}
		opened = append(opened, openedSelfHelperFile{descriptor: item, source: source})
	}
	if hooks.afterSourcesOpened != nil {
		hooks.afterSourcesOpened()
	}

	// Prepare and verify every candidate before the first destination rename.
	for i := range opened {
		parentPath := filepath.Dir(opened[i].descriptor.Destination)
		parentFD, openErr := syscall.Open(parentPath, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
		if openErr != nil {
			return invalidSelfHelper("destination parent is unavailable or unsafe")
		}
		opened[i].parent = os.NewFile(uintptr(parentFD), parentPath)
		name, tempFile, createErr := createTemporaryAt(parentFD, opened[i].descriptor.Mode)
		if createErr != nil {
			return createErr
		}
		opened[i].temporary = name
		copyErr := copyAndVerify(opened[i].source, tempFile, opened[i].descriptor.SHA256)
		closeErr := tempFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return fmt.Errorf("close helper candidate: %w", closeErr)
		}
	}

	for i := range opened {
		item := &opened[i]
		base := filepath.Base(item.descriptor.Destination)
		if err := revalidateDestination(item.descriptor, policy, item.parent); err != nil {
			return err
		}
		if err := syscall.Renameat(int(item.parent.Fd()), item.temporary, int(item.parent.Fd()), base); err != nil {
			return fmt.Errorf("atomically publish allowlisted destination: %w", err)
		}
		item.temporary = ""
		if err := item.parent.Sync(); err != nil {
			return fmt.Errorf("sync destination directory: %w", err)
		}
	}
	return nil
}

func validateSelfHelperRequest(request SelfHelperRequest, policy SelfHelperPolicy) error {
	if request.Schema != SelfHelperSchema || request.Operation != SelfHelperPublishFiles {
		return invalidSelfHelper("schema or operation is not allowlisted")
	}
	if len(request.Files) == 0 || len(request.Files) > maxSelfHelperFiles || len(policy.Destinations) == 0 {
		return invalidSelfHelper("file batch is empty, oversized, or has no trusted policy")
	}
	seenSources, seenDestinations := map[string]bool{}, map[string]bool{}
	for _, item := range request.Files {
		if !filepath.IsAbs(item.Source) || filepath.Clean(item.Source) != item.Source ||
			!filepath.IsAbs(item.Destination) || filepath.Clean(item.Destination) != item.Destination {
			return invalidSelfHelper("source or destination is not canonical and absolute")
		}
		if seenSources[item.Source] || seenDestinations[item.Destination] {
			return invalidSelfHelper("file batch contains duplicate paths")
		}
		seenSources[item.Source], seenDestinations[item.Destination] = true, true
		mode, ok := policy.Destinations[item.Destination]
		if !ok || uint32(mode.Perm()) != item.Mode || item.Mode == 0 || item.Mode&^uint32(0o777) != 0 {
			return invalidSelfHelper("destination or mode is not allowlisted")
		}
		decoded, err := hex.DecodeString(item.SHA256)
		if err != nil || len(decoded) != sha256.Size || item.SHA256 != strings.ToLower(item.SHA256) {
			return invalidSelfHelper("source hash is invalid")
		}
	}
	return nil
}

func openStagedSource(path string, uid uint32) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, invalidSelfHelper("staged source is unavailable or unsafe")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		_ = file.Close()
		return nil, invalidSelfHelper("staged source is not a private regular file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uid {
		_ = file.Close()
		return nil, invalidSelfHelper("staged source owner does not match policy")
	}
	return file, nil
}

func createTemporaryAt(parentFD int, mode uint32) (string, *os.File, error) {
	for attempt := 0; attempt < 16; attempt++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, fmt.Errorf("create helper candidate name: %w", err)
		}
		name := ".alex-cachyos-" + hex.EncodeToString(random[:])
		fd, err := syscall.Openat(parentFD, name, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, mode)
		if errors.Is(err, syscall.EEXIST) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create helper candidate: %w", err)
		}
		file := os.NewFile(uintptr(fd), name)
		if err := file.Chmod(os.FileMode(mode)); err != nil {
			_ = file.Close()
			_ = syscall.Unlinkat(parentFD, name)
			return "", nil, err
		}
		return name, file, nil
	}
	return "", nil, errors.New("cannot allocate helper candidate")
}

func copyAndVerify(source, destination *os.File, want string) error {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return invalidSelfHelper("staged source cannot be rewound")
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(source, MaxSelfHelperFileBytes+1))
	if err != nil {
		return fmt.Errorf("copy staged source: %w", err)
	}
	if written > MaxSelfHelperFileBytes || hex.EncodeToString(hash.Sum(nil)) != want {
		return invalidSelfHelper("staged source changed or exceeded its bound")
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("sync helper candidate: %w", err)
	}
	return nil
}

func revalidateDestination(item SelfHelperFile, policy SelfHelperPolicy, parent *os.File) error {
	mode, ok := policy.Destinations[item.Destination]
	if !ok || uint32(mode.Perm()) != item.Mode || filepath.Base(item.Destination) == "." {
		return invalidSelfHelper("destination policy changed before publication")
	}
	current, err := os.Lstat(filepath.Dir(item.Destination))
	opened, openedErr := parent.Stat()
	if err != nil || openedErr != nil || current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(current, opened) {
		return invalidSelfHelper("destination parent changed before publication")
	}
	return nil
}

func invalidSelfHelper(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidSelfHelperRequest, reason)
}

// Compile-time proof that the command produced for the helper remains usable
// through the same Runner boundary as all other privileged operations.
var _ Runner = NewElevationRunner(ExecRunner{})
