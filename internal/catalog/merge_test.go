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

func TestMergeDocumentsUnsupportedPinSourcesFailClosed(t *testing.T) {
	tests := []struct {
		name      string
		pins      *PinsDocument
		wantPaths []string
	}{
		{name: "pacmanArtifacts", pins: &PinsDocument{PacmanArtifacts: emptyPacmanArtifactMap()}, wantPaths: []string{"/pins/pacmanArtifacts"}},
		{name: "aurLocal", pins: &PinsDocument{AURLocal: emptyAURLocalMap()}, wantPaths: []string{"/pins/aurLocal"}},
		{name: "remoteArtifacts", pins: &PinsDocument{RemoteArtifacts: emptyRemoteArtifactMap()}, wantPaths: []string{"/pins/remoteArtifacts"}},
		{name: "combined", pins: &PinsDocument{
			PacmanArtifacts: emptyPacmanArtifactMap(),
			AURLocal:        emptyAURLocalMap(),
			RemoteArtifacts: emptyRemoteArtifactMap(),
		}, wantPaths: []string{"/pins/aurLocal", "/pins/pacmanArtifacts", "/pins/remoteArtifacts"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := 1
			kind := KindGlobal
			_, err := MergeDocuments([]Document{{
				CatalogVersion: &version,
				Kind:           &kind,
				Pins:           test.pins,
			}})
			if err == nil {
				t.Fatal("unsupported pin source was accepted")
			}
			var catErr *CatalogValidationError
			if !errors.As(err, &catErr) {
				t.Fatalf("error %v is not a CatalogValidationError", err)
			}
			issues := catErr.Errors()
			if len(issues) != len(test.wantPaths) {
				t.Fatalf("issues = %#v, want %d", issues, len(test.wantPaths))
			}
			for i, want := range test.wantPaths {
				if issues[i].Path != want {
					t.Fatalf("issues = %#v, want path %q at index %d", issues, want, i)
				}
			}
		})
	}
}

func emptyPacmanArtifactMap() *map[string]PacmanArtifactPinDocument {
	m := map[string]PacmanArtifactPinDocument{}
	return &m
}

func emptyAURLocalMap() *map[string]AURLocalPinDocument {
	m := map[string]AURLocalPinDocument{}
	return &m
}

func emptyRemoteArtifactMap() *map[string]RemoteArtifactPinDocument {
	m := map[string]RemoteArtifactPinDocument{}
	return &m
}
