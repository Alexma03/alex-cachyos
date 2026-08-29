package assets

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func assetsRoot(t *testing.T) string {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func runAssetTool(root string, args ...string) error {
	cmd := exec.Command("go", append([]string{"run", "./tools/sync-assets"}, args...)...)
	cmd.Dir = root
	return cmd.Run()
}

func TestGenerateCheckAndEmbeddedCopy(t *testing.T) {
	root := assetsRoot(t)
	generate := exec.Command("go", "generate", "./internal/assets/...")
	generate.Dir = root
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("go generate: %v\n%s", err, out)
	}
	if err := runAssetTool(root, "--check"); err != nil {
		t.Fatalf("fresh --check: %v", err)
	}
	copyPath := filepath.Join(root, "internal", "assets", "data", "catalog", ".gitkeep")
	original, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	restored := false
	t.Cleanup(func() {
		if restored {
			return
		}
		if err := runAssetTool(root); err != nil {
			t.Errorf("restore generated assets: %v", err)
		}
	})
	if err := os.WriteFile(copyPath, append(original, 'x'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runAssetTool(root, "--check"); err == nil {
		t.Fatal("--check accepted a perturbed embedded copy")
	}
	if err := runAssetTool(root); err != nil {
		t.Fatalf("restore generated assets: %v", err)
	}
	restored = true
	after, err := os.ReadFile(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("generated copy was not restored")
	}
	if _, err := FS.ReadFile("data/source-manifest.json"); err != nil {
		t.Fatalf("embedded source manifest: %v", err)
	}
}

func TestEmbeddedSourceMountsMatchRepository(t *testing.T) {
	root := assetsRoot(t)
	for _, name := range []string{
		"templates/apps/packages.aur",
		"templates/bootstrap/packages.remove",
		"templates/desktop/packages.pacman",
		"templates/devtools/pnpm.config.yaml",
		"templates/hyprwhspr/config.json",
		"templates/niri/config.kdl",
		"templates/noctalia/settings.toml",
		"templates/quickshell-polkit/shell.qml",
		"templates/vicinae/cosmic-shortcuts-custom",
		"overlays/galaxy/etc/pam.d/cosmic-greeter",
	} {
		want, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read source %q: %v", name, err)
		}
		got, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read embedded %q: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("embedded %q differs from source", name)
		}
	}
}

func TestEmbeddedDataNamespaceRemainsReadable(t *testing.T) {
	root := assetsRoot(t)
	for _, name := range []string{
		"data/catalog/schema/catalog-v1.schema.json",
		"data/source-manifest.json",
	} {
		want, err := os.ReadFile(filepath.Join(root, "internal", "assets", filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read source %q: %v", name, err)
		}
		got, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read embedded %q: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("embedded %q differs from source", name)
		}
	}
}

func TestCompositeFSRejectsMissingAndTraversal(t *testing.T) {
	for _, test := range []struct {
		name string
		want error
	}{
		{"templates/embed.go", fs.ErrNotExist},
		{"overlays/embed.go", fs.ErrNotExist},
		{"templates/niri/missing.kdl", fs.ErrNotExist},
		{"templatesx/niri/config.kdl", fs.ErrNotExist},
		{"../templates/niri/config.kdl", fs.ErrInvalid},
		{"templates/niri/../../../etc/passwd", fs.ErrInvalid},
	} {
		_, err := FS.Open(test.name)
		if !errors.Is(err, test.want) {
			t.Fatalf("Open(%q) error = %v, want %v", test.name, err, test.want)
		}
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("Open(%q) error type = %T, want *fs.PathError", test.name, err)
		}
	}
	if _, err := FS.ReadFile("overlays/galaxy/missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadFile missing error = %v, want fs.ErrNotExist", err)
	} else {
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			t.Fatalf("ReadFile missing error type = %T, want *fs.PathError", err)
		}
	}
}

func TestCompositeFSStatWorksForCatalog(t *testing.T) {
	info, err := fs.Stat(FS, "templates/niri/config.kdl")
	if err != nil {
		t.Fatalf("stat template: %v", err)
	}
	if info.IsDir() || info.Size() == 0 {
		t.Fatalf("template info = mode %v size %d", info.Mode(), info.Size())
	}
	info, err = fs.Stat(FS, "overlays/galaxy")
	if err != nil {
		t.Fatalf("stat overlay: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("overlay mode = %v, want directory", info.Mode())
	}
}

type errorFS struct{ err error }

func (e errorFS) Open(string) (fs.File, error) { return nil, e.err }

func TestCompositeFSPropagatesMountErrors(t *testing.T) {
	mountErr := errors.New("mount failure")
	fsys := compositeFS{mounts: []mount{{prefix: "templates", fsys: errorFS{mountErr}}}}
	if _, err := fsys.Open("templates/file"); !errors.Is(err, mountErr) {
		t.Fatalf("Open error = %v, want mount error", err)
	}
	if _, err := fsys.ReadFile("templates/file"); !errors.Is(err, mountErr) {
		t.Fatalf("ReadFile error = %v, want mount error", err)
	}
}
