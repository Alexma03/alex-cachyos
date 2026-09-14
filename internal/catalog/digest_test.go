package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestNormalizeCanonicalizesMapsAndPreservesListOrder(t *testing.T) {
	first := Catalog{
		CatalogVersion: 1,
		Kind:           KindHost,
		Modules: moduleSetInOrder(
			[]string{"verify", "bootstrap"},
			[]bool{false, true},
		),
		Templates: []string{"templates/second", "templates/first"},
		Overlays:  []string{"overlays/second", "overlays/first"},
		CheckoutPins: checkoutPinsInOrder(
			[]string{"zeta", "alpha"},
			[]CheckoutPin{
				{Remote: "https://example.invalid/zeta.git", Branch: "main", Commit: "zeta"},
				{Remote: "https://example.invalid/alpha.git", Branch: "main", Commit: "alpha"},
			},
		),
	}
	second := Catalog{
		CatalogVersion: 1,
		Kind:           KindHost,
		Modules: moduleSetInOrder(
			[]string{"bootstrap", "verify"},
			[]bool{true, false},
		),
		Templates: []string{"templates/second", "templates/first"},
		Overlays:  []string{"overlays/second", "overlays/first"},
		CheckoutPins: checkoutPinsInOrder(
			[]string{"alpha", "zeta"},
			[]CheckoutPin{
				{Remote: "https://example.invalid/alpha.git", Branch: "main", Commit: "alpha"},
				{Remote: "https://example.invalid/zeta.git", Branch: "main", Commit: "zeta"},
			},
		),
	}

	const want = `{"catalogVersion":1,"kind":"Host","modules":{"bootstrap":true,"verify":false},"templates":["templates/second","templates/first"],"overlays":["overlays/second","overlays/first"],"checkoutPins":{"alpha":{"remote":"https://example.invalid/alpha.git","branch":"main","commit":"alpha"},"zeta":{"remote":"https://example.invalid/zeta.git","branch":"main","commit":"zeta"}}}
`
	for i := 0; i < 3; i++ {
		got, err := Normalize(first)
		if err != nil {
			t.Fatalf("normalize first catalog: %v", err)
		}
		if string(got) != want {
			t.Fatalf("normalized first catalog = %q, want %q", got, want)
		}
		secondBytes, err := Normalize(second)
		if err != nil {
			t.Fatalf("normalize second catalog: %v", err)
		}
		if string(secondBytes) != string(got) {
			t.Fatalf("normalization differs by map insertion order: %q != %q", secondBytes, got)
		}
		if strings.HasSuffix(string(got[:len(got)-1]), "\n") {
			t.Fatal("normalized catalog has more than one trailing newline")
		}
	}
}

func TestNormalizePreservesOptionalCollectionPresence(t *testing.T) {
	omitted, err := Normalize(Catalog{CatalogVersion: 1, Kind: KindGlobal})
	if err != nil {
		t.Fatalf("normalize omitted collections: %v", err)
	}
	if got, want := string(omitted), "{\"catalogVersion\":1,\"kind\":\"Global\"}\n"; got != want {
		t.Fatalf("normalized omitted collections = %q, want %q", got, want)
	}

	empty, err := Normalize(Catalog{
		CatalogVersion: 1,
		Kind:           KindGlobal,
		Modules:        ModuleSet{},
		Templates:      []string{},
		Overlays:       []string{},
		CheckoutPins:   map[string]CheckoutPin{},
	})
	if err != nil {
		t.Fatalf("normalize explicit empty collections: %v", err)
	}
	const want = "{\"catalogVersion\":1,\"kind\":\"Global\",\"modules\":{},\"templates\":[],\"overlays\":[],\"checkoutPins\":{}}\n"
	if string(empty) != want {
		t.Fatalf("normalized explicit empty collections = %q, want %q", empty, want)
	}
}

func TestDigestMatchesNormalizeAndChangesForContentOrListOrder(t *testing.T) {
	catalog := Catalog{
		CatalogVersion: 1,
		Kind:           KindRole,
		Modules:        ModuleSet{"bootstrap": true},
		Templates:      []string{"templates/first", "templates/second"},
		Overlays:       []string{"overlays/first", "overlays/second"},
	}
	normalized, err := Normalize(catalog)
	if err != nil {
		t.Fatalf("normalize catalog: %v", err)
	}
	digest, err := Digest(catalog)
	if err != nil {
		t.Fatalf("digest catalog: %v", err)
	}
	sum := sha256.Sum256(normalized)
	if digest != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %q, want SHA-256 of normalized bytes %q", digest, hex.EncodeToString(sum[:]))
	}
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		t.Fatalf("digest = %q, want lowercase 64-hex", digest)
	}

	contentChange := catalog
	contentChange.Modules = ModuleSet{"bootstrap": false}
	changedDigest, err := Digest(contentChange)
	if err != nil {
		t.Fatalf("digest changed catalog: %v", err)
	}
	if changedDigest == digest {
		t.Fatal("content change did not change digest")
	}

	listOrderChange := catalog
	listOrderChange.Templates = []string{"templates/second", "templates/first"}
	listDigest, err := Digest(listOrderChange)
	if err != nil {
		t.Fatalf("digest reordered catalog: %v", err)
	}
	if listDigest == digest {
		t.Fatal("ordered-list change did not change digest")
	}
}

func moduleSetInOrder(names []string, values []bool) ModuleSet {
	modules := make(ModuleSet, len(names))
	for i, name := range names {
		modules[name] = values[i]
	}
	return modules
}

func checkoutPinsInOrder(names []string, pins []CheckoutPin) map[string]CheckoutPin {
	checkoutPins := make(map[string]CheckoutPin, len(names))
	for i, name := range names {
		checkoutPins[name] = pins[i]
	}
	return checkoutPins
}

func TestNormalizeCanonicalizesSourcePins(t *testing.T) {
	sha := strings.Repeat("a", 64)
	commit := strings.Repeat("b", 40)
	patch := strings.Repeat("c", 64)
	remoteSHA := strings.Repeat("d", 64)

	first := Catalog{
		CatalogVersion: 1,
		Kind:           KindHost,
		Pins: &Pins{
			NPM: map[string]string{
				"zeta":  "npm:zeta@1.0.0",
				"alpha": "npm:alpha@2.0.0",
			},
			PacmanArtifacts: map[string]PacmanArtifactPin{
				"two": {Package: "package-two", Version: "2.0.0-1", Source: PacmanArtifactArchive, SHA256: sha},
				"one": {Package: "package-one", Version: "1.0.0-1", Source: PacmanArtifactCache, SHA256: sha},
			},
			AURLocal: map[string]AURLocalPin{
				"driver": {SourceCommit: commit, PatchSHA256: patch},
			},
			RemoteArtifacts: map[string]RemoteArtifactPin{
				"tool": {URL: "https://downloads.example.invalid/tool.tar.zst", SHA256: remoteSHA},
			},
			LocalPathPackages: map[string]string{
				"local-tool": "../../Projects/local-tool",
			},
		},
	}
	second := Catalog{
		CatalogVersion: 1,
		Kind:           KindHost,
		Pins: &Pins{
			NPM: map[string]string{
				"alpha": "npm:alpha@2.0.0",
				"zeta":  "npm:zeta@1.0.0",
			},
			PacmanArtifacts: map[string]PacmanArtifactPin{
				"one": {Package: "package-one", Version: "1.0.0-1", Source: PacmanArtifactCache, SHA256: sha},
				"two": {Package: "package-two", Version: "2.0.0-1", Source: PacmanArtifactArchive, SHA256: sha},
			},
			AURLocal: map[string]AURLocalPin{
				"driver": {SourceCommit: commit, PatchSHA256: patch},
			},
			RemoteArtifacts: map[string]RemoteArtifactPin{
				"tool": {URL: "https://downloads.example.invalid/tool.tar.zst", SHA256: remoteSHA},
			},
			LocalPathPackages: map[string]string{
				"local-tool": "../../Projects/local-tool",
			},
		},
	}

	want := `{"catalogVersion":1,"kind":"Host","pins":{"npm":{"alpha":"npm:alpha@2.0.0","zeta":"npm:zeta@1.0.0"},"pacmanArtifacts":{"one":{"package":"package-one","version":"1.0.0-1","source":"cache","sha256":"` + sha + `"},"two":{"package":"package-two","version":"2.0.0-1","source":"archive","sha256":"` + sha + `"}},"aurLocal":{"driver":{"sourceCommit":"` + commit + `","patchSHA256":"` + patch + `"}},"remoteArtifacts":{"tool":{"url":"https://downloads.example.invalid/tool.tar.zst","sha256":"` + remoteSHA + `"}},"localPathPackages":{"local-tool":"../../Projects/local-tool"}}}
`

	firstBytes, err := Normalize(first)
	if err != nil {
		t.Fatalf("normalize first catalog: %v", err)
	}
	if string(firstBytes) != want {
		t.Fatalf("normalized first catalog = %q, want %q", firstBytes, want)
	}
	secondBytes, err := Normalize(second)
	if err != nil {
		t.Fatalf("normalize second catalog: %v", err)
	}
	if string(secondBytes) != want {
		t.Fatalf("normalized second catalog = %q, want %q", secondBytes, want)
	}
}

func TestNormalizePreservesSourcePinPresence(t *testing.T) {
	emptyPins, err := Normalize(Catalog{CatalogVersion: 1, Kind: KindGlobal, Pins: &Pins{}})
	if err != nil {
		t.Fatalf("normalize empty pins: %v", err)
	}
	if got, want := string(emptyPins), "{\"catalogVersion\":1,\"kind\":\"Global\",\"pins\":{}}\n"; got != want {
		t.Fatalf("normalized empty pins = %q, want %q", got, want)
	}

	emptyNPM, err := Normalize(Catalog{
		CatalogVersion: 1,
		Kind:           KindGlobal,
		Pins:           &Pins{NPM: map[string]string{}},
	})
	if err != nil {
		t.Fatalf("normalize empty npm source: %v", err)
	}
	if got, want := string(emptyNPM), "{\"catalogVersion\":1,\"kind\":\"Global\",\"pins\":{\"npm\":{}}}\n"; got != want {
		t.Fatalf("normalized empty npm source = %q, want %q", got, want)
	}
}

func TestDigestChangesForSourcePinContentAndPresence(t *testing.T) {
	catalog := Catalog{CatalogVersion: 1, Kind: KindGlobal}
	base, err := Digest(catalog)
	if err != nil {
		t.Fatalf("digest base catalog: %v", err)
	}

	withPins := catalog
	withPins.Pins = &Pins{NPM: map[string]string{"provider": "npm:@scope/provider@1.2.3"}}
	pinsDigest, err := Digest(withPins)
	if err != nil {
		t.Fatalf("digest catalog with pins: %v", err)
	}
	if pinsDigest == base {
		t.Fatal("adding pins did not change digest")
	}

	contentChange := catalog
	contentChange.Pins = &Pins{NPM: map[string]string{"provider": "npm:@scope/provider@1.2.4"}}
	changedDigest, err := Digest(contentChange)
	if err != nil {
		t.Fatalf("digest catalog with changed pin: %v", err)
	}
	if changedDigest == pinsDigest {
		t.Fatal("pin content change did not change digest")
	}
}

func TestNormalizeDoesNotMutateSourcePins(t *testing.T) {
	pins := &Pins{
		NPM:               map[string]string{"z": "npm:z@1.0.0", "a": "npm:a@2.0.0"},
		LocalPathPackages: map[string]string{"local-tool": "../../Projects/local-tool"},
	}
	catalog := Catalog{CatalogVersion: 1, Kind: KindGlobal, Pins: pins}

	if _, err := Normalize(catalog); err != nil {
		t.Fatalf("normalize catalog: %v", err)
	}

	if len(pins.NPM) != 2 || pins.NPM["z"] != "npm:z@1.0.0" || pins.NPM["a"] != "npm:a@2.0.0" {
		t.Fatalf("normalize mutated npm pins: %#v", pins.NPM)
	}
	if len(pins.LocalPathPackages) != 1 || pins.LocalPathPackages["local-tool"] != "../../Projects/local-tool" {
		t.Fatalf("normalize mutated local path packages: %#v", pins.LocalPathPackages)
	}
}
