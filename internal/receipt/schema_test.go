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

func receiptWithMutation(t *testing.T, mutate func(map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(goldenReceipt(t), &doc); err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAllowsBiometricIdentifiersInStructuredFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "step module and id",
			mutate: func(doc map[string]any) {
				steps := doc["steps"].([]any)
				step := steps[0].(map[string]any)
				step["module"] = "fingerprint"
				step["id"] = "fingerprint.install"
			},
		},
		{
			name: "package values",
			mutate: func(doc map[string]any) {
				packages := doc["desiredPackages"].(map[string]any)
				packages["pacmanNames"] = []any{"foot", "fprintd", "libfprint-egismoc-sdcp-git"}
			},
		},
		{
			name: "package map key",
			mutate: func(doc map[string]any) {
				exact := doc["desiredExactPins"].(map[string]any)
				npm := exact["npm"].(map[string]any)
				npm["libfprint-egismoc-sdcp-git"] = "1.0.0"
			},
		},
		{
			name: "managed file path",
			mutate: func(doc map[string]any) {
				files := doc["managedFiles"].([]any)
				files[0].(map[string]any)["path"] = "/etc/fingerprint/config.kdl"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(receiptWithMutation(t, tc.mutate)); err != nil {
				t.Fatalf("identifier-like receipt value rejected: %v", err)
			}
		})
	}
}

func TestRejectsBiometricProseInWarningsAndErrors(t *testing.T) {
	cases := []struct {
		name, field, value string
	}{
		{name: "warning template content", field: "warnings", value: "fingerprint template <content>"},
		{name: "error enrollment prose", field: "errors", value: "biometric enrollment data"},
		{name: "warning device prose", field: "warnings", value: "fprint device output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := receiptWithMutation(t, func(doc map[string]any) {
				doc[tc.field] = []any{tc.value}
			})
			if err := Validate(data); err == nil {
				t.Fatal("biometric prose was accepted")
			}
		})
	}
}

func TestAllowsOnlyBiometricPresenceWarnings(t *testing.T) {
	cases := []struct {
		field, value string
	}{
		{field: "warnings", value: "fingerprint"},
		{field: "errors", value: "fprintd"},
		{field: "warnings", value: "  FPRINTD  "},
	}
	for _, tc := range cases {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			data := receiptWithMutation(t, func(doc map[string]any) {
				doc[tc.field] = []any{tc.value}
			})
			if err := Validate(data); err != nil {
				t.Fatalf("presence-only biometric identifier rejected: %v", err)
			}
		})
	}
}

func TestRejectsBiometricIdentifiersWithSecretMarkers(t *testing.T) {
	cases := []struct{ target, value string }{
		{"value", "fingerprintToken"}, {"value", "biometric_secret"}, {"value", "fingerprint-template-secret"},
		{"value", "fingerprint.api.key"}, {"value", "fingerprint/api:key"}, {"value", "Fingerprint API Key"},
		{"key", "fingerprintToken"}, {"key", "biometric.secret"}, {"key", "fingerprint/api:key"}, {"key", "FINGERPRINT API KEY"},
	}
	for _, tc := range cases {
		t.Run(tc.target+"/"+tc.value, func(t *testing.T) {
			data := receiptWithMutation(t, func(doc map[string]any) {
				if tc.target == "key" {
					npm := doc["desiredExactPins"].(map[string]any)["npm"].(map[string]any)
					npm[tc.value] = "1.0.0"
				} else {
					steps := doc["steps"].([]any)
					steps[0].(map[string]any)["id"] = tc.value
				}
			})
			if err := Validate(data); err == nil {
				t.Fatal("biometric identifier containing a secret marker was accepted")
			}
		})
	}
}

func TestRejectsBiometricWarningIdentifiersOutsideAllowlist(t *testing.T) {
	cases := []struct {
		field, value string
	}{
		{field: "warnings", value: "fingerprint.install"},
		{field: "errors", value: "fingerprint-template"},
		{field: "warnings", value: "biometric"},
		{field: "errors", value: "fprintd present"},
	}
	for _, tc := range cases {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			data := receiptWithMutation(t, func(doc map[string]any) {
				doc[tc.field] = []any{tc.value}
			})
			if err := Validate(data); err == nil {
				t.Fatal("biometric warning/error outside the explicit allowlist was accepted")
			}
		})
	}
}

func TestCredentialKeyScope(t *testing.T) {
	if err := Validate(goldenReceipt(t)); err != nil {
		t.Fatalf("root credentials section rejected: %v", err)
	}
	for _, key := range []string{"credentials", "credential:s", "referencedNames", "referenced.names"} {
		t.Run(key, func(t *testing.T) {
			data := receiptWithMutation(t, func(doc map[string]any) {
				npm := doc["desiredExactPins"].(map[string]any)["npm"].(map[string]any)
				npm[key] = "1.0.0"
			})
			if err := Validate(data); err == nil {
				t.Fatal("nested credential-like key was accepted")
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
