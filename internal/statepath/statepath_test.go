package statepath

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func setup(t *testing.T) (string, string) {
	t.Helper()
	home, run := t.TempDir(), filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(run, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", run)
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	return home, run
}

func under(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestXDGDefaultsOverrideAndPiBoundary(t *testing.T) {
	home, _ := setup(t)
	for _, tc := range []struct{ state, want string }{
		{"", filepath.Join(home, ".local", "state")},
		{filepath.Join(home, "custom-state"), filepath.Join(home, "custom-state")},
	} {
		t.Setenv("XDG_STATE_HOME", tc.state)
		got, err := Resolve()
		if err != nil {
			t.Fatal(err)
		}
		if got.StateHome != tc.want || got.DataHome != filepath.Join(home, ".local", "share") || got.CacheHome != filepath.Join(home, ".cache") {
			t.Fatalf("paths = %#v, want state=%q and XDG defaults", got, tc.want)
		}
		for _, path := range []string{got.StateHome, got.DataHome, got.CacheHome, got.RuntimeDir} {
			if under(path, filepath.Join(home, ".pi")) {
				t.Fatalf("path escaped into Pi state: %q", path)
			}
		}
	}
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".pi", "state"))
	if _, err := Resolve(); err == nil {
		t.Fatal("Pi state path was accepted")
	}
}

func TestRuntimeIsSecureAndHasNoTmpFallback(t *testing.T) {
	home, run := setup(t)
	got, err := Resolve()
	if err != nil || got.RuntimeDir != run || !filepath.IsAbs(got.RuntimeDir) {
		t.Fatalf("runtime = %#v, %v", got, err)
	}
	bad := filepath.Join(home, "bad")
	if err := os.Mkdir(bad, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", bad)
	if _, err := Resolve(); err == nil {
		t.Fatal("world-readable runtime directory was accepted")
	}
	t.Setenv("XDG_RUNTIME_DIR", "relative")
	if _, err := Resolve(); err == nil {
		t.Fatal("relative runtime directory was accepted")
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	p, err := Resolve()
	if err == nil && p.RuntimeDir != filepath.Join("/run/user", strconv.Itoa(os.Getuid())) {
		t.Fatalf("unset runtime used unsafe fallback: %q", p.RuntimeDir)
	}
}
