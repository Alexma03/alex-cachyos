package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func assetFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "catalog", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"catalog/declared.txt":     []byte("declared\n"),
		"catalog/nested/asset.bin": {0, 1, 2, 255},
		"not-declared.txt":         []byte("outside\n"),
	} {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func syncedFixture(t *testing.T) (string, string) {
	root := assetFixture(t)
	dest := filepath.Join(root, "internal", "assets", "data")
	if err := syncAssets(root, dest, false); err != nil {
		t.Fatal(err)
	}
	return root, dest
}

func TestSyncWritesManifestAndOnlyDeclaredAssets(t *testing.T) {
	root, dest := syncedFixture(t)
	var m manifest
	b, err := os.ReadFile(filepath.Join(dest, "source-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	canonical, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	if string(b) != string(canonical) {
		t.Fatal("manifest is not canonical JSON with one trailing newline")
	}
	if len(m.Entries) != 2 || m.Entries[0].Path != "catalog/declared.txt" || m.Entries[1].Path != "catalog/nested/asset.bin" {
		t.Fatalf("manifest entries = %#v", m.Entries)
	}
	data := []byte("declared\n")
	sum := sha256.Sum256(data)
	if m.Entries[0].Size != int64(len(data)) || m.Entries[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("manifest entry = %#v", m.Entries[0])
	}
	if _, err := os.Stat(filepath.Join(dest, "not-declared.txt")); !os.IsNotExist(err) {
		t.Fatalf("undeclared source was copied: %v", err)
	}
	if err := syncAssets(root, filepath.Join(root, "..", "outside"), false); err == nil {
		t.Fatal("outside destination was accepted")
	}
}
func TestSyncRejectsDestinationSymlink(t *testing.T) {
	root := assetFixture(t)
	dest := filepath.Join(root, "internal", "assets", "data")
	outside := t.TempDir()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dest, "catalog")); err != nil {
		t.Fatal(err)
	}
	if err := syncAssets(root, dest, false); err == nil {
		t.Fatal("destination symlink was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "declared.txt")); !os.IsNotExist(err) {
		t.Fatal("asset was written through destination symlink")
	}
}

func TestCheckRejectsCopyDriftWithoutWriting(t *testing.T) {
	root, dest := syncedFixture(t)
	manifestPath := filepath.Join(dest, "source-manifest.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(dest, "catalog", "declared.txt")
	if err := os.WriteFile(copyPath, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncAssets(root, dest, true); err == nil {
		t.Fatal("--check accepted a drifted copy")
	}
	if after, err := os.ReadFile(copyPath); err != nil || string(after) != "changed\n" {
		t.Fatalf("--check rewrote the drifted copy: %v", err)
	}
	if after, err := os.ReadFile(manifestPath); err != nil || string(after) != string(before) {
		t.Fatalf("--check changed the manifest: %v", err)
	}
}
