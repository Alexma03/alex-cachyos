package catalog

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func decodeMergeDocument(t *testing.T, data string) Document {
	t.Helper()
	document, err := DecodeDocument([]byte(data))
	if err != nil {
		t.Fatalf("decode catalog document: %v", err)
	}
	return document
}

func TestMergeDocumentsUsesPresenceAwareLayerPrecedence(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
modules:
  bootstrap: true
  verify: true
templates:
  - global-template
overlays:
  - global-overlay
checkoutPins:
  shared:
    remote: https://example.invalid/global.git
    branch: main
    commit: global
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
modules:
  bootstrap: false
  desktop: true
checkoutPins:
  shared:
    remote: https://example.invalid/role.git
    branch: release
    commit: role
  role-only:
    remote: https://example.invalid/role-only.git
    branch: main
    commit: role-only
`)
	host := decodeMergeDocument(t, `catalogVersion: 1
kind: Host
modules:
  verify: false
templates: []
overlays: []
checkoutPins: {}
`)

	got, err := MergeDocuments([]Document{global, role, host})
	if err != nil {
		t.Fatalf("merge documents: %v", err)
	}
	wantModules := ModuleSet{
		"bootstrap": false,
		"desktop":   true,
		"verify":    false,
	}
	if !reflect.DeepEqual(got.Modules, wantModules) {
		t.Fatalf("modules = %#v, want %#v", got.Modules, wantModules)
	}
	if got.Templates == nil || len(got.Templates) != 0 {
		t.Fatalf("templates = %#v, want explicit empty list", got.Templates)
	}
	if got.Overlays == nil || len(got.Overlays) != 0 {
		t.Fatalf("overlays = %#v, want explicit empty list", got.Overlays)
	}
	wantPins := map[string]CheckoutPin{
		"shared":    {Remote: "https://example.invalid/role.git", Branch: "release", Commit: "role"},
		"role-only": {Remote: "https://example.invalid/role-only.git", Branch: "main", Commit: "role-only"},
	}
	if !reflect.DeepEqual(got.CheckoutPins, wantPins) {
		t.Fatalf("checkout pins = %#v, want %#v", got.CheckoutPins, wantPins)
	}
	if got.CatalogVersion != 1 || got.Kind != KindHost {
		t.Fatalf("scalar result = version %d, kind %q; want 1, Host", got.CatalogVersion, got.Kind)
	}
}

func TestMergeDocumentsAcceptsAnEmptyRoleDocument(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
modules:
  verify: true
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
`)
	host := decodeMergeDocument(t, `catalogVersion: 1
kind: Host
`)

	got, err := MergeDocuments([]Document{global, role, host})
	if err != nil {
		t.Fatalf("merge empty role document: %v", err)
	}
	if !reflect.DeepEqual(got.Modules, ModuleSet{"verify": true}) {
		t.Fatalf("modules = %#v, want inherited global module", got.Modules)
	}
	if got.Templates != nil || got.Overlays != nil || got.CheckoutPins != nil {
		t.Fatalf("omitted fields were not preserved as omitted: %#v", got)
	}
}

func TestMergeDocumentsReplacesPresentOrderedListsAsWhole(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
templates:
  - global-first
  - global-second
overlays:
  - global-overlay
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
templates:
  - role-only
overlays:
  - role-overlay
`)
	host := decodeMergeDocument(t, `catalogVersion: 1
kind: Host
`)

	got, err := MergeDocuments([]Document{global, role, host})
	if err != nil {
		t.Fatalf("merge ordered lists: %v", err)
	}
	if !reflect.DeepEqual(got.Templates, []string{"role-only"}) {
		t.Fatalf("templates = %#v, want role replacement", got.Templates)
	}
	if !reflect.DeepEqual(got.Overlays, []string{"role-overlay"}) {
		t.Fatalf("overlays = %#v, want role replacement", got.Overlays)
	}
}

func TestMergeDocumentsRejectsInvalidCheckoutPinsWithNamedPaths(t *testing.T) {
	version := 1
	kind := KindGlobal
	remote := "https://example.invalid/repository.git"
	commit := "abc123"
	pins := map[string]CheckoutPinDocument{
		"missing-branch": {Remote: &remote, Commit: &commit},
	}

	_, err := MergeDocuments([]Document{{
		CatalogVersion: &version,
		Kind:           &kind,
		CheckoutPins:   &pins,
	}})
	if err == nil {
		t.Fatal("invalid checkout pin was accepted")
	}
	if !strings.Contains(err.Error(), "/checkoutPins/missing-branch/branch") {
		t.Fatalf("error %q does not name the invalid checkout-pin field", err)
	}
}

func TestMergeDocumentsRejectsNilCheckoutPinMaps(t *testing.T) {
	version := 1
	kind := KindGlobal
	var pins map[string]CheckoutPinDocument

	_, err := MergeDocuments([]Document{{
		CatalogVersion: &version,
		Kind:           &kind,
		CheckoutPins:   &pins,
	}})
	if err == nil {
		t.Fatal("nil checkout-pin map was accepted")
	}
	if !strings.Contains(err.Error(), "/checkoutPins") {
		t.Fatalf("error %q does not name the checkoutPins field", err)
	}
}

func TestMergeDocumentsMergesPinsByPresenceAcrossLayers(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
pins:
  npm:
    shared: npm:shared@1.0.0
    global-only: npm:global-only@1.0.0
  localPathPackages:
    global-path: not-a-resolved-path
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
pins:
  npm:
    shared: npm:shared@2.0.0
    role-only: npm:role-only@2.0.0
`)
	host := decodeMergeDocument(t, `catalogVersion: 1
kind: Host
pins:
  localPathPackages: {}
`)

	got, err := MergeDocuments([]Document{global, role, host})
	if err != nil {
		t.Fatalf("merge pins: %v", err)
	}
	if got.Pins == nil {
		t.Fatal("merged pins are nil")
	}
	wantNPM := map[string]string{
		"shared":      "npm:shared@2.0.0",
		"global-only": "npm:global-only@1.0.0",
		"role-only":   "npm:role-only@2.0.0",
	}
	if !reflect.DeepEqual(got.Pins.NPM, wantNPM) {
		t.Fatalf("npm = %#v, want %#v", got.Pins.NPM, wantNPM)
	}
	wantLocal := map[string]string{"global-path": "not-a-resolved-path"}
	if !reflect.DeepEqual(got.Pins.LocalPathPackages, wantLocal) {
		t.Fatalf("local = %#v, want %#v", got.Pins.LocalPathPackages, wantLocal)
	}
}

func TestMergeDocumentsPinsTopLevelPresenceAndOmission(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
pins:
  npm:
    keep: npm:keep@1.0.0
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
`)
	host := decodeMergeDocument(t, `catalogVersion: 1
kind: Host
pins: {}
`)

	got, err := MergeDocuments([]Document{global, role, host})
	if err != nil {
		t.Fatalf("merge pins: %v", err)
	}
	if got.Pins == nil {
		t.Fatal("pins presence lost")
	}
	if got.Pins.NPM["keep"] != "npm:keep@1.0.0" {
		t.Fatalf("inherited npm pin lost: %#v", got.Pins.NPM)
	}
}

func TestMergeDocumentsPinsExplicitEmptySourceMapsPreserveInheritedKeys(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
pins:
  npm:
    keep: npm:keep@1.0.0
  localPathPackages:
    keep-path: not-a-resolved-path
`)
	role := decodeMergeDocument(t, `catalogVersion: 1
kind: Role
pins:
  npm: {}
  localPathPackages: {}
`)

	got, err := MergeDocuments([]Document{global, role})
	if err != nil {
		t.Fatalf("merge pins: %v", err)
	}
	if got.Pins == nil || got.Pins.NPM == nil || got.Pins.LocalPathPackages == nil {
		t.Fatal("source presence lost")
	}
	if got.Pins.NPM["keep"] != "npm:keep@1.0.0" {
		t.Fatalf("npm inherited key lost: %#v", got.Pins.NPM)
	}
	if got.Pins.LocalPathPackages["keep-path"] != "not-a-resolved-path" {
		t.Fatalf("local inherited key lost: %#v", got.Pins.LocalPathPackages)
	}
}

func TestMergeDocumentsPinsDoesNotAliasInputMaps(t *testing.T) {
	global := decodeMergeDocument(t, `catalogVersion: 1
kind: Global
pins:
  npm:
    provider: npm:provider@1.0.0
  localPathPackages:
    gentle-pi: not-a-resolved-path
`)

	got, err := MergeDocuments([]Document{global})
	if err != nil {
		t.Fatalf("merge pins: %v", err)
	}
	(*global.Pins.NPM)["provider"] = "changed"
	delete(*global.Pins.LocalPathPackages, "gentle-pi")
	if got.Pins.NPM["provider"] != "npm:provider@1.0.0" {
		t.Fatalf("npm pins alias input map: %#v", got.Pins.NPM)
	}
	if got.Pins.LocalPathPackages["gentle-pi"] != "not-a-resolved-path" {
		t.Fatalf("local pins alias input map: %#v", got.Pins.LocalPathPackages)
	}
}

func TestMergeDocumentsPinsRejectsNilSourceMapsWithNamedPaths(t *testing.T) {
	version := 1
	kind := KindGlobal
	tests := []struct {
		name string
		pins *PinsDocument
		path string
	}{
		{name: "npm", pins: &PinsDocument{NPM: nilStringMap()}, path: "/pins/npm"},
		{name: "localPathPackages", pins: &PinsDocument{LocalPathPackages: nilStringMap()}, path: "/pins/localPathPackages"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MergeDocuments([]Document{{
				CatalogVersion: &version,
				Kind:           &kind,
				Pins:           test.pins,
			}})
			if err == nil {
				t.Fatal("nil pin source map was accepted")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name %q", err, test.path)
			}
		})
	}
}

func TestMergeDocumentsPinsIssuesAreStableAndSorted(t *testing.T) {
	version := 1
	kind := KindGlobal

	_, err := MergeDocuments([]Document{{
		CatalogVersion: &version,
		Kind:           &kind,
		Pins:           &PinsDocument{NPM: nilStringMap(), LocalPathPackages: nilStringMap()},
	}})
	if err == nil {
		t.Fatal("nil pin source maps were accepted")
	}
	var catErr *CatalogValidationError
	if !errors.As(err, &catErr) {
		t.Fatalf("error %v is not a CatalogValidationError", err)
	}
	issues := catErr.Errors()
	if len(issues) != 2 {
		t.Fatalf("issues = %d, want 2", len(issues))
	}
	if issues[0].Path != "/pins/localPathPackages" || issues[1].Path != "/pins/npm" {
		t.Fatalf("issues not sorted: %#v", issues)
	}
}

func nilStringMap() *map[string]string {
	var m map[string]string
	return &m
}

func TestMergeDocumentsMergesTypedPinMapsByPresenceAcrossLayers(t *testing.T) {
	shaA, shaB, shaC := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)
	commitA, commitB, commitC := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	version, kind := 1, KindGlobal

	global := typedPinsDocument(
		map[string]PacmanArtifactPinDocument{
			"shared":      pacmanPinDocument("linux", "6.8.0-1", "cache", shaA),
			"global-only": pacmanPinDocument("vim", "9.1.0-1", "archive", shaB),
		},
		map[string]AURLocalPinDocument{
			"shared":      aurPinDocument(commitA, shaA),
			"global-only": aurPinDocument(commitB, shaB),
		},
		map[string]RemoteArtifactPinDocument{
			"shared":      remotePinDocument("https://example.invalid/global/shared.tar.gz", shaA),
			"global-only": remotePinDocument("https://example.invalid/global/only.tar.gz", shaB),
		},
	)
	role := typedPinsDocument(
		map[string]PacmanArtifactPinDocument{
			"shared":    pacmanPinDocument("linux-lts", "6.8.1-1", "archive", shaC),
			"role-only": pacmanPinDocument("neovim", "0.10.0-1", "cache", shaA),
		},
		map[string]AURLocalPinDocument{
			"shared":    aurPinDocument(commitC, shaC),
			"role-only": aurPinDocument(commitA, shaA),
		},
		map[string]RemoteArtifactPinDocument{
			"shared":    remotePinDocument("https://example.invalid/role/shared.tar.gz", shaC),
			"role-only": remotePinDocument("https://example.invalid/role/only.tar.gz", shaA),
		},
	)

	got, err := MergeDocuments([]Document{
		{CatalogVersion: &version, Kind: &kind, Pins: global},
		{CatalogVersion: &version, Kind: &kind, Pins: role},
	})
	if err != nil {
		t.Fatalf("merge typed pins: %v", err)
	}
	if got.Pins == nil {
		t.Fatal("merged pins are nil")
	}
	wantPacman := map[string]PacmanArtifactPin{
		"shared":      {Package: "linux-lts", Version: "6.8.1-1", Source: PacmanArtifactArchive, SHA256: shaC},
		"global-only": {Package: "vim", Version: "9.1.0-1", Source: PacmanArtifactArchive, SHA256: shaB},
		"role-only":   {Package: "neovim", Version: "0.10.0-1", Source: PacmanArtifactCache, SHA256: shaA},
	}
	if !reflect.DeepEqual(got.Pins.PacmanArtifacts, wantPacman) {
		t.Fatalf("pacmanArtifacts = %#v, want %#v", got.Pins.PacmanArtifacts, wantPacman)
	}
	wantAUR := map[string]AURLocalPin{
		"shared":      {SourceCommit: commitC, PatchSHA256: shaC},
		"global-only": {SourceCommit: commitB, PatchSHA256: shaB},
		"role-only":   {SourceCommit: commitA, PatchSHA256: shaA},
	}
	if !reflect.DeepEqual(got.Pins.AURLocal, wantAUR) {
		t.Fatalf("aurLocal = %#v, want %#v", got.Pins.AURLocal, wantAUR)
	}
	wantRemote := map[string]RemoteArtifactPin{
		"shared":      {URL: "https://example.invalid/role/shared.tar.gz", SHA256: shaC},
		"global-only": {URL: "https://example.invalid/global/only.tar.gz", SHA256: shaB},
		"role-only":   {URL: "https://example.invalid/role/only.tar.gz", SHA256: shaA},
	}
	if !reflect.DeepEqual(got.Pins.RemoteArtifacts, wantRemote) {
		t.Fatalf("remoteArtifacts = %#v, want %#v", got.Pins.RemoteArtifacts, wantRemote)
	}
}

func TestMergeDocumentsTypedPinMapsExplicitEmptyPreserveInheritedKeys(t *testing.T) {
	sha := strings.Repeat("a", 64)
	commit := strings.Repeat("a", 40)
	version, kind := 1, KindGlobal

	global := typedPinsDocument(
		map[string]PacmanArtifactPinDocument{"keep": pacmanPinDocument("linux", "6.8.0-1", "cache", sha)},
		map[string]AURLocalPinDocument{"keep": aurPinDocument(commit, sha)},
		map[string]RemoteArtifactPinDocument{"keep": remotePinDocument("https://example.invalid/keep.tar.gz", sha)},
	)
	role := typedPinsDocument(
		map[string]PacmanArtifactPinDocument{},
		map[string]AURLocalPinDocument{},
		map[string]RemoteArtifactPinDocument{},
	)

	got, err := MergeDocuments([]Document{
		{CatalogVersion: &version, Kind: &kind, Pins: global},
		{CatalogVersion: &version, Kind: &kind, Pins: role},
	})
	if err != nil {
		t.Fatalf("merge typed pins: %v", err)
	}
	if got.Pins == nil || got.Pins.PacmanArtifacts == nil || got.Pins.AURLocal == nil || got.Pins.RemoteArtifacts == nil {
		t.Fatal("typed source presence lost")
	}
	if want := (PacmanArtifactPin{Package: "linux", Version: "6.8.0-1", Source: PacmanArtifactCache, SHA256: sha}); got.Pins.PacmanArtifacts["keep"] != want {
		t.Fatalf("pacman inherited key lost: %#v", got.Pins.PacmanArtifacts)
	}
	if want := (AURLocalPin{SourceCommit: commit, PatchSHA256: sha}); got.Pins.AURLocal["keep"] != want {
		t.Fatalf("aur inherited key lost: %#v", got.Pins.AURLocal)
	}
	if want := (RemoteArtifactPin{URL: "https://example.invalid/keep.tar.gz", SHA256: sha}); got.Pins.RemoteArtifacts["keep"] != want {
		t.Fatalf("remote inherited key lost: %#v", got.Pins.RemoteArtifacts)
	}
}

func TestMergeDocumentsTypedPinMapsDoNotAliasInputMaps(t *testing.T) {
	sha := strings.Repeat("a", 64)
	commit := strings.Repeat("a", 40)
	version, kind := 1, KindGlobal

	global := typedPinsDocument(
		map[string]PacmanArtifactPinDocument{"provider": pacmanPinDocument("linux", "6.8.0-1", "cache", sha)},
		map[string]AURLocalPinDocument{"provider": aurPinDocument(commit, sha)},
		map[string]RemoteArtifactPinDocument{"provider": remotePinDocument("https://example.invalid/provider.tar.gz", sha)},
	)

	got, err := MergeDocuments([]Document{{CatalogVersion: &version, Kind: &kind, Pins: global}})
	if err != nil {
		t.Fatalf("merge typed pins: %v", err)
	}
	(*global.PacmanArtifacts)["provider"] = pacmanPinDocument("changed", "0.0.0-1", "archive", sha)
	delete(*global.AURLocal, "provider")
	(*global.RemoteArtifacts)["provider"] = remotePinDocument("https://example.invalid/changed.tar.gz", sha)

	if got.Pins.PacmanArtifacts["provider"].Package != "linux" {
		t.Fatalf("pacman pins alias input map: %#v", got.Pins.PacmanArtifacts)
	}
	if _, ok := got.Pins.AURLocal["provider"]; !ok {
		t.Fatalf("aur pins alias input map: %#v", got.Pins.AURLocal)
	}
	if got.Pins.RemoteArtifacts["provider"].URL != "https://example.invalid/provider.tar.gz" {
		t.Fatalf("remote pins alias input map: %#v", got.Pins.RemoteArtifacts)
	}
}

func TestMergeDocumentsRejectsNilTypedPinMapsWithNamedPaths(t *testing.T) {
	version, kind := 1, KindGlobal
	tests := []struct {
		name string
		pins *PinsDocument
		path string
	}{
		{name: "pacmanArtifacts", pins: &PinsDocument{PacmanArtifacts: nilPacmanArtifactMap()}, path: "/pins/pacmanArtifacts"},
		{name: "aurLocal", pins: &PinsDocument{AURLocal: nilAURLocalMap()}, path: "/pins/aurLocal"},
		{name: "remoteArtifacts", pins: &PinsDocument{RemoteArtifacts: nilRemoteArtifactMap()}, path: "/pins/remoteArtifacts"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MergeDocuments([]Document{{CatalogVersion: &version, Kind: &kind, Pins: test.pins}})
			if err == nil {
				t.Fatal("nil typed pin source map was accepted")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name %q", err, test.path)
			}
		})
	}
}

func TestMergeDocumentsRejectsMissingTypedPinFieldsWithNamedPaths(t *testing.T) {
	version, kind := 1, KindGlobal
	sha := strings.Repeat("a", 64)
	commit := strings.Repeat("a", 40)

	tests := []struct {
		name string
		pins *PinsDocument
		path string
	}{
		{
			name: "pacmanArtifacts sha256",
			pins: typedPinsDocument(
				map[string]PacmanArtifactPinDocument{"broken": {Package: strPtr("linux"), Version: strPtr("6.8.0-1"), Source: strPtr("cache")}},
				map[string]AURLocalPinDocument{},
				map[string]RemoteArtifactPinDocument{},
			),
			path: "/pins/pacmanArtifacts/broken/sha256",
		},
		{
			name: "aurLocal patchSHA256",
			pins: typedPinsDocument(
				map[string]PacmanArtifactPinDocument{},
				map[string]AURLocalPinDocument{"broken": {SourceCommit: strPtr(commit)}},
				map[string]RemoteArtifactPinDocument{},
			),
			path: "/pins/aurLocal/broken/patchSHA256",
		},
		{
			name: "remoteArtifacts url",
			pins: typedPinsDocument(
				map[string]PacmanArtifactPinDocument{},
				map[string]AURLocalPinDocument{},
				map[string]RemoteArtifactPinDocument{"broken": {SHA256: strPtr(sha)}},
			),
			path: "/pins/remoteArtifacts/broken/url",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := MergeDocuments([]Document{{CatalogVersion: &version, Kind: &kind, Pins: test.pins}})
			if err == nil {
				t.Fatal("missing typed pin field was accepted")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name %q", err, test.path)
			}
		})
	}
}

func TestMergeDocumentsTypedPinIssuesAreStableAndSorted(t *testing.T) {
	version, kind := 1, KindGlobal
	commit := strings.Repeat("a", 40)
	pins := &PinsDocument{
		PacmanArtifacts: nilPacmanArtifactMap(),
		AURLocal:        &map[string]AURLocalPinDocument{"broken": {SourceCommit: &commit}},
		RemoteArtifacts: nilRemoteArtifactMap(),
	}

	_, err := MergeDocuments([]Document{{CatalogVersion: &version, Kind: &kind, Pins: pins}})
	if err == nil {
		t.Fatal("invalid typed pin sources were accepted")
	}
	var catErr *CatalogValidationError
	if !errors.As(err, &catErr) {
		t.Fatalf("error %v is not a CatalogValidationError", err)
	}
	issues := catErr.Errors()
	if len(issues) != 3 {
		t.Fatalf("issues = %d, want 3", len(issues))
	}
	want := []string{"/pins/aurLocal/broken/patchSHA256", "/pins/pacmanArtifacts", "/pins/remoteArtifacts"}
	for i, path := range want {
		if issues[i].Path != path {
			t.Fatalf("issues = %#v, want path %q at index %d", issues, path, i)
		}
	}
}

func strPtr(value string) *string { return &value }

func pacmanPinDocument(packageName, version, source, sha string) PacmanArtifactPinDocument {
	return PacmanArtifactPinDocument{Package: strPtr(packageName), Version: strPtr(version), Source: strPtr(source), SHA256: strPtr(sha)}
}

func aurPinDocument(commit, patch string) AURLocalPinDocument {
	return AURLocalPinDocument{SourceCommit: strPtr(commit), PatchSHA256: strPtr(patch)}
}

func remotePinDocument(url, sha string) RemoteArtifactPinDocument {
	return RemoteArtifactPinDocument{URL: strPtr(url), SHA256: strPtr(sha)}
}

func typedPinsDocument(pacman map[string]PacmanArtifactPinDocument, aur map[string]AURLocalPinDocument, remote map[string]RemoteArtifactPinDocument) *PinsDocument {
	return &PinsDocument{PacmanArtifacts: &pacman, AURLocal: &aur, RemoteArtifacts: &remote}
}

func nilPacmanArtifactMap() *map[string]PacmanArtifactPinDocument {
	var m map[string]PacmanArtifactPinDocument
	return &m
}

func nilAURLocalMap() *map[string]AURLocalPinDocument {
	var m map[string]AURLocalPinDocument
	return &m
}

func nilRemoteArtifactMap() *map[string]RemoteArtifactPinDocument {
	var m map[string]RemoteArtifactPinDocument
	return &m
}
