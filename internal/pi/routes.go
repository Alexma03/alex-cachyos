package pi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

var requiredRoutes = []string{
	"gentle-ai-explore", "gentle-ai-verify", "gentle-ai-worker",
	"jd-fix-agent", "jd-judge-a", "jd-judge-b",
	"review-readability", "review-reliability", "review-resilience", "review-risk",
	"sdd-apply", "sdd-archive", "sdd-design", "sdd-explore", "sdd-init", "sdd-onboard", "sdd-proposal", "sdd-research", "sdd-spec", "sdd-status", "sdd-sync", "sdd-tasks", "sdd-verify",
}

type RouteLevel string

const (
	RouteHigh  RouteLevel = "high"
	RouteXHigh RouteLevel = "xhigh"
	RouteMax   RouteLevel = "max"
)

type Route struct {
	Name  string     `json:"name"`
	Model string     `json:"model"`
	Level RouteLevel `json:"level"`
}

type subagentRoute struct {
	Model  string     `json:"model"`
	Effort RouteLevel `json:"effort"`
}

type modelRoute struct {
	Model    string     `json:"model"`
	Thinking RouteLevel `json:"thinking"`
}

type subagentsDocument struct {
	SessionResources string                   `json:"session_resources"`
	DefaultMode      string                   `json:"default_mode"`
	EnableContinue   bool                     `json:"enable_continue"`
	Debug            bool                     `json:"debug"`
	ModelProfiles    map[string]subagentRoute `json:"model_profiles"`
}

type RouteFiles struct {
	Subagents RenderedFile
	Models    RenderedFile
}

func RequiredRouteNames() []string { return append([]string(nil), requiredRoutes...) }

func RenderRoutes(home string, routes []Route, tokens TokenContext) (RouteFiles, error) {
	if !canonicalAbsolute(home) || tokens.Home != home {
		return RouteFiles{}, renderAuthority("route home does not match token authority")
	}
	resolved, err := validateAndResolveRoutes(routes, tokens)
	if err != nil {
		return RouteFiles{}, err
	}
	subagentProfiles := make(map[string]subagentRoute, len(resolved))
	models := make(map[string]modelRoute, len(resolved))
	for _, route := range resolved {
		subagentProfiles[route.Name] = subagentRoute{Model: route.Model, Effort: route.Level}
		models[route.Name] = modelRoute{Model: route.Model, Thinking: route.Level}
	}
	subagentsBytes, err := canonicalJSON(subagentsDocument{SessionResources: "lean", DefaultMode: "task", EnableContinue: false, Debug: false, ModelProfiles: subagentProfiles})
	if err != nil {
		return RouteFiles{}, err
	}
	modelsBytes, err := canonicalJSON(models)
	if err != nil {
		return RouteFiles{}, err
	}
	return RouteFiles{
		Subagents: RenderedFile{Path: filepath.Join(home, ".pi", "agent", "subagents.json"), Content: subagentsBytes, DesiredSHA256: digestHex(subagentsBytes)},
		Models:    RenderedFile{Path: filepath.Join(home, ".pi", "gentle-ai", "models.json"), Content: modelsBytes, DesiredSHA256: digestHex(modelsBytes)},
	}, nil
}

func validateAndResolveRoutes(routes []Route, tokens TokenContext) ([]Route, error) {
	if len(routes) != len(requiredRoutes) {
		return nil, renderAuthority("route inventory must contain exactly 23 entries")
	}
	index := make(map[string]Route, len(routes))
	for _, route := range routes {
		if _, duplicate := index[route.Name]; duplicate {
			return nil, renderAuthority("route inventory contains a duplicate")
		}
		model, err := resolveTokens(route.Model, tokens)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(model) == "" || strings.ContainsAny(model, "\x00\r\n") || !validRouteLevel(route.Level) {
			return nil, renderAuthority("route model or level is invalid")
		}
		route.Model = model
		index[route.Name] = route
	}
	result := make([]Route, 0, len(requiredRoutes))
	for _, name := range requiredRoutes {
		route, ok := index[name]
		if !ok {
			return nil, renderAuthority("route inventory is missing a required name")
		}
		result = append(result, route)
	}
	return result, nil
}

func validRouteLevel(level RouteLevel) bool {
	return level == RouteHigh || level == RouteXHigh || level == RouteMax
}

type RouteDrift struct {
	File     string
	Route    string
	Field    string
	Expected string
	Actual   string
}

type RouteCheckReport struct {
	Drift   bool
	Entries []RouteDrift
}

func CheckRouteDrift(expected RouteFiles, actualSubagents, actualModels []byte) RouteCheckReport {
	var wantSubagents, gotSubagents subagentsDocument
	var wantModels, gotModels map[string]modelRoute
	report := RouteCheckReport{}
	if err := decodeStrictJSON(expected.Subagents.Content, &wantSubagents); err != nil {
		addRouteDrift(&report, expected.Subagents.Path, "", "document", "valid expected JSON", "invalid")
		return report
	}
	if err := decodeStrictJSON(expected.Models.Content, &wantModels); err != nil {
		addRouteDrift(&report, expected.Models.Path, "", "document", "valid expected JSON", "invalid")
		return report
	}
	subagentsValid := decodeStrictJSON(actualSubagents, &gotSubagents) == nil
	modelsValid := decodeStrictJSON(actualModels, &gotModels) == nil
	if !subagentsValid {
		addRouteDrift(&report, expected.Subagents.Path, "", "document", "canonical rendered JSON", "invalid")
	} else {
		compareSubagentDocuments(&report, expected.Subagents.Path, wantSubagents, gotSubagents)
		for _, field := range missingSubagentFields(actualSubagents) {
			addRouteDrift(&report, expected.Subagents.Path, "", field, subagentExpectedValue(wantSubagents, field), "missing")
		}
		canonical, _ := canonicalJSON(gotSubagents)
		if !bytes.Equal(canonical, actualSubagents) && !routeFileHasDrift(report, expected.Subagents.Path) {
			addRouteDrift(&report, expected.Subagents.Path, "", "encoding", "canonical JSON", "noncanonical JSON")
		}
	}
	if !modelsValid {
		addRouteDrift(&report, expected.Models.Path, "", "document", "canonical rendered JSON", "invalid")
	} else {
		compareModelDocuments(&report, expected.Models.Path, wantModels, gotModels)
		canonical, _ := canonicalJSON(gotModels)
		if !bytes.Equal(canonical, actualModels) && !routeFileHasDrift(report, expected.Models.Path) {
			addRouteDrift(&report, expected.Models.Path, "", "encoding", "canonical JSON", "noncanonical JSON")
		}
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		left, right := report.Entries[i], report.Entries[j]
		return fmt.Sprintf("%s\x00%s\x00%s", left.File, left.Route, left.Field) < fmt.Sprintf("%s\x00%s\x00%s", right.File, right.Route, right.Field)
	})
	report.Drift = len(report.Entries) != 0
	return report
}

func missingSubagentFields(data []byte) []string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil
	}
	var missing []string
	for _, field := range []string{"session_resources", "default_mode", "enable_continue", "debug", "model_profiles"} {
		if _, ok := fields[field]; !ok {
			missing = append(missing, field)
		}
	}
	return missing
}

func subagentExpectedValue(document subagentsDocument, field string) string {
	switch field {
	case "session_resources":
		return document.SessionResources
	case "default_mode":
		return document.DefaultMode
	case "enable_continue":
		return fmt.Sprint(document.EnableContinue)
	case "debug":
		return fmt.Sprint(document.Debug)
	case "model_profiles":
		return "23 required routes"
	default:
		return "required"
	}
}

func compareSubagentDocuments(report *RouteCheckReport, file string, want, got subagentsDocument) {
	compareRouteValue(report, file, "", "session_resources", want.SessionResources, got.SessionResources)
	compareRouteValue(report, file, "", "default_mode", want.DefaultMode, got.DefaultMode)
	compareRouteValue(report, file, "", "enable_continue", fmt.Sprint(want.EnableContinue), fmt.Sprint(got.EnableContinue))
	compareRouteValue(report, file, "", "debug", fmt.Sprint(want.Debug), fmt.Sprint(got.Debug))
	for _, name := range unionRouteNames(want.ModelProfiles, got.ModelProfiles) {
		wantRoute, wantOK := want.ModelProfiles[name]
		gotRoute, gotOK := got.ModelProfiles[name]
		if !wantOK || !gotOK {
			compareRouteValue(report, file, name, "route", fmt.Sprint(wantOK), fmt.Sprint(gotOK))
			continue
		}
		compareRouteValue(report, file, name, "model", wantRoute.Model, gotRoute.Model)
		compareRouteValue(report, file, name, "effort", string(wantRoute.Effort), string(gotRoute.Effort))
	}
}

func compareModelDocuments(report *RouteCheckReport, file string, want, got map[string]modelRoute) {
	for _, name := range unionRouteNames(want, got) {
		wantRoute, wantOK := want[name]
		gotRoute, gotOK := got[name]
		if !wantOK || !gotOK {
			compareRouteValue(report, file, name, "route", fmt.Sprint(wantOK), fmt.Sprint(gotOK))
			continue
		}
		compareRouteValue(report, file, name, "model", wantRoute.Model, gotRoute.Model)
		compareRouteValue(report, file, name, "thinking", string(wantRoute.Thinking), string(gotRoute.Thinking))
	}
}

func compareRouteValue(report *RouteCheckReport, file, route, field, expected, actual string) {
	if expected != actual {
		addRouteDrift(report, file, route, field, expected, actual)
	}
}

func addRouteDrift(report *RouteCheckReport, file, route, field, expected, actual string) {
	report.Entries = append(report.Entries, RouteDrift{File: file, Route: route, Field: field, Expected: expected, Actual: actual})
	report.Drift = true
}

func routeFileHasDrift(report RouteCheckReport, file string) bool {
	for _, drift := range report.Entries {
		if drift.File == file {
			return true
		}
	}
	return false
}

func unionRouteNames[A, B any](left map[string]A, right map[string]B) []string {
	seen := make(map[string]bool, len(left)+len(right))
	for name := range left {
		seen[name] = true
	}
	for name := range right {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
