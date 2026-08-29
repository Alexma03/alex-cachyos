package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"alex-cachyos/internal/statepath"
)

func lockTestPaths(t *testing.T) statepath.Paths {
	t.Helper()
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	return statepath.Paths{RuntimeDir: runtimeDir}
}

func TestAcquireLockUsesRuntimeDirAndRestrictiveEmptyFile(t *testing.T) {
	paths := lockTestPaths(t)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "not-used"))
	lock, err := AcquireLock(paths)
	if err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(paths.RuntimeDir, fmt.Sprintf("alex-cachyos-%d.lock", os.Getuid()))
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("lock mode = %s, want regular 0600", info.Mode())
	}
	contents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Fatalf("lock contains %d bytes; lock files must be empty", len(contents))
	}

	if err := lock.Release(); err != nil {
		t.Fatal(err)
	}
	if err := lock.Release(); err != nil {
		t.Fatalf("second Release() = %v, want nil", err)
	}
	again, err := AcquireLock(paths)
	if err != nil {
		t.Fatalf("reacquire after Release() = %v", err)
	}
	if err := again.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestAcquireLockRejectsUnsafeRuntimeDirs(t *testing.T) {
	valid := lockTestPaths(t).RuntimeDir
	link := filepath.Join(filepath.Dir(valid), "runtime-link")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	insecure := filepath.Join(filepath.Dir(valid), "runtime-insecure")
	if err := os.Mkdir(insecure, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(insecure, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(filepath.Dir(valid), "runtime-file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}

	for name, runtimeDir := range map[string]string{
		"empty":     "",
		"relative":  "runtime",
		"tmp":       "/tmp",
		"symlink":   link,
		"insecure":  insecure,
		"not a dir": file,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AcquireLock(statepath.Paths{RuntimeDir: runtimeDir}); err == nil {
				t.Fatalf("AcquireLock(%q) succeeded", runtimeDir)
			}
		})
	}
}

func TestAcquireLockRejectsLockSymlink(t *testing.T) {
	paths := lockTestPaths(t)
	lockPath := filepath.Join(paths.RuntimeDir, fmt.Sprintf("alex-cachyos-%d.lock", os.Getuid()))
	target := filepath.Join(filepath.Dir(paths.RuntimeDir), "lock-target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, lockPath); err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(paths); err == nil {
		t.Fatal("AcquireLock accepted a symlink lock path")
	}
}

func TestAcquireLockReturnsTypedContentionAndReleasesForConcurrentAcquisition(t *testing.T) {
	paths := lockTestPaths(t)
	first, err := AcquireLock(paths)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	result := make(chan error, 1)
	go func() {
		second, err := AcquireLock(paths)
		if second != nil {
			_ = second.Release()
		}
		result <- err
	}()
	err = <-result
	if !errors.Is(err, ErrLockContention) {
		t.Fatalf("concurrent acquisition error = %v, want ErrLockContention", err)
	}
	var contention *LockContentionError
	if !errors.As(err, &contention) || contention.Path == "" {
		t.Fatalf("concurrent acquisition error = %v, want LockContentionError", err)
	}

	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(paths)
	if err != nil {
		t.Fatalf("acquisition after contention = %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}
