package pi

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestRenderPersonaAndBackgroundMatchFixtureAuthority(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	rendered, err := RenderIdentity(home, fixture.Persona, fixture.Background, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Persona.Path != filepath.Join(home, ".pi", "gentle-ai", "persona.json") || rendered.Background.Path != filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json") {
		t.Fatalf("identity paths = %#v", rendered)
	}
	if want := []byte("{\"mode\":\"fixture-neutral\"}\n"); !bytes.Equal(rendered.Persona.Content, want) {
		t.Fatalf("persona = %q, want %q", rendered.Persona.Content, want)
	}
	if want := []byte("{\"policy\":\"fixture-on\",\"schema\":\"fixture.background-subagents/v1\"}\n"); !bytes.Equal(rendered.Background.Content, want) {
		t.Fatalf("background = %q, want %q", rendered.Background.Content, want)
	}
	if rendered.Persona.DesiredSHA256 != digestHex(rendered.Persona.Content) || rendered.Background.DesiredSHA256 != digestHex(rendered.Background.Content) {
		t.Fatalf("identity digests do not bind canonical bytes: %#v", rendered)
	}
}

func TestRenderIdentityRejectsUnknownTokensAndMultilineValues(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	tests := []struct {
		persona    PersonaInput
		background BackgroundInput
	}{
		{PersonaInput{Mode: "@HOST@"}, BackgroundInput{Schema: "fixture/v1", Policy: "on"}},
		{PersonaInput{Mode: "fixture\nmode"}, BackgroundInput{Schema: "fixture/v1", Policy: "on"}},
		{PersonaInput{Mode: "fixture"}, BackgroundInput{Schema: "fixture/v1", Policy: ""}},
	}
	for _, test := range tests {
		if _, err := RenderIdentity(home, test.persona, test.background, TokenContext{Home: home, User: "fixture"}); err == nil {
			t.Fatalf("forbidden identity values were accepted: %#v", test)
		}
	}
}
