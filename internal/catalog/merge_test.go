package catalog

import (
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
