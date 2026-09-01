package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryResolvesMultipleHostsInDeclaredRoleOrder(t *testing.T) {
	global := mustDocument(t, `catalogVersion: 1
kind: Global
modules:
  bootstrap: true
  verify: true
`)
	workstation := mustDocument(t, `catalogVersion: 1
kind: Role
modules:
  desktop: true
  bootstrap: false
templates:
  - templates/roles/workstation/base
`)
	developer := mustDocument(t, `catalogVersion: 1
kind: Role
modules:
  devtools: true
  bootstrap: true
`)
	portable := mustDocument(t, `catalogVersion: 1
kind: Host
roles: [workstation, developer]
modules:
  verify: false
templates: []
riskPolicy:
  fixedDisplays: true
`)
	minimal := mustDocument(t, `catalogVersion: 1
kind: Host
roles: []
`)

	repository, err := NewRepository(global, map[string]Document{
		"workstation": workstation,
		"developer":   developer,
	}, map[string]Document{
		"portable-fixture": portable,
		"minimal-fixture":  minimal,
	}, nil)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}

	got, err := repository.Resolve("portable-fixture")
	if err != nil {
		t.Fatalf("resolve portable fixture: %v", err)
	}
	if !reflect.DeepEqual(got.Roles, []string{"workstation", "developer"}) {
		t.Fatalf("roles = %#v", got.Roles)
	}
	if !reflect.DeepEqual(got.Desired.Modules, ModuleSet{
		"bootstrap": true,
		"verify":    false,
		"desktop":   true,
		"devtools":  true,
	}) {
		t.Fatalf("modules = %#v", got.Desired.Modules)
	}
	if got.Desired.Templates == nil || len(got.Desired.Templates) != 0 {
		t.Fatalf("host template override = %#v, want explicit empty", got.Desired.Templates)
	}
	if !got.Allows(RiskFixedDisplays) || got.Allows(RiskFingerprintPAM) {
		t.Fatalf("resolved risk policy = %#v", got.Risks)
	}

	other, err := repository.Resolve("minimal-fixture")
	if err != nil {
		t.Fatalf("resolve second host: %v", err)
	}
	if len(other.Roles) != 0 || !other.Desired.Modules["bootstrap"] || !other.Desired.Modules["verify"] {
		t.Fatalf("empty-role host = %#v", other)
	}
	if other.Desired.Templates != nil || other.Desired.Overlays != nil || other.Desired.RiskPolicy != nil {
		t.Fatalf("omitted host fields were synthesized: %#v", other.Desired)
	}
}

func TestRepositoryRejectsUnknownAndDuplicateRoles(t *testing.T) {
	global := mustDocument(t, "catalogVersion: 1\nkind: Global\n")
	role := mustDocument(t, "catalogVersion: 1\nkind: Role\n")
	tests := []struct {
		name  string
		host  string
		roles map[string]Document
		want  string
	}{
		{"unknown", "catalogVersion: 1\nkind: Host\nroles: [missing]\n", map[string]Document{}, "/roles/0"},
		{"duplicate", "catalogVersion: 1\nkind: Host\nroles: [workstation, workstation]\n", map[string]Document{"workstation": role}, "/roles/1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, err := NewRepository(global, test.roles, map[string]Document{"portable-fixture": mustDocument(t, test.host)}, nil)
			if err != nil {
				t.Fatalf("new repository: %v", err)
			}
			_, err = repository.Resolve("portable-fixture")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want path %q", err, test.want)
			}
		})
	}
}

func TestRepositoryRejectsUnknownHostBeforeAssetPort(t *testing.T) {
	lookup := &recordingLookup{}
	repository, err := NewRepository(
		mustDocument(t, "catalogVersion: 1\nkind: Global\n"),
		nil,
		map[string]Document{"portable-fixture": mustDocument(t, "catalogVersion: 1\nkind: Host\ntemplates: [templates/fixture]\n")},
		lookup,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repository.Resolve("invented-production-host")
	if err == nil {
		t.Fatal("unknown host was accepted")
	}
	if lookup.calls != 0 {
		t.Fatalf("asset port called %d times before unknown-host failure", lookup.calls)
	}
	for _, known := range []string{"portable-fixture"} {
		if !strings.Contains(err.Error(), known) {
			t.Fatalf("error %q does not list known host %q", err, known)
		}
	}
	if strings.Contains(err.Error(), "invented-production-host.yaml") {
		t.Fatalf("resolver synthesized a production catalog path: %v", err)
	}
}

func TestRepositoryInputsAndResultsAreImmutableClones(t *testing.T) {
	global := mustDocument(t, "catalogVersion: 1\nkind: Global\nmodules:\n  verify: true\n")
	host := mustDocument(t, "catalogVersion: 1\nkind: Host\nroles: []\nriskPolicy:\n  fingerprintPam: true\n")
	hosts := map[string]Document{"portable-fixture": host}
	repository, err := NewRepository(global, nil, hosts, nil)
	if err != nil {
		t.Fatal(err)
	}

	(*global.Modules)["verify"] = false
	delete(hosts, "portable-fixture")
	first, err := repository.Resolve("portable-fixture")
	if err != nil {
		t.Fatal(err)
	}
	first.Roles = append(first.Roles, "mutated")
	first.Desired.Modules["verify"] = false
	first.Risks.FingerprintPAM = false

	second, err := repository.Resolve("portable-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Desired.Modules["verify"] || !second.Allows(RiskFingerprintPAM) || len(second.Roles) != 0 {
		t.Fatalf("repository state aliased mutable input/result: %#v", second)
	}
}

func TestRiskPolicyIsClosedHostOwnedAndDefaultDeny(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"global roles", "catalogVersion: 1\nkind: Global\nroles: [workstation]\n", "/roles"},
		{"role policy", "catalogVersion: 1\nkind: Role\nriskPolicy:\n  cosmicPrune: true\n", "/riskPolicy"},
		{"unknown risk", "catalogVersion: 1\nkind: Host\nriskPolicy:\n  hardwareAutodetect: true\n", "hardwareAutodetect"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeDocument([]byte(test.data))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want path %q", err, test.want)
			}
		})
	}

	host := mustDocument(t, "catalogVersion: 1\nkind: Host\n")
	repository, err := NewRepository(mustDocument(t, "catalogVersion: 1\nkind: Global\n"), nil, map[string]Document{"portable-fixture": host}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := repository.Resolve("portable-fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range RiskCapabilities() {
		if resolved.Allows(capability) {
			t.Fatalf("omitted capability %q was enabled", capability)
		}
	}
	if resolved.Allows(RiskCapability("observedMatchingHardware")) {
		t.Fatal("runtime observation enabled a non-catalog capability")
	}
}

func TestRiskPolicyDecodesEveryCanonicalCapability(t *testing.T) {
	tests := []struct {
		name       string
		capability RiskCapability
	}{
		{"fingerprintPam", RiskFingerprintPAM},
		{"fixedDisplays", RiskFixedDisplays},
		{"fixedInputDevices", RiskFixedInputDevices},
		{"literalHomePaths", RiskLiteralHomePaths},
		{"bootstrapSystemUpdate", RiskBootstrapSystemUpdate},
		{"bootstrapPackageRemoval", RiskBootstrapPackageRemoval},
		{"bootstrapBootMutation", RiskBootstrapBootMutation},
		{"cosmicPrune", RiskCosmicPrune},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := mustDocument(t, fmt.Sprintf("catalogVersion: 1\nkind: Host\nriskPolicy:\n  %s: true\n", test.name))
			repository, err := NewRepository(
				mustDocument(t, "catalogVersion: 1\nkind: Global\n"),
				nil,
				map[string]Document{"portable-fixture": host},
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := repository.Resolve("portable-fixture")
			if err != nil {
				t.Fatal(err)
			}
			for _, capability := range RiskCapabilities() {
				if got, want := resolved.Allows(capability), capability == test.capability; got != want {
					t.Fatalf("Allows(%q) = %v, want %v", capability, got, want)
				}
			}
		})
	}
}

func TestDigestIncludesOrderedRolesAndCanonicalRiskPolicy(t *testing.T) {
	policy := &RiskPolicy{FingerprintPAM: true, BootstrapBootMutation: true}
	first := Catalog{CatalogVersion: 1, Kind: KindHost, Roles: []string{"workstation", "developer"}, RiskPolicy: policy}
	second := Catalog{CatalogVersion: 1, Kind: KindHost, Roles: []string{"workstation", "developer"}, RiskPolicy: &RiskPolicy{BootstrapBootMutation: true, FingerprintPAM: true}}

	firstDigest, err := Digest(first)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := Digest(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("equal policies have unstable digests: %s != %s", firstDigest, secondDigest)
	}

	reordered := first
	reordered.Roles = []string{"developer", "workstation"}
	reorderedDigest, err := Digest(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if reorderedDigest == firstDigest {
		t.Fatal("role order did not affect digest")
	}

	changed := first
	changed.RiskPolicy = &RiskPolicy{FingerprintPAM: false, BootstrapBootMutation: true}
	changedDigest, err := Digest(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == firstDigest {
		t.Fatal("risk policy change did not affect digest")
	}
}

func mustDocument(t *testing.T, data string) Document {
	t.Helper()
	document, err := DecodeDocument([]byte(data))
	if err != nil {
		t.Fatalf("decode document: %v", err)
	}
	return document
}

type recordingLookup struct{ calls int }

func (l *recordingLookup) Open(string) (fs.File, error) {
	l.calls++
	return nil, errors.New("unexpected asset lookup")
}
