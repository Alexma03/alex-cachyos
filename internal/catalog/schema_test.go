package catalog

import (
	"strings"
	"testing"
)

func TestSchemaValidatorAcceptsMinimalCatalog(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := validator.ValidateJSON([]byte(`{"catalogVersion":1,"kind":"Global"}`)); err != nil {
		t.Fatalf("minimal catalog rejected: %v", err)
	}
}

func TestSchemaValidatorAcceptsCatalogBoundaryShapes(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	const document = `{
		"catalogVersion": 1,
		"kind": "Host",
		"modules": {"bootstrap": true, "verify": false},
		"templates": ["templates/niri/config.kdl"],
		"overlays": ["overlays/galaxy"],
		"checkoutPins": {
			"gentle-ai": {
				"remote": "https://example.invalid/gentle-ai.git",
				"branch": "main",
				"commit": "0123456789abcdef"
			}
		}
	}`
	if err := validator.ValidateJSON([]byte(document)); err != nil {
		t.Fatalf("boundary shapes rejected: %v", err)
	}
}

func TestSchemaValidatorAcceptsSourceSpecificPinShapes(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	const document = `{
		"catalogVersion": 1,
		"kind": "Host",
		"checkoutPins": {
			"gentle-ai": {
				"remote": "https://example.invalid/gentle-ai.git",
				"branch": "main",
				"commit": "0123456789abcdef0123456789abcdef01234567"
			}
		},
		"pins": {
			"npm": {
				"provider": "npm:@scope/provider@1.2.3"
			},
			"pacmanArtifacts": {
				"example": {
					"package": "example-package",
					"version": "1:2.3.4-1",
					"source": "repository",
					"sha256": "not-a-digest"
				}
			},
			"aurLocal": {
				"driver": {
					"sourceCommit": "not-a-commit",
					"patchSHA256": "not-a-digest"
				}
			},
			"remoteArtifacts": {
				"tool": {
					"url": "http://downloads.example.invalid/tool.tar.zst",
					"sha256": "not-a-digest"
				}
			},
			"localPathPackages": {
				"gentle-pi": "not-a-resolved-path"
			}
		}
	}`
	if err := validator.ValidateJSON([]byte(document)); err != nil {
		t.Fatalf("source-specific pin shapes rejected: %v", err)
	}
}

func TestSchemaValidatorRejectsUnknownPinFields(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{
			name: "unknown pin source",
			doc:  `{"catalogVersion":1,"kind":"Global","pins":{"checkoutPins":{}}}`,
			path: "/pins/checkoutPins",
		},
		{
			name: "unknown pacman artifact field",
			doc:  `{"catalogVersion":1,"kind":"Global","pins":{"pacmanArtifacts":{"example":{"package":"example","version":"1","source":"cache","sha256":"digest","unexpected":true}}}}`,
			path: "/pins/pacmanArtifacts/example/unexpected",
		},
		{
			name: "unknown AUR local field",
			doc:  `{"catalogVersion":1,"kind":"Global","pins":{"aurLocal":{"driver":{"sourceCommit":"commit","patchSHA256":"digest","unexpected":true}}}}`,
			path: "/pins/aurLocal/driver/unexpected",
		},
		{
			name: "unknown remote artifact field",
			doc:  `{"catalogVersion":1,"kind":"Global","pins":{"remoteArtifacts":{"tool":{"url":"https://example.invalid/tool","sha256":"digest","unexpected":true}}}}`,
			path: "/pins/remoteArtifacts/tool/unexpected",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validator.ValidateJSON([]byte(test.doc))
			if err == nil {
				t.Fatal("catalog with unknown pin field accepted")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name path %q", err, test.path)
			}
		})
	}
}

func TestSchemaValidatorRejectsInvalidCatalogWithPaths(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		doc  string
		path string
	}{
		{
			name: "unsupported version",
			doc:  `{"catalogVersion":2,"kind":"Global"}`,
			path: "/catalogVersion",
		},
		{
			name: "unknown top-level field",
			doc:  `{"catalogVersion":1,"kind":"Global","unexpected":true}`,
			path: "/unexpected",
		},
		{
			name: "bad kind",
			doc:  `{"catalogVersion":1,"kind":"Profile"}`,
			path: "/kind",
		},
		{
			name: "malformed checkout pin",
			doc:  `{"catalogVersion":1,"kind":"Global","checkoutPins":{"gentle-ai":{"remote":"","branch":"main","commit":"abc"}}}`,
			path: "/checkoutPins/gentle-ai/remote",
		},
		{
			name: "modules wrong type",
			doc:  `{"catalogVersion":1,"kind":"Global","modules":[]}`,
			path: "/modules",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validator.ValidateJSON([]byte(test.doc))
			if err == nil {
				t.Fatal("invalid catalog accepted")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("error %q does not name path %q", err, test.path)
			}
		})
	}
}
