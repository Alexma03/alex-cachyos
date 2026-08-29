package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assetFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join("catalog", "nested"),
		filepath.Join("packaging", "libfprint-egismoc-sdcp-git", "patches"),
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	patch := []byte("--- a/egismoc.c\n+++ b/egismoc.c\n")
	for name, data := range map[string][]byte{
		"catalog/declared.txt":                          []byte("declared\n"),
		"catalog/nested/asset.bin":                      {0, 1, 2, 255},
		"packaging/libfprint-egismoc-sdcp-git/PKGBUILD": []byte("pkgname=example\n"),
		"packaging/libfprint-egismoc-sdcp-git/0001-egismoc-drop-sdcp-claim-on-close.patch":         patch,
		"packaging/libfprint-egismoc-sdcp-git/patches/0001-egismoc-drop-sdcp-claim-on-close.patch": patch,
		"not-declared.txt": []byte("outside\n"),
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
	want := []string{
		"catalog/declared.txt",
		"catalog/nested/asset.bin",
		"packaging/libfprint-egismoc-sdcp-git/0001-egismoc-drop-sdcp-claim-on-close.patch",
		"packaging/libfprint-egismoc-sdcp-git/PKGBUILD",
		"packaging/libfprint-egismoc-sdcp-git/patches/0001-egismoc-drop-sdcp-claim-on-close.patch",
	}
	if len(m.Entries) != len(want) {
		t.Fatalf("manifest entries = %#v", m.Entries)
	}
	for i, w := range want {
		if m.Entries[i].Path != w {
			t.Fatalf("manifest entries = %#v", m.Entries)
		}
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
func TestSyncEmbedsPackagingAssets(t *testing.T) {
	root, dest := syncedFixture(t)
	base := filepath.Join(dest, "packaging", "libfprint-egismoc-sdcp-git")
	rels := []string{
		"PKGBUILD",
		"0001-egismoc-drop-sdcp-claim-on-close.patch",
		"patches/0001-egismoc-drop-sdcp-claim-on-close.patch",
	}
	src := make(map[string][]byte, len(rels))
	for _, rel := range rels {
		s, err := os.ReadFile(filepath.Join(root, "packaging", "libfprint-egismoc-sdcp-git", rel))
		if err != nil {
			t.Fatal(err)
		}
		c, err := os.ReadFile(filepath.Join(base, rel))
		if err != nil {
			t.Fatalf("generated packaging asset %s missing: %v", rel, err)
		}
		if !bytes.Equal(s, c) {
			t.Fatalf("generated packaging asset %s drifted from source", rel)
		}
		src[rel] = s
	}
	// Both patch locations must be present and byte-identical.
	if !bytes.Equal(src["0001-egismoc-drop-sdcp-claim-on-close.patch"], src["patches/0001-egismoc-drop-sdcp-claim-on-close.patch"]) {
		t.Fatal("patch copies differ between the two locations")
	}
	// Manifest must record byte-identical hashes for each packaging copy.
	var m manifest
	mb, err := os.ReadFile(filepath.Join(dest, "source-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range m.Entries {
		if !strings.HasPrefix(e.Path, "packaging/") {
			continue
		}
		rel := strings.TrimPrefix(e.Path, "packaging/libfprint-egismoc-sdcp-git/")
		s, ok := src[rel]
		if !ok {
			t.Fatalf("manifest references unexpected packaging path %q", e.Path)
		}
		sum := sha256.Sum256(s)
		if e.SHA256 != hex.EncodeToString(sum[:]) || e.Size != int64(len(s)) {
			t.Fatalf("manifest hash/size mismatch for %s: %#v", e.Path, e)
		}
		seen[rel] = true
	}
	for _, rel := range rels {
		if !seen[rel] {
			t.Fatalf("manifest missing packaging entry %q", rel)
		}
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
