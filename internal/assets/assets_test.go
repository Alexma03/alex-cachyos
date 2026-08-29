package assets

import (
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
