package receipt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func goldenReceipt(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "receipts", "golden-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGoldenV1(t *testing.T) {
	data := goldenReceipt(t)
	r, err := Decode(data)
	if err != nil {
		t.Fatalf("golden receipt rejected: %v", err)
	}
	if r.Schema != SchemaV1 || r.RunID == "" || r.Status == "" {
		t.Fatalf("receipt identity = %#v", r)
	}
	if len(r.Steps) == 0 || len(r.ManagedFiles) == 0 || len(r.Mutations) == 0 || len(r.Checkouts) == 0 {
		t.Fatal("golden receipt omitted required audit collections")
	}
	if len(r.DesiredPackages.PacmanNames) == 0 || len(r.DesiredExactPins.NPM) == 0 || len(r.ResolvedInstalledVersions.NPM) == 0 {
		t.Fatal("desired and resolved package evidence was not kept separate")
	}
	if len(r.Credentials.ReferencedNames) == 0 {
		t.Fatal("golden receipt omitted credential references")
	}
	if _, err := CanonicalJSON(r); err != nil {
		t.Fatalf("canonical receipt encoding failed: %v", err)
	}
}

func TestRejectsSensitiveReceiptContentWithoutEchoingIt(t *testing.T) {
	const secret = "WU7-secret-must-not-appear-in-errors"
	base := goldenReceipt(t)
	cases := []struct {
		name string
		edit func(map[string]json.RawMessage) error
	}{
		{
			name: "credential value",
			edit: func(doc map[string]json.RawMessage) error {
				doc["credentials"] = json.RawMessage(`{"referencedNames":["OPENAI_API_KEY"],"values":"` + secret + `"}`)
				return nil
			},
		},
		{
			name: "biometric content",
			edit: func(doc map[string]json.RawMessage) error {
				doc["warnings"] = json.RawMessage(`["fingerprint template ` + secret + `"]`)
				return nil
			},
		},
		{
			name: "dirty diff content",
			edit: func(doc map[string]json.RawMessage) error {
				doc["mutations"] = json.RawMessage(`[{"kind":"file","target":"/etc/example","before":"diff --git a/a b/b ` + secret + `","after":"","inverse":"restore","rollbackPrecondition":"after"}]`)
				return nil
			},
		},
		{
			name: "secret-bearing argv",
			edit: func(doc map[string]json.RawMessage) error {
				doc["argv"] = json.RawMessage(`["--token=` + secret + `"]`)
				return nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(base, &doc); err != nil {
				t.Fatal(err)
			}
			if err := tc.edit(doc); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			err = Validate(data)
			if err == nil {
				t.Fatal("sensitive receipt content was accepted")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("validation error echoed sensitive content: %v", err)
			}
		})
	}
}

func TestRejectsUnknownFieldsAndMissingSections(t *testing.T) {
	base := goldenReceipt(t)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(base, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "desiredPackages")
	if data, err := json.Marshal(doc); err != nil {
		t.Fatal(err)
	} else if err := Validate(data); err == nil {
		t.Fatal("receipt without desiredPackages was accepted")
	}

	if err := json.Unmarshal(base, &doc); err != nil {
		t.Fatal(err)
	}
	doc["unexpected"] = json.RawMessage(`true`)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(data); err == nil {
		t.Fatal("receipt with an unknown field was accepted")
	}
}
