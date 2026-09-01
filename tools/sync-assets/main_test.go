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
		filepath.Join("overlays", "galaxy", "etc", "pam.d"),
		filepath.Join("packaging", "libfprint-egismoc-sdcp-git", "patches"),
		filepath.Join("templates", "apps"),
		filepath.Join("templates", "bootstrap"),
		filepath.Join("templates", "desktop"),
		filepath.Join("templates", "devtools"),
		filepath.Join("templates", "hosts", "galaxy", "fixedDisplays", "niri"),
		filepath.Join("templates", "hosts", "galaxy", "fixedDisplays", "noctalia"),
		filepath.Join("templates", "hosts", "galaxy", "fixedInputDevices", "hyprwhspr"),
		filepath.Join("templates", "hosts", "galaxy", "fixedInputDevices", "noctalia"),
		filepath.Join("templates", "hosts", "galaxy", "literalHomePaths", "noctalia"),
		filepath.Join("templates", "roles", "workstation", "hyprwhspr"),
		filepath.Join("templates", "roles", "workstation", "niri"),
		filepath.Join("templates", "roles", "workstation", "noctalia"),
		filepath.Join("templates", "quickshell-polkit"),
		filepath.Join("templates", "vicinae"),
		"bin",
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	patch := []byte("--- a/egismoc.c\n+++ b/egismoc.c\n")
	for name, data := range map[string][]byte{
		"catalog/declared.txt":                          []byte("declared\n"),
		"catalog/nested/asset.bin":                      {0, 1, 2, 255},
		"overlays/galaxy/etc/pam.d/greetd":              []byte("auth required pam_unix.so\n"),
		"packaging/libfprint-egismoc-sdcp-git/PKGBUILD": []byte("pkgname=example\n"),
		"packaging/libfprint-egismoc-sdcp-git/0001-egismoc-drop-sdcp-claim-on-close.patch":         patch,
		"packaging/libfprint-egismoc-sdcp-git/patches/0001-egismoc-drop-sdcp-claim-on-close.patch": patch,
		"templates/apps/packages.aur":                                     []byte("warp-terminal-bin\n"),
		"templates/apps/packages.pacman":                                  []byte("cursor-bin\n"),
		"templates/apps/webapps.list":                                     []byte("WhatsApp|https://web.whatsapp.com|icon\n"),
		"templates/bootstrap/packages.remove":                             []byte("old-package\n"),
		"templates/bootstrap/packages.want":                               []byte("new-package\n"),
		"templates/desktop/packages.pacman":                               []byte("niri\n"),
		"templates/devtools/mise.config.toml":                             []byte("[tools]\nnode = \"lts\"\n"),
		"templates/devtools/npmrc":                                        []byte("min-release-age=3\n"),
		"templates/devtools/pnpm.config.yaml":                             []byte("minimumReleaseAge: 4320\n"),
		"templates/hosts/galaxy/fixedDisplays/niri/config.kdl":            []byte("output {}\n"),
		"templates/hosts/galaxy/fixedDisplays/noctalia/settings.toml":     []byte("[display]\n"),
		"templates/hosts/galaxy/fixedInputDevices/hyprwhspr/config.json":  []byte("{}\n"),
		"templates/hosts/galaxy/fixedInputDevices/noctalia/settings.toml": []byte("[device]\n"),
		"templates/hosts/galaxy/literalHomePaths/noctalia/settings.toml":  []byte("[paths]\n"),
		"templates/roles/workstation/hyprwhspr/config.json":               []byte("{}\n"),
		"templates/roles/workstation/niri/config.kdl":                     []byte("input {}\n"),
		"templates/roles/workstation/noctalia/settings.toml":              []byte("[settings]\n"),
		"templates/quickshell-polkit/PolkitModel.js":                      []byte("export default {}\n"),
		"templates/quickshell-polkit/shell.qml":                           []byte("Item {}\n"),
		"templates/vicinae/99-vicinae-cosmic.conf":                        []byte("COSMIC_DATA_CONTROL_ENABLED=1\n"),
		"templates/vicinae/cosmic-shortcuts-custom":                       []byte("(modifiers: [Super],): Disable\n"),
		"bin/alex-cachyos-webapp-launch":                                  []byte("#!/usr/bin/env bash\n"),
		"not-declared.txt":                                                []byte("outside\n"),
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
		"bin/alex-cachyos-webapp-launch",
		"catalog/declared.txt",
		"catalog/nested/asset.bin",
		"overlays/galaxy/etc/pam.d/greetd",
		"packaging/libfprint-egismoc-sdcp-git/0001-egismoc-drop-sdcp-claim-on-close.patch",
		"packaging/libfprint-egismoc-sdcp-git/PKGBUILD",
		"packaging/libfprint-egismoc-sdcp-git/patches/0001-egismoc-drop-sdcp-claim-on-close.patch",
		"templates/apps/packages.aur",
		"templates/apps/packages.pacman",
		"templates/apps/webapps.list",
		"templates/bootstrap/packages.remove",
		"templates/bootstrap/packages.want",
		"templates/desktop/packages.pacman",
		"templates/devtools/mise.config.toml",
		"templates/devtools/npmrc",
		"templates/devtools/pnpm.config.yaml",
		"templates/hosts/galaxy/fixedDisplays/niri/config.kdl",
		"templates/hosts/galaxy/fixedDisplays/noctalia/settings.toml",
		"templates/hosts/galaxy/fixedInputDevices/hyprwhspr/config.json",
		"templates/hosts/galaxy/fixedInputDevices/noctalia/settings.toml",
		"templates/hosts/galaxy/literalHomePaths/noctalia/settings.toml",
		"templates/quickshell-polkit/PolkitModel.js",
		"templates/quickshell-polkit/shell.qml",
		"templates/roles/workstation/hyprwhspr/config.json",
		"templates/roles/workstation/niri/config.kdl",
		"templates/roles/workstation/noctalia/settings.toml",
		"templates/vicinae/99-vicinae-cosmic.conf",
		"templates/vicinae/cosmic-shortcuts-custom",
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
	if m.Entries[1].Size != int64(len(data)) || m.Entries[1].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("manifest entry = %#v", m.Entries[1])
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

func TestSyncEmbedsDevtoolsAppsVicinaeLauncher(t *testing.T) {
	root, dest := syncedFixture(t)
	copied := "bin/alex-cachyos-webapp-launch"
	embedded := []string{
		"templates/apps/packages.aur",
		"templates/apps/packages.pacman",
		"templates/apps/webapps.list",
		"templates/bootstrap/packages.remove",
		"templates/bootstrap/packages.want",
		"templates/desktop/packages.pacman",
		"templates/devtools/mise.config.toml",
		"templates/devtools/npmrc",
		"templates/devtools/pnpm.config.yaml",
		"templates/hosts/galaxy/fixedDisplays/niri/config.kdl",
		"templates/hosts/galaxy/fixedDisplays/noctalia/settings.toml",
		"templates/hosts/galaxy/fixedInputDevices/hyprwhspr/config.json",
		"templates/hosts/galaxy/fixedInputDevices/noctalia/settings.toml",
		"templates/hosts/galaxy/literalHomePaths/noctalia/settings.toml",
		"templates/roles/workstation/hyprwhspr/config.json",
		"templates/roles/workstation/niri/config.kdl",
		"templates/roles/workstation/noctalia/settings.toml",
		"templates/quickshell-polkit/PolkitModel.js",
		"templates/quickshell-polkit/shell.qml",
		"templates/vicinae/99-vicinae-cosmic.conf",
		"templates/vicinae/cosmic-shortcuts-custom",
	}
	var m manifest
	mb, err := os.ReadFile(filepath.Join(dest, "source-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &m); err != nil {
		t.Fatal(err)
	}
	byPath := map[string]int{}
	entryByPath := map[string]entry{}
	for _, e := range m.Entries {
		byPath[e.Path]++
		entryByPath[e.Path] = e
	}
	for _, rel := range append([]string{copied}, embedded...) {
		if byPath[rel] != 1 {
			t.Fatalf("manifest has %d entries for %q, want exactly one", byPath[rel], rel)
		}
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(src)
		e := entryByPath[rel]
		if e.Size != int64(len(src)) || e.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("manifest entry mismatch for %s: %#v", rel, e)
		}
		if rel == copied {
			cp, err := os.ReadFile(filepath.Join(dest, rel))
			if err != nil {
				t.Fatalf("generated copied asset %s missing: %v", rel, err)
			}
			if !bytes.Equal(src, cp) {
				t.Fatalf("generated copied asset %s drifted from source", rel)
			}
			continue
		}
		if _, err := os.Lstat(filepath.Join(dest, rel)); !os.IsNotExist(err) {
			t.Fatalf("embedded asset %s was copied: %v", rel, err)
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

func useDeclaredAssets(t *testing.T, declarations ...declaredAsset) {
	t.Helper()
	previous := declaredAssets
	declaredAssets = append([]declaredAsset(nil), declarations...)
	t.Cleanup(func() { declaredAssets = previous })
}

func writeAssetFixture(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedAssetsAreHashedButNotCopied(t *testing.T) {
	root := t.TempDir()
	writeAssetFixture(t, root, "copied.txt", []byte("copied\n"))
	writeAssetFixture(t, root, "embedded/nested.txt", []byte("embedded\n"))
	useDeclaredAssets(t,
		declaredAsset{path: "copied.txt", mode: assetCopied},
		declaredAsset{path: "embedded/nested.txt", mode: assetEmbedded},
	)
	dest := filepath.Join(root, "generated")
	if err := syncAssets(root, dest, false); err != nil {
		t.Fatal(err)
	}
	var got manifest
	manifestBytes, err := os.ReadFile(filepath.Join(dest, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(manifestBytes, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("manifest entries = %#v, want copied and embedded sources", got.Entries)
	}
	byPath := map[string]entry{}
	for _, item := range got.Entries {
		byPath[item.Path] = item
	}
	sum := sha256.Sum256([]byte("embedded\n"))
	if item := byPath["embedded/nested.txt"]; item.Size != int64(len("embedded\n")) || item.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("embedded manifest entry = %#v", item)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "copied.txt")); err != nil || string(data) != "copied\n" {
		t.Fatalf("copied asset = %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "embedded/nested.txt")); !os.IsNotExist(err) {
		t.Fatalf("embedded source was copied: %v", err)
	}
}

func TestEmbeddedDestinationCleanupCheckAndUndeclaredPreservation(t *testing.T) {
	root := t.TempDir()
	writeAssetFixture(t, root, "embedded/owned.txt", []byte("source\n"))
	useDeclaredAssets(t, declaredAsset{path: "embedded/", mode: assetEmbedded})
	dest := filepath.Join(root, "generated")
	owned := filepath.Join(dest, "embedded", "owned.txt")
	keep := filepath.Join(dest, "embedded", "undeclared.txt")
	writeAssetFixture(t, root, filepath.ToSlash(filepath.Join("generated", "embedded", "owned.txt")), []byte("stale\n"))
	writeAssetFixture(t, root, filepath.ToSlash(filepath.Join("generated", "embedded", "undeclared.txt")), []byte("keep\n"))
	if err := syncAssets(root, dest, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(owned); !os.IsNotExist(err) {
		t.Fatalf("embedded destination was not removed: %v", err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep\n" {
		t.Fatalf("undeclared destination changed: %q, %v", data, err)
	}
	if err := os.WriteFile(owned, []byte("stale again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncAssets(root, dest, true); err == nil {
		t.Fatal("--check accepted a stale embedded destination")
	}
	if data, err := os.ReadFile(owned); err != nil || string(data) != "stale again\n" {
		t.Fatalf("--check changed stale embedded destination: %q, %v", data, err)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep\n" {
		t.Fatalf("--check changed undeclared destination: %q, %v", data, err)
	}
}

func TestDeclarationsRejectOverlapDuplicatesAndTraversalBeforeWriting(t *testing.T) {
	tests := []struct {
		name         string
		declarations []declaredAsset
		sources      map[string][]byte
	}{
		{
			name: "duplicate across modes",
			declarations: []declaredAsset{
				{path: "same.txt", mode: assetCopied},
				{path: "same.txt", mode: assetEmbedded},
			},
			sources: map[string][]byte{"same.txt": []byte("same\n")},
		},
		{
			name: "directory and child overlap",
			declarations: []declaredAsset{
				{path: "tree/", mode: assetCopied},
				{path: "tree/child.txt", mode: assetEmbedded},
			},
			sources: map[string][]byte{"tree/child.txt": []byte("child\n")},
		},
		{
			name:         "traversal",
			declarations: []declaredAsset{{path: "../escape.txt", mode: assetEmbedded}},
		},
		{
			name:         "noncanonical traversal",
			declarations: []declaredAsset{{path: "tree/../escape.txt", mode: assetCopied}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, data := range test.sources {
				writeAssetFixture(t, root, rel, data)
			}
			keep := filepath.Join(root, "generated", "keep.txt")
			writeAssetFixture(t, root, filepath.ToSlash(filepath.Join("generated", "keep.txt")), []byte("keep\n"))
			useDeclaredAssets(t, test.declarations...)
			if err := syncAssets(root, filepath.Dir(keep), false); err == nil {
				t.Fatal("unsafe declarations were accepted")
			}
			if data, err := os.ReadFile(keep); err != nil || string(data) != "keep\n" {
				t.Fatalf("failed declaration changed existing destination: %q, %v", data, err)
			}
			if _, err := os.Lstat(filepath.Join(filepath.Dir(keep), manifestName)); !os.IsNotExist(err) {
				t.Fatalf("failed declaration wrote a manifest: %v", err)
			}
		})
	}
}

func TestManifestUnionEntriesAreDeterministic(t *testing.T) {
	root := t.TempDir()
	writeAssetFixture(t, root, "z-copy.txt", []byte("z\n"))
	writeAssetFixture(t, root, "embedded/a.txt", []byte("a\n"))
	writeAssetFixture(t, root, "embedded/nested/m.txt", []byte("m\n"))
	declarations := []declaredAsset{
		{path: "z-copy.txt", mode: assetCopied},
		{path: "embedded/", mode: assetEmbedded},
	}
	useDeclaredAssets(t, declarations...)
	first := filepath.Join(root, "first")
	if err := syncAssets(root, first, false); err != nil {
		t.Fatal(err)
	}
	firstManifest, err := os.ReadFile(filepath.Join(first, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	declaredAssets = []declaredAsset{declarations[1], declarations[0]}
	second := filepath.Join(root, "second")
	if err := syncAssets(root, second, false); err != nil {
		t.Fatal(err)
	}
	secondManifest, err := os.ReadFile(filepath.Join(second, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatalf("manifest changed with declaration order:\n%s\n---\n%s", firstManifest, secondManifest)
	}
	var got manifest
	if err := json.Unmarshal(firstManifest, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{"embedded/a.txt", "embedded/nested/m.txt", "z-copy.txt"}
	if len(got.Entries) != len(want) {
		t.Fatalf("manifest entries = %#v", got.Entries)
	}
	for i, path := range want {
		if got.Entries[i].Path != path {
			t.Fatalf("manifest order = %#v, want %v", got.Entries, want)
		}
	}
}
