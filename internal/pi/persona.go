package pi

import (
	"path/filepath"
	"strings"
)

type PersonaInput struct {
	Mode string `json:"mode"`
}

type BackgroundInput struct {
	Schema string `json:"schema"`
	Policy string `json:"policy"`
}

type IdentityFiles struct {
	Persona    RenderedFile
	Background RenderedFile
}

type personaDocument struct {
	Mode string `json:"mode"`
}

type backgroundDocument struct {
	Policy string `json:"policy"`
	Schema string `json:"schema"`
}

func RenderIdentity(home string, persona PersonaInput, background BackgroundInput, tokens TokenContext) (IdentityFiles, error) {
	if !canonicalAbsolute(home) || tokens.Home != home {
		return IdentityFiles{}, renderAuthority("identity home does not match token authority")
	}
	mode, err := resolveTokens(persona.Mode, tokens)
	if err != nil {
		return IdentityFiles{}, err
	}
	policy, err := resolveTokens(background.Policy, tokens)
	if err != nil {
		return IdentityFiles{}, err
	}
	schema, err := resolveTokens(background.Schema, tokens)
	if err != nil {
		return IdentityFiles{}, err
	}
	for _, value := range []string{mode, policy, schema} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
			return IdentityFiles{}, renderAuthority("persona and background values must be non-empty single-line strings")
		}
	}
	personaBytes, err := canonicalJSON(personaDocument{Mode: mode})
	if err != nil {
		return IdentityFiles{}, err
	}
	backgroundBytes, err := canonicalJSON(backgroundDocument{Policy: policy, Schema: schema})
	if err != nil {
		return IdentityFiles{}, err
	}
	return IdentityFiles{
		Persona:    RenderedFile{Path: filepath.Join(home, ".pi", "gentle-ai", "persona.json"), Content: personaBytes, DesiredSHA256: digestHex(personaBytes)},
		Background: RenderedFile{Path: filepath.Join(home, ".pi", "gentle-ai", "background-subagents.json"), Content: backgroundBytes, DesiredSHA256: digestHex(backgroundBytes)},
	}, nil
}
