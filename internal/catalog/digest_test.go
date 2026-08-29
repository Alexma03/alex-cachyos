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
			[]string{"zeta", "gentle-ai"},
			[]CheckoutPin{
				{Remote: "https://example.invalid/zeta.git", Branch: "main", Commit: "zeta"},
				{Remote: "https://example.invalid/gentle-ai.git", Branch: "main", Commit: "gentle-ai"},
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
			[]string{"gentle-ai", "zeta"},
			[]CheckoutPin{
				{Remote: "https://example.invalid/gentle-ai.git", Branch: "main", Commit: "gentle-ai"},
				{Remote: "https://example.invalid/zeta.git", Branch: "main", Commit: "zeta"},
			},
		),
	}

	const want = `{"catalogVersion":1,"kind":"Host","modules":{"bootstrap":true,"verify":false},"templates":["templates/second","templates/first"],"overlays":["overlays/second","overlays/first"],"checkoutPins":{"gentle-ai":{"remote":"https://example.invalid/gentle-ai.git","branch":"main","commit":"gentle-ai"},"zeta":{"remote":"https://example.invalid/zeta.git","branch":"main","commit":"zeta"}}}
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
