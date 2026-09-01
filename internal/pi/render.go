package pi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"alex-cachyos/internal/catalog"
)

var (
	ErrRenderAuthority = errors.New("invalid Pi render authority")
	unresolvedToken    = regexp.MustCompile(`@[A-Z][A-Z0-9_]*@`)
)

type TokenContext struct {
	Home string
	User string
}

type SettingsInput struct {
	DefaultModel            string `json:"defaultModel"`
	DefaultProvider         string `json:"defaultProvider"`
	DefaultThinkingLevel    string `json:"defaultThinkingLevel"`
	Theme                   string `json:"theme"`
	ResolvedChangelogMarker string `json:"resolvedChangelogMarker"`
}

type RenderedFile struct {
	Path          string
	Content       []byte
	DesiredSHA256 string
}

type settingsDocument struct {
	DefaultModel         string   `json:"defaultModel"`
	DefaultProvider      string   `json:"defaultProvider"`
	DefaultThinkingLevel string   `json:"defaultThinkingLevel"`
	LastChangelogVersion string   `json:"lastChangelogVersion"`
	Packages             []string `json:"packages"`
	Theme                string   `json:"theme"`
}

type desiredSettingsDocument struct {
	DefaultModel         string   `json:"defaultModel"`
	DefaultProvider      string   `json:"defaultProvider"`
	DefaultThinkingLevel string   `json:"defaultThinkingLevel"`
	Packages             []string `json:"packages"`
	Theme                string   `json:"theme"`
}

func RenderSettings(home string, input SettingsInput, packages PackagePlan, tokens TokenContext) (RenderedFile, error) {
	if !canonicalAbsolute(home) || packages.HomeRoot != home || !validPackagePlan(packages) {
		return RenderedFile{}, renderAuthority("settings home does not match package authority")
	}
	resolved, err := resolveSettingsInput(input, tokens)
	if err != nil {
		return RenderedFile{}, err
	}
	if err := catalog.ValidateNpmPin("pi-runtime", catalog.NpmPin{Name: "pi-runtime", Version: resolved.ResolvedChangelogMarker}); err != nil {
		return RenderedFile{}, renderAuthority("resolved changelog marker is not an exact runtime version")
	}
	specs := make([]string, len(packages.Desired))
	for i, item := range packages.Desired {
		specs[i] = item.Spec
	}
	document := settingsDocument{
		DefaultModel: resolved.DefaultModel, DefaultProvider: resolved.DefaultProvider,
		DefaultThinkingLevel: resolved.DefaultThinkingLevel, LastChangelogVersion: resolved.ResolvedChangelogMarker,
		Packages: specs, Theme: resolved.Theme,
	}
	content, err := canonicalJSON(document)
	if err != nil {
		return RenderedFile{}, err
	}
	desired, err := canonicalJSON(desiredSettingsDocument{
		DefaultModel: document.DefaultModel, DefaultProvider: document.DefaultProvider,
		DefaultThinkingLevel: document.DefaultThinkingLevel, Packages: append([]string(nil), document.Packages...), Theme: document.Theme,
	})
	if err != nil {
		return RenderedFile{}, err
	}
	return RenderedFile{Path: filepath.Join(home, ".pi", "agent", "settings.json"), Content: content, DesiredSHA256: digestHex(desired)}, nil
}

type SettingsCheckReport struct {
	Drift  bool
	Reason string
}

func CheckSettings(expected RenderedFile, actual []byte) SettingsCheckReport {
	var want, got settingsDocument
	if err := decodeStrictJSON(expected.Content, &want); err != nil {
		return SettingsCheckReport{Drift: true, Reason: "expected-settings-invalid"}
	}
	if err := decodeStrictJSON(actual, &got); err != nil {
		return SettingsCheckReport{Drift: true, Reason: "settings-shape-drift"}
	}
	if err := catalog.ValidateNpmPin("pi-runtime", catalog.NpmPin{Name: "pi-runtime", Version: got.LastChangelogVersion}); err != nil {
		return SettingsCheckReport{Drift: true, Reason: "changelog-marker-invalid"}
	}
	wantDesired := desiredSettingsDocument{DefaultModel: want.DefaultModel, DefaultProvider: want.DefaultProvider, DefaultThinkingLevel: want.DefaultThinkingLevel, Packages: want.Packages, Theme: want.Theme}
	gotDesired := desiredSettingsDocument{DefaultModel: got.DefaultModel, DefaultProvider: got.DefaultProvider, DefaultThinkingLevel: got.DefaultThinkingLevel, Packages: got.Packages, Theme: got.Theme}
	if !reflect.DeepEqual(wantDesired, gotDesired) || digestHex(mustCanonicalJSON(gotDesired)) != expected.DesiredSHA256 {
		return SettingsCheckReport{Drift: true, Reason: "settings-desired-state-drift"}
	}
	canonicalActual, err := canonicalJSON(got)
	if err != nil || !bytes.Equal(canonicalActual, actual) {
		return SettingsCheckReport{Drift: true, Reason: "settings-encoding-drift"}
	}
	return SettingsCheckReport{}
}

func resolveSettingsInput(input SettingsInput, tokens TokenContext) (SettingsInput, error) {
	resolved := input
	fields := []*string{&resolved.DefaultModel, &resolved.DefaultProvider, &resolved.DefaultThinkingLevel, &resolved.Theme, &resolved.ResolvedChangelogMarker}
	for _, field := range fields {
		value, err := resolveTokens(*field, tokens)
		if err != nil {
			return SettingsInput{}, err
		}
		*field = value
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
			return SettingsInput{}, renderAuthority("settings values must be non-empty single-line strings")
		}
	}
	return resolved, nil
}

func resolveTokens(value string, tokens TokenContext) (string, error) {
	if !canonicalAbsolute(tokens.Home) || tokens.User == "" || strings.TrimSpace(tokens.User) != tokens.User || strings.ContainsAny(tokens.User, " /\\\x00\r\n\t") {
		return "", renderAuthority("HOME and USER token values are invalid")
	}
	resolved := strings.ReplaceAll(value, "@HOME@", tokens.Home)
	resolved = strings.ReplaceAll(resolved, "@USER@", tokens.User)
	if unresolvedToken.MatchString(resolved) {
		return "", renderAuthority("render contains an unknown token")
	}
	return resolved, nil
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("render canonical JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func mustCanonicalJSON(value any) []byte {
	data, err := canonicalJSON(value)
	if err != nil {
		panic(err)
	}
	return data
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JSON contains trailing values")
	}
	return nil
}

func digestHex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func renderAuthority(message string) error { return fmt.Errorf("%w: %s", ErrRenderAuthority, message) }
