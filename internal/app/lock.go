package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"alex-cachyos/internal/statepath"
)

var ErrLockContention = errors.New("lock contention")

// LockContentionError identifies the lock path that was already held.
type LockContentionError struct {
	Path string
}

func (e *LockContentionError) Error() string {
	if e == nil {
		return ErrLockContention.Error()
	}
	return fmt.Sprintf("lock contention on %q", e.Path)
}

func (e *LockContentionError) Unwrap() error { return ErrLockContention }

// Lock holds the process-wide apply lock until Release is called.
type Lock struct {
	file       *os.File
	once       sync.Once
	releaseErr error
}

// AcquireLock validates paths.RuntimeDir and takes the user-scoped apply lock
// without waiting for another mutating run to finish.
func AcquireLock(paths statepath.Paths) (*Lock, error) {
	runtimeDir, err := validateRuntimeDir(paths.RuntimeDir)
	if err != nil {
		return nil, err
	}

	lockPath := filepath.Join(runtimeDir, fmt.Sprintf("alex-cachyos-%d.lock", os.Getuid()))
	file, err := openLockFile(lockPath)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		closeErr := file.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			contention := &LockContentionError{Path: lockPath}
			if closeErr != nil {
				return nil, errors.Join(contention, fmt.Errorf("close lock: %w", closeErr))
			}
			return nil, contention
		}
		if closeErr != nil {
			return nil, errors.Join(fmt.Errorf("acquire lock %q: %w", lockPath, err), fmt.Errorf("close lock: %w", closeErr))
		}
		return nil, fmt.Errorf("acquire lock %q: %w", lockPath, err)
	}
	return &Lock{file: file}, nil
}

// Release unlocks and closes the lock. It is safe to call more than once.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.file == nil {
			return
		}
		file := l.file
		unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		closeErr := file.Close()
		l.file = nil
		switch {
		case unlockErr != nil && closeErr != nil:
			l.releaseErr = errors.Join(fmt.Errorf("unlock lock: %w", unlockErr), fmt.Errorf("close lock: %w", closeErr))
		case unlockErr != nil:
			l.releaseErr = fmt.Errorf("unlock lock: %w", unlockErr)
		case closeErr != nil:
			l.releaseErr = fmt.Errorf("close lock: %w", closeErr)
		}
	})
	return l.releaseErr
}

func validateRuntimeDir(runtimeDir string) (string, error) {
	if runtimeDir == "" || !filepath.IsAbs(runtimeDir) {
		return "", errors.New("runtime directory must be an absolute path")
	}
	runtimeDir = filepath.Clean(runtimeDir)
	if runtimeDir == "/tmp" {
		return "", errors.New("runtime directory /tmp is not allowed")
	}

	info, err := os.Lstat(runtimeDir)
	if err != nil {
		return "", fmt.Errorf("inspect runtime directory %q: %w", runtimeDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("runtime directory %q must not be a symlink", runtimeDir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("runtime directory %q is not a directory", runtimeDir)
	}
	if info.Mode().Perm() != 0700 {
		return "", fmt.Errorf("runtime directory %q must be mode 0700", runtimeDir)
	}
	if !ownedByCurrentUser(info) {
		return "", fmt.Errorf("runtime directory %q is not owned by the current user", runtimeDir)
	}

	resolved, err := filepath.EvalSymlinks(runtimeDir)
	if err != nil {
		return "", fmt.Errorf("resolve runtime directory %q: %w", runtimeDir, err)
	}
	if filepath.Clean(resolved) != runtimeDir {
		return "", fmt.Errorf("runtime directory %q contains a symlink", runtimeDir)
	}
	return runtimeDir, nil
}

func openLockFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("lock path %q must be a regular non-symlink file", path)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect lock path %q: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock %q: %w", path, err)
	}
	closeOnError := func(err error) (*os.File, error) {
		if closeErr := file.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close lock: %w", closeErr))
		}
		return nil, err
	}

	info, err = file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("stat lock %q: %w", path, err))
	}
	if !info.Mode().IsRegular() {
		return closeOnError(fmt.Errorf("lock path %q is not a regular file", path))
	}
	if !ownedByCurrentUser(info) {
		return closeOnError(fmt.Errorf("lock path %q is not owned by the current user", path))
	}
	if err := file.Chmod(0600); err != nil {
		return closeOnError(fmt.Errorf("restrict lock %q: %w", path, err))
	}
	return file, nil
}

func ownedByCurrentUser(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint32(st.Uid) == uint32(os.Getuid())
}
