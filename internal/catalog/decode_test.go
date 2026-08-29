package catalog

import (
	"fmt"
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

func TestDecodeAcceptsTypedPins(t *testing.T) {
	got, err := Decode([]byte(`catalogVersion: 1
kind: Host
pins:
  npm:
    provider: npm:provider@1.2.3
  pacmanArtifacts:
    example:
      package: example-package
      version: 1.2.3-1
      source: repository
      sha256: not-a-digest
  aurLocal:
    driver:
      sourceCommit: not-a-commit
      patchSHA256: not-a-digest
  remoteArtifacts:
    tool:
      url: http://downloads.example.invalid/tool.tar.zst
      sha256: not-a-digest
  localPathPackages:
    gentle-pi: not-a-resolved-path
`))
	if err != nil {
		t.Fatalf("decode pins: %v", err)
	}
	if got.Pins == nil {
		t.Fatal("typed catalog lost pins")
	}
	if got.Pins.NPM["provider"] != "npm:provider@1.2.3" {
		t.Fatalf("typed npm pins = %#v", got.Pins.NPM)
	}
	if got.Pins.PacmanArtifacts["example"] != (PacmanArtifactPin{
		Package: "example-package",
		Version: "1.2.3-1",
		Source:  PacmanArtifactRepository,
		SHA256:  "not-a-digest",
	}) {
		t.Fatalf("typed pacman pins = %#v", got.Pins.PacmanArtifacts)
	}
	if got.Pins.AURLocal["driver"] != (AURLocalPin{SourceCommit: "not-a-commit", PatchSHA256: "not-a-digest"}) {
		t.Fatalf("typed AUR pins = %#v", got.Pins.AURLocal)
	}
	if got.Pins.RemoteArtifacts["tool"] != (RemoteArtifactPin{URL: "http://downloads.example.invalid/tool.tar.zst", SHA256: "not-a-digest"}) {
		t.Fatalf("typed remote pins = %#v", got.Pins.RemoteArtifacts)
	}
	if got.Pins.LocalPathPackages["gentle-pi"] != "not-a-resolved-path" {
		t.Fatalf("typed local path pins = %#v", got.Pins.LocalPathPackages)
	}
}

func TestDecodePinsPreservesPresenceAndCopiesMaps(t *testing.T) {
	minimal := []byte("catalogVersion: 1\nkind: Global\n")
	omittedDocument, err := DecodeDocument(minimal)
	if err != nil {
		t.Fatal(err)
	}
	if omittedDocument.Pins != nil {
		t.Fatal("omitted pins retained document presence")
	}
	omittedCatalog, err := Decode(minimal)
	if err != nil {
		t.Fatal(err)
	}
	if omittedCatalog.Pins != nil {
		t.Fatal("omitted pins retained catalog presence")
	}

	emptyDocument, err := DecodeDocument([]byte("catalogVersion: 1\nkind: Global\npins: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if emptyDocument.Pins == nil {
		t.Fatal("explicit empty pins lost document presence")
	}
	if emptyDocument.Pins.NPM != nil || emptyDocument.Pins.PacmanArtifacts != nil || emptyDocument.Pins.AURLocal != nil || emptyDocument.Pins.RemoteArtifacts != nil || emptyDocument.Pins.LocalPathPackages != nil {
		t.Fatalf("empty pins unexpectedly populated sources: %#v", emptyDocument.Pins)
	}
	emptyCatalog, err := Decode([]byte("catalogVersion: 1\nkind: Global\npins: {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if emptyCatalog.Pins == nil {
		t.Fatal("explicit empty pins lost catalog presence")
	}

	sources, err := DecodeDocument([]byte(`catalogVersion: 1
kind: Global
pins:
  npm: {}
  pacmanArtifacts: {}
  aurLocal: {}
  remoteArtifacts: {}
  localPathPackages: {}
`))
	if err != nil {
		t.Fatal(err)
	}
	if sources.Pins == nil || sources.Pins.NPM == nil || sources.Pins.PacmanArtifacts == nil || sources.Pins.AURLocal == nil || sources.Pins.RemoteArtifacts == nil || sources.Pins.LocalPathPackages == nil {
		t.Fatalf("explicit empty source maps lost presence: %#v", sources.Pins)
	}
	typed, err := catalogFromDocument(sources)
	if err != nil {
		t.Fatal(err)
	}
	if typed.Pins == nil || typed.Pins.NPM == nil || typed.Pins.PacmanArtifacts == nil || typed.Pins.AURLocal == nil || typed.Pins.RemoteArtifacts == nil || typed.Pins.LocalPathPackages == nil {
		t.Fatalf("typed empty source maps lost presence: %#v", typed.Pins)
	}

	data := []byte(`catalogVersion: 1
kind: Global
pins:
  npm:
    provider: npm:provider@1.2.3
  localPathPackages:
    gentle-pi: not-a-resolved-path
`)
	document, err := DecodeDocument(data)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := catalogFromDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	(*document.Pins.NPM)["provider"] = "changed"
	delete(*document.Pins.LocalPathPackages, "gentle-pi")
	if catalog.Pins.NPM["provider"] != "npm:provider@1.2.3" || catalog.Pins.LocalPathPackages["gentle-pi"] != "not-a-resolved-path" {
		t.Fatalf("typed pins alias structural maps: %#v", catalog.Pins)
	}
}

func TestDecodeRejectsMissingArtifactFieldsWithStablePaths(t *testing.T) {
	tests := []struct {
		name   string
		source string
		fields string
		path   string
	}{
		{
			name:   "pacman package",
			source: "pacmanArtifacts",
			fields: "      version: 1.2.3-1\n      source: cache\n      sha256: digest\n",
			path:   "/pins/pacmanArtifacts/example/package",
		},
		{
			name:   "pacman version",
			source: "pacmanArtifacts",
			fields: "      package: example-package\n      source: cache\n      sha256: digest\n",
			path:   "/pins/pacmanArtifacts/example/version",
		},
		{
			name:   "pacman source",
			source: "pacmanArtifacts",
			fields: "      package: example-package\n      version: 1.2.3-1\n      sha256: digest\n",
			path:   "/pins/pacmanArtifacts/example/source",
		},
		{
			name:   "pacman sha256",
			source: "pacmanArtifacts",
			fields: "      package: example-package\n      version: 1.2.3-1\n      source: cache\n",
			path:   "/pins/pacmanArtifacts/example/sha256",
		},
		{
			name:   "aur source commit",
			source: "aurLocal",
			fields: "      patchSHA256: digest\n",
			path:   "/pins/aurLocal/example/sourceCommit",
		},
		{
			name:   "aur patch sha256",
			source: "aurLocal",
			fields: "      sourceCommit: commit\n",
			path:   "/pins/aurLocal/example/patchSHA256",
		},
		{
			name:   "remote url",
			source: "remoteArtifacts",
			fields: "      sha256: digest\n",
			path:   "/pins/remoteArtifacts/example/url",
		},
		{
			name:   "remote sha256",
			source: "remoteArtifacts",
			fields: "      url: http://example.invalid/tool\n",
			path:   "/pins/remoteArtifacts/example/sha256",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := fmt.Sprintf("catalogVersion: 1\nkind: Global\npins:\n  %s:\n    example:\n%s", test.source, test.fields)
			if _, err := Decode([]byte(data)); err == nil {
				t.Fatal("missing pin field was accepted")
			} else if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name %q", err, test.path)
			}
		})
	}
}
