package catalog

import (
	"os"
	"strings"
	"testing"
)

func TestDecodeCatalogMinimal(t *testing.T) {
	got, err := Decode([]byte("catalogVersion: 1\nkind: Global\n"))
	if err != nil {
		t.Fatalf("decode minimal catalog: %v", err)
	}
	if got.CatalogVersion != 1 || got.Kind != "Global" {
		t.Fatalf("decoded catalog = %#v", got)
	}
}

func TestDecodeReturnsTypedCheckoutPins(t *testing.T) {
	got, err := Decode([]byte("catalogVersion: 1\nkind: Host\nmodules:\n  verify: false\ncheckoutPins:\n  gentle-ai:\n    remote: https://example.invalid/gentle-ai.git\n    branch: main\n    commit: abc\n"))
	if err != nil {
		t.Fatal(err)
	}
	if enabled, ok := got.Modules["verify"]; !ok || enabled {
		t.Fatalf("typed module = %v, present=%v", enabled, ok)
	}
	want := CheckoutPin{Remote: "https://example.invalid/gentle-ai.git", Branch: "main", Commit: "abc"}
	if got.CheckoutPins["gentle-ai"] != want {
		t.Fatalf("typed checkout pin = %#v, want %#v", got.CheckoutPins["gentle-ai"], want)
	}
}

func TestDecodeRejectsInvalidYAMLDocuments(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{"unknown top-level field", "catalogVersion: 1\nkind: Global\nunexpected: true\n", "unexpected"},
		{"unknown nested field", "catalogVersion: 1\nkind: Global\ncheckoutPins:\n  ai:\n    remote: https://example.invalid/ai.git\n    branch: main\n    commit: abc\n    unexpected: true\n", "unexpected"},
		{"duplicate key", "catalogVersion: 1\nkind: Global\nkind: Host\n", "kind"},
		{"multiple documents", "catalogVersion: 1\nkind: Global\n---\ncatalogVersion: 1\nkind: Host\n", "multiple"},
		{"missing version", "kind: Global\n", "catalogVersion"},
		{"missing kind", "catalogVersion: 1\n", "kind"},
		{"unsupported version", "catalogVersion: 2\nkind: Global\n", "catalogVersion"},
		{"unsupported kind", "catalogVersion: 1\nkind: Profile\n", "kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.data)); err == nil {
				t.Fatal("invalid document was accepted")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not name %q", err, test.want)
			}
		})
	}
}

func TestDecodePreservesOptionalFieldPresence(t *testing.T) {
	omitted, err := DecodeDocument([]byte("catalogVersion: 1\nkind: Global\n"))
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := DecodeDocument([]byte("catalogVersion: 1\nkind: Host\nmodules:\n  verify: false\ntemplates: []\ncheckoutPins: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if omitted.Modules != nil || omitted.Templates != nil || omitted.CheckoutPins != nil {
		t.Fatalf("omitted fields retained presence: %#v", omitted)
	}
	if explicit.Modules == nil || explicit.Templates == nil || explicit.CheckoutPins == nil {
		t.Fatalf("explicit fields lost presence: %#v", explicit)
	}
	if enabled, ok := (*explicit.Modules)["verify"]; !ok || enabled {
		t.Fatalf("explicit false module = %v, present=%v", enabled, ok)
	}
	if omitted.Modules != nil {
		if _, ok := (*omitted.Modules)["verify"]; ok {
			t.Fatal("omitted module unexpectedly present")
		}
	}
	if len(*explicit.Templates) != 0 || len(*explicit.CheckoutPins) != 0 {
		t.Fatal("explicit empty collections were not preserved")
	}
}

func TestDecodeIsPureWithTemporaryWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	before, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode([]byte("catalogVersion: 1\nkind: Global\nmodules:\n  verify: false\n")); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("decode wrote files in cwd: before=%d after=%d", len(before), len(after))
	}
}
