package planner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCanonicalizesPlan(t *testing.T) {
	plan := Plan{
		Selection: Selection{
			Only:    []string{"b", "a", "b"},
			With:    []string{"c"},
			Without: []string{"d"},
		},
		Steps: []Step{{
			ID:          "s1",
			Module:      "m1",
			Description: "do things",
			DependsOn:   []string{"s0"},
			Scope:       ScopeSystem,
			Network:     NetworkRequired,
			Operation:   Operation("install"),
			Disposition: DispositionApply,
			Desired:     json.RawMessage(`{"k2":2,"k1":1}`),
			Observed:    json.RawMessage(`{"ok":true}`),
			Inverse:     &InverseDescriptor{Operation: Operation("uninstall"), Value: json.RawMessage(`{"k2":2,"k1":1}`)},
		}},
	}

	const wantJSON = `{"selection":{"only":["a","b"],"with":["c"],"without":["d"]},"steps":[{"id":"s1","module":"m1","description":"do things","dependsOn":["s0"],"scope":"system","network":"required","operation":"install","desired":{"k1":1,"k2":2},"observed":{"ok":true},"disposition":"apply","inverse":{"operation":"uninstall","value":{"k1":1,"k2":2}}}]}`
	want := wantJSON + "\n"

	got, err := Normalize(plan)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if string(got) != want {
		t.Fatalf("Normalize = %q, want %q", got, want)
	}
	if !strings.HasSuffix(string(got), "\n") || strings.Count(string(got), "\n") != 1 {
		t.Fatalf("Normalize must end with exactly one newline: %q", got)
	}
}
