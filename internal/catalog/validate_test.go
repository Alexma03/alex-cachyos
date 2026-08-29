package catalog

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

type countingAssetLookup struct{ calls int }

func (l *countingAssetLookup) Open(string) (fs.File, error) {
	l.calls++
	return nil, fs.ErrNotExist
}

func TestKnownModules(t *testing.T) {
	want := []string{"bootstrap", "fingerprint", "devtools", "apps", "vicinae", "desktop", "verify"}
	if len(KnownModules) != len(want) {
		t.Fatalf("known modules = %v, want %v", KnownModules, want)
	}
	for i := range want {
		if KnownModules[i] != want[i] {
			t.Fatalf("known modules = %v, want %v", KnownModules, want)
		}
	}
}

func TestLoadRejectsInvalidCatalogs(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"unsupported version", "catalogVersion: 2\nkind: Host\n", "/catalogVersion"},
		{"unknown field", "catalogVersion: 1\nkind: Host\nunexpected: true\n", "unexpected"},
		{"unknown module", "catalogVersion: 1\nkind: Host\nmodules:\n  mystery: true\n", "mystery"},
		{"missing asset", "catalogVersion: 1\nkind: Host\ntemplates:\n  - templates/missing.kdl\n", "templates/missing.kdl"},
		{"checkout shape", "catalogVersion: 1\nkind: Host\ncheckoutPins:\n  gentle-ai:\n    remote: https://example.invalid/gentle-ai.git\n    branch: main\n", "commit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Load([]byte(test.data), nil)
			if err == nil {
				t.Fatal("invalid catalog was accepted")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not name %q", err, test.want)
			}
		})
	}
}

func TestValidationDoesNotWriteToWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	before, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	lookup := &countingAssetLookup{}
	if err := Validate(Catalog{CatalogVersion: 1, Kind: KindHost}, lookup); err != nil {
		t.Fatal(err)
	}
	if lookup.calls != 0 {
		t.Fatalf("validation performed %d asset lookups without references", lookup.calls)
	}
	after, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("validation wrote files in cwd: before=%d after=%d", len(before), len(after))
	}
}

func TestValidateUsesInjectedAssetLookup(t *testing.T) {
	lookup := fstest.MapFS{
		"templates/niri/config.kdl": {Data: []byte("layout {}\n")},
		"overlays/galaxy":           {Mode: fs.ModeDir},
	}
	catalog := Catalog{
		CatalogVersion: 1,
		Kind:           KindHost,
		Templates:      []string{"templates/niri/config.kdl"},
		Overlays:       []string{"overlays/galaxy"},
	}
	if err := Validate(catalog, lookup); err != nil {
		t.Fatalf("injected assets rejected: %v", err)
	}
}

func TestCatalogFixtures(t *testing.T) {
	read := func(t *testing.T, name string) []byte {
		t.Helper()
		data, err := os.ReadFile("../../testdata/catalog/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Run("valid host has no external asset dependency", func(t *testing.T) {
		data := read(t, "galaxy-minimal.yaml")
		t.Chdir(t.TempDir())
		got, err := LoadEmbedded(data)
		if err != nil {
			t.Fatal(err)
		}
		if got.Kind != KindHost || got.Modules["verify"] != true {
			t.Fatalf("catalog = %#v", got)
		}
	})
	for _, test := range []struct {
		name string
		want string
	}{
		{"invalid-version.yaml", "catalogVersion"},
		{"invalid-module.yaml", "mystery"},
		{"invalid-asset.yaml", "templates/missing.kdl"},
		{"invalid-checkout-pin.yaml", "commit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadEmbedded(read(t, test.name))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want offending entry %q", err, test.want)
			}
		})
	}
}
