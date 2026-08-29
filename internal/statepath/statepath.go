// Package statepath resolves configurator-owned XDG locations without using Pi's state.
package statepath

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Paths struct {
	StateHome, DataHome, CacheHome, RuntimeDir string
}

func Resolve() (Paths, error) {
	home := os.Getenv("HOME")
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve home: %w", err)
		}
	}
	var err error
	home, err = filepath.Abs(home)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home: %w", err)
	}
	pi := filepath.Join(home, ".pi")
	var p Paths
	if p.StateHome, err = xdg("XDG_STATE_HOME", filepath.Join(home, ".local", "state"), pi); err != nil {
		return Paths{}, err
	}
	if p.DataHome, err = xdg("XDG_DATA_HOME", filepath.Join(home, ".local", "share"), pi); err != nil {
		return Paths{}, err
	}
	if p.CacheHome, err = xdg("XDG_CACHE_HOME", filepath.Join(home, ".cache"), pi); err != nil {
		return Paths{}, err
	}
	p.RuntimeDir, err = runtime(os.Getenv("XDG_RUNTIME_DIR"), pi)
	return p, err
}

func xdg(name, fallback, pi string) (string, error) {
	path := os.Getenv(name)
	if path == "" {
		path = fallback
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%s must be absolute", name)
	}
	path = filepath.Clean(path)
	if within(path, pi) {
		return "", fmt.Errorf("%s must not use Pi state", name)
	}
	return path, nil
}

func runtime(path, pi string) (string, error) {
	if path == "" {
		path = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR must be absolute")
	}
	path = filepath.Clean(path)
	if within(path, pi) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR must not use Pi state")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("runtime directory %q unavailable; refusing /tmp fallback: %w", path, err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0700 || !ok || uint32(st.Uid) != uint32(os.Getuid()) {
		return "", fmt.Errorf("runtime directory %q must be an owned 0700 directory", path)
	}
	return path, nil
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
