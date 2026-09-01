package pi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type renderFixture struct {
	Settings   SettingsInput   `json:"settings"`
	Routes     []Route         `json:"routes"`
	Persona    PersonaInput    `json:"persona"`
	Background BackgroundInput `json:"background"`
}

func TestRenderSettingsIsMinimalCanonicalAndRuntimeMarkerIsNotDesiredState(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	plan := fixturePackagePlan(t, home)
	first, err := RenderSettings(home, fixture.Settings, plan, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderSettings(home, fixture.Settings, plan, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != filepath.Join(home, ".pi", "agent", "settings.json") || !bytes.Equal(first.Content, second.Content) || first.Content[len(first.Content)-1] != '\n' || bytes.Count(first.Content, []byte("\n")) != 1 {
		t.Fatalf("settings render is not canonical: %#v / %q", first, first.Content)
	}
	if len(first.DesiredSHA256) != 64 || CheckSettings(first, first.Content).Drift {
		t.Fatalf("settings desired digest/check is invalid: %#v", first)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(first.Content, &document); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	wantKeys := []string{"defaultModel", "defaultProvider", "defaultThinkingLevel", "lastChangelogVersion", "packages", "theme"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("settings keys = %#v, want %#v", keys, wantKeys)
	}
	for _, forbidden := range []string{"session_resources", "default_mode", "enable_continue", "debug", "model_profiles"} {
		if _, ok := document[forbidden]; ok {
			t.Fatalf("forbidden settings key %q rendered", forbidden)
		}
	}

	changed := fixture.Settings
	changed.ResolvedChangelogMarker = "0.0.0-fixture.11"
	withNewMarker, err := RenderSettings(home, changed, plan, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if first.DesiredSHA256 != withNewMarker.DesiredSHA256 || bytes.Equal(first.Content, withNewMarker.Content) {
		t.Fatalf("runtime marker changed desired state: first=%#v second=%#v", first, withNewMarker)
	}
	if report := CheckSettings(first, withNewMarker.Content); report.Drift {
		t.Fatalf("runtime marker-only change reported drift: %#v", report)
	}
}

func TestRenderSettingsResolvesOnlyHomeAndUserTokensBeforePublication(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	plan := fixturePackagePlan(t, home)
	fixture.Settings.Theme = "@HOME@/theme-@USER@"
	rendered, err := RenderSettings(home, fixture.Settings, plan, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rendered.Content, []byte("@HOME@")) || bytes.Contains(rendered.Content, []byte("@USER@")) || !bytes.Contains(rendered.Content, []byte(home+"/theme-fixture")) {
		t.Fatalf("tokens were not resolved exactly: %s", rendered.Content)
	}
	fixture.Settings.Theme = "@HOST@"
	if _, err := RenderSettings(home, fixture.Settings, plan, TokenContext{Home: home, User: "fixture"}); err == nil {
		t.Fatal("unknown token was accepted")
	}
}

func TestRenderSettingsRejectsForgedPackagePlan(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	plan := fixturePackagePlan(t, home)
	plan.Desired[0].Spec = "npm:fixture@latest"
	if _, err := RenderSettings(home, fixture.Settings, plan, TokenContext{Home: home, User: "fixture"}); err == nil {
		t.Fatal("forged package plan was accepted")
	}
}

func loadRenderFixture(t *testing.T) renderFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "render-authority.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture renderFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func fixturePackagePlan(t *testing.T, home string) PackagePlan {
	t.Helper()
	fixture := loadPackageCatalog(t)
	plan, err := BuildPackagePlan(PackagePlanInput{
		Pins: fixture.Pins, SettingsPath: filepath.Join(home, ".pi", "agent", "settings.json"), IntendedLayout: LayoutAgent,
		Checkout: CheckoutReference{Name: "gentle-pi", Path: filepath.Join(home, "Projects", "gentle-pi"), ReadOnly: true, Pin: fixture.CheckoutPins["gentle-pi"]},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
