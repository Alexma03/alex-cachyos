package pi

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestRenderRoutesUsesOneExact23NameSourceForBothFiles(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	rendered, err := RenderRoutes(home, fixture.Routes, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Subagents.Path != filepath.Join(home, ".pi", "agent", "subagents.json") || rendered.Models.Path != filepath.Join(home, ".pi", "gentle-ai", "models.json") {
		t.Fatalf("route paths = %#v", rendered)
	}
	if rendered.Subagents.DesiredSHA256 != digestHex(rendered.Subagents.Content) || rendered.Models.DesiredSHA256 != digestHex(rendered.Models.Content) {
		t.Fatalf("route digests do not bind canonical bytes: %#v", rendered)
	}
	if bytes.Count(rendered.Subagents.Content, []byte("\n")) != 1 || bytes.Count(rendered.Models.Content, []byte("\n")) != 1 {
		t.Fatalf("route files are not canonical one-line JSON: %q / %q", rendered.Subagents.Content, rendered.Models.Content)
	}
	var subagents struct {
		SessionResources string `json:"session_resources"`
		DefaultMode      string `json:"default_mode"`
		EnableContinue   bool   `json:"enable_continue"`
		Debug            bool   `json:"debug"`
		ModelProfiles    map[string]struct {
			Model  string `json:"model"`
			Effort string `json:"effort"`
		} `json:"model_profiles"`
	}
	var models map[string]struct {
		Model    string `json:"model"`
		Thinking string `json:"thinking"`
	}
	if err := json.Unmarshal(rendered.Subagents.Content, &subagents); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(rendered.Models.Content, &models); err != nil {
		t.Fatal(err)
	}
	if subagents.SessionResources != "lean" || subagents.DefaultMode != "task" || subagents.EnableContinue || subagents.Debug {
		t.Fatalf("subagent behavior = %#v", subagents)
	}
	wantNames := RequiredRouteNames()
	if got := sortedRouteMapNames(subagents.ModelProfiles); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("subagent routes = %#v, want %#v", got, wantNames)
	}
	if got := sortedRouteMapNames(models); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("model routes = %#v, want %#v", got, wantNames)
	}
	for _, name := range wantNames {
		if subagents.ModelProfiles[name].Model != models[name].Model || subagents.ModelProfiles[name].Effort != models[name].Thinking {
			t.Fatalf("route %q diverged: %#v / %#v", name, subagents.ModelProfiles[name], models[name])
		}
	}
	reversed := append([]Route(nil), fixture.Routes...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	again, err := RenderRoutes(home, reversed, TokenContext{Home: home, User: "fixture"})
	if err != nil || !bytes.Equal(rendered.Subagents.Content, again.Subagents.Content) || !bytes.Equal(rendered.Models.Content, again.Models.Content) {
		t.Fatalf("route order changed canonical output: err=%v", err)
	}
}

func TestRenderRoutesRejectsIncompleteDuplicateOrInvalidAuthority(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	tests := []struct {
		name   string
		mutate func([]Route) []Route
	}{
		{"missing route", func(routes []Route) []Route { return routes[:len(routes)-1] }},
		{"duplicate route", func(routes []Route) []Route { return append(routes, routes[0]) }},
		{"unknown route", func(routes []Route) []Route { routes[0].Name = "fixture-route"; return routes }},
		{"empty model", func(routes []Route) []Route { routes[0].Model = ""; return routes }},
		{"invalid level", func(routes []Route) []Route { routes[0].Level = "fixture"; return routes }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			routes := append([]Route(nil), fixture.Routes...)
			routes = test.mutate(routes)
			if _, err := RenderRoutes(home, routes, TokenContext{Home: home, User: "fixture"}); err == nil {
				t.Fatal("invalid route authority was accepted")
			}
		})
	}
}

func TestCheckRouteDriftNamesTheEditedFileRouteAndExpectedLevel(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	rendered, err := RenderRoutes(home, fixture.Routes, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	var models map[string]map[string]string
	if err := json.Unmarshal(rendered.Models.Content, &models); err != nil {
		t.Fatal(err)
	}
	route := "sdd-apply"
	wantLevel := models[route]["thinking"]
	models[route]["thinking"] = "max"
	if wantLevel == "max" {
		models[route]["thinking"] = "high"
	}
	edited, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	edited = append(edited, '\n')
	report := CheckRouteDrift(rendered, rendered.Subagents.Content, edited)
	if !report.Drift {
		t.Fatal("hand-edited route level was not reported")
	}
	var found bool
	for _, drift := range report.Entries {
		if drift.File == rendered.Models.Path && drift.Route == route && drift.Field == "thinking" && drift.Expected == wantLevel {
			found = true
		}
	}
	if !found {
		t.Fatalf("route-specific drift missing: %#v", report)
	}
}

func TestCheckRouteDriftRequiresExplicitFalseFlags(t *testing.T) {
	fixture := loadRenderFixture(t)
	home := filepath.Join(t.TempDir(), "home")
	rendered, err := RenderRoutes(home, fixture.Routes, TokenContext{Home: home, User: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	withoutDebug := bytes.Replace(rendered.Subagents.Content, []byte(`"debug":false,`), nil, 1)
	if bytes.Equal(withoutDebug, rendered.Subagents.Content) {
		t.Fatal("fixture did not contain an explicit debug field")
	}
	report := CheckRouteDrift(rendered, withoutDebug, rendered.Models.Content)
	if !report.Drift {
		t.Fatal("omitted explicit false flag was accepted as equivalent")
	}
	for _, drift := range report.Entries {
		if drift.File == rendered.Subagents.Path && drift.Field == "debug" {
			return
		}
	}
	t.Fatalf("omitted debug field was not identified: %#v", report)
}

func sortedRouteMapNames[T any](values map[string]T) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
