//go:build linux

// Package safefile provides descriptor-relative bounded reads below a trusted root.
package safefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var (
	ErrInvalidPath        = errors.New("invalid relative path")
	ErrUnsafePath         = errors.New("unsafe path")
	ErrUnsafeRoot         = errors.New("unsafe trusted root")
	ErrNotRegular         = errors.New("not a regular file")
	ErrNotDirectory       = errors.New("not a directory")
	ErrTooLarge           = errors.New("file exceeds read limit")
	ErrChanged            = errors.New("file changed while reading")
	ErrInvalidLimit       = errors.New("invalid read limit")
	ErrOpenat2Unavailable = errors.New("openat2 unavailable")
)

type Metadata struct {
	Mode    os.FileMode
	Size    int64
	ModTime time.Time
	UID     uint32
	GID     uint32
}

type ReadResult struct {
	Data     []byte
	Metadata Metadata
}

type Root struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func OpenRoot(path string) (*Root, error) {
	if path == "" || !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
		return nil, fmt.Errorf("%w: root must be absolute and NUL-free", ErrUnsafeRoot)
	}
	path = filepath.Clean(path)
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW),
		Resolve: unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, path, how)
	if err != nil {
		return nil, classifyOpenError(ErrUnsafeRoot, path, err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: create root descriptor", ErrUnsafeRoot)
	}
	return &Root{file: file, path: path}, nil
}

func ReadRegularFile(rootPath, relativePath string, maxBytes int64) ([]byte, error) {
	root, err := OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.ReadRegularFile(relativePath, maxBytes)
}

func (r *Root) ReadRegularFile(relativePath string, maxBytes int64) ([]byte, error) {
	result, err := r.ReadRegularFileWithMetadata(relativePath, maxBytes)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (r *Root) ReadRegularFileWithMetadata(relativePath string, maxBytes int64) (ReadResult, error) {
	if err := validateRelativePath(relativePath); err != nil {
		return ReadResult{}, err
	}
	if maxBytes <= 0 {
		return ReadResult{}, ErrInvalidLimit
	}
	file, err := r.openRelative(relativePath, false)
	if err != nil {
		return ReadResult{}, err
	}
	result, readErr := readOpened(file, relativePath, maxBytes)
	closeErr := file.Close()
	if readErr != nil {
		return ReadResult{}, readErr
	}
	if closeErr != nil {
		return ReadResult{}, fmt.Errorf("close %q: %w", relativePath, closeErr)
	}
	return result, nil
}

func (r *Root) OpenDir(relativePath string) (*Root, error) {
	if err := validateRelativePath(relativePath); err != nil {
		return nil, err
	}
	file, err := r.openRelative(relativePath, true)
	if err != nil {
		return nil, err
	}
	return &Root{file: file, path: filepath.Join(r.path, relativePath)}, nil
}

func (r *Root) Stat() (Metadata, error) {
	if r == nil {
		return Metadata{}, errors.New("nil trusted root")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return Metadata{}, os.ErrClosed
	}
	snapshot, err := snapshotMetadata(r.file)
	return snapshot.Metadata, err
}

// ReadDirNames returns a stable snapshot of the names in the already-open
// directory descriptor. Callers must still open and validate every selected
// entry through this Root; names alone never grant file authority.
func (r *Root) ReadDirNames() ([]string, error) {
	if r == nil {
		return nil, errors.New("nil trusted root")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil, os.ErrClosed
	}
	if _, err := r.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind %q: %w", r.path, err)
	}
	entries, err := r.file.ReadDir(-1)
	if err != nil {
		return nil, fmt.Errorf("read directory %q: %w", r.path, err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	sort.Strings(names)
	return names, nil
}

func (r *Root) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	file := r.file
	r.file = nil
	return file.Close()
}

func (r *Root) openRelative(path string, directory bool) (*os.File, error) {
	if r == nil {
		return nil, errors.New("nil trusted root")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil, os.ErrClosed
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}
	how := &unix.OpenHow{Flags: uint64(flags), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS}
	fd, err := unix.Openat2(int(r.file.Fd()), path, how)
	if err != nil {
		return nil, classifyOpenError(ErrUnsafePath, path, err)
	}
	file := os.NewFile(uintptr(fd), filepath.Join(r.path, path))
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("%w: create descriptor for %q", ErrUnsafePath, path)
	}
	return file, nil
}

type metadataSnapshot struct {
	Metadata
	dev       uint64
	ino       uint64
	ctimeSec  int64
	ctimeNsec int64
}

func snapshotMetadata(file *os.File) (metadataSnapshot, error) {
	info, err := file.Stat()
	if err != nil {
		return metadataSnapshot{}, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return metadataSnapshot{}, err
	}
	return metadataSnapshot{
		Metadata: Metadata{Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime(), UID: stat.Uid, GID: stat.Gid},
		dev:      uint64(stat.Dev), ino: stat.Ino, ctimeSec: stat.Ctim.Sec, ctimeNsec: stat.Ctim.Nsec,
	}, nil
}

func (m metadataSnapshot) same(other metadataSnapshot) bool {
	return m.Metadata == other.Metadata && m.dev == other.dev && m.ino == other.ino && m.ctimeSec == other.ctimeSec && m.ctimeNsec == other.ctimeNsec
}

func readOpened(file *os.File, path string, maxBytes int64) (ReadResult, error) {
	before, err := snapshotMetadata(file)
	if err != nil {
		return ReadResult{}, fmt.Errorf("inspect %q: %w", path, err)
	}
	if !before.Mode.IsRegular() {
		return ReadResult{}, fmt.Errorf("%w: %q", ErrNotRegular, path)
	}
	if before.Size < 0 || before.Size > maxBytes {
		return ReadResult{}, fmt.Errorf("%w: %q", ErrTooLarge, path)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return ReadResult{}, fmt.Errorf("read %q: %w", path, err)
	}
	if int64(len(data)) > maxBytes {
		return ReadResult{}, fmt.Errorf("%w: %q", ErrTooLarge, path)
	}
	after, err := snapshotMetadata(file)
	if err != nil {
		return ReadResult{}, fmt.Errorf("inspect %q after read: %w", path, err)
	}
	if !before.same(after) || int64(len(data)) != before.Size {
		return ReadResult{}, fmt.Errorf("%w: %q", ErrChanged, path)
	}
	return ReadResult{Data: data, Metadata: after.Metadata}, nil
}

func validateRelativePath(path string) error {
	if path == "" || filepath.IsAbs(path) || filepath.VolumeName(path) != "" || filepath.Clean(path) != path || strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("%w: path must be strict and relative", ErrInvalidPath)
	}
	for _, component := range strings.Split(path, string(filepath.Separator)) {
		if component == "" || component == "." || component == ".." {
			return ErrInvalidPath
		}
	}
	return nil
}

func classifyOpenError(class error, path string, err error) error {
	if errors.Is(err, unix.ENOSYS) {
		return fmt.Errorf("%w: %v", ErrOpenat2Unavailable, err)
	}
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ENOTDIR) {
		return fmt.Errorf("%w: open %q: %v", class, path, err)
	}
	return fmt.Errorf("open %q: %w", path, err)
}
