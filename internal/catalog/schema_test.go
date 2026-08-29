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
