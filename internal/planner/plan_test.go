package planner

import (
	"reflect"
	"testing"
)

func TestBuildPlanOrdersByModuleRankThenStepID(t *testing.T) {
	modules := []Module{
		{Name: "desktop", Enabled: true, DependsOn: []string{"apps"}, Steps: []Step{{ID: "desktop-config", Module: "desktop"}}},
		{Name: "verify", Enabled: true, DependsOn: []string{"desktop"}, Steps: []Step{{ID: "verify", Module: "verify"}}},
		{Name: "apps", Enabled: true, Steps: []Step{{ID: "apps-z", Module: "apps"}, {ID: "apps-a", Module: "apps"}}},
		{Name: "bootstrap", Enabled: true, Steps: []Step{{ID: "bootstrap-z", Module: "bootstrap"}, {ID: "bootstrap-a", Module: "bootstrap"}}},
	}
	plan, err := BuildPlan(modules, Selection{})
	if err != nil { t.Fatal(err) }
	got := make([]string, len(plan.Steps))
	for i, step := range plan.Steps { got[i] = step.ID }
	want := []string{"bootstrap-a", "bootstrap-z", "apps-a", "apps-z", "desktop-config", "verify"}
	if !reflect.DeepEqual(got, want) { t.Fatalf("step IDs = %v, want %v", got, want) }
}

func plannerModules() []Module {
	return []Module{
		{Name: "verify", Enabled: true, DependsOn: []string{"desktop"}, Steps: []Step{{ID: "verify"}}},
		{Name: "desktop", Enabled: true, DependsOn: []string{"apps"}, Steps: []Step{{ID: "desktop"}}},
		{Name: "apps", Enabled: true, DependsOn: []string{"bootstrap"}, Steps: []Step{{ID: "apps"}}},
		{Name: "bootstrap", Enabled: true, Steps: []Step{{ID: "bootstrap"}}},
		{Name: "vicinae", Enabled: true, Steps: []Step{{ID: "vicinae"}}},
		{Name: "optional", Enabled: false, DependsOn: []string{"bootstrap"}, Steps: []Step{{ID: "optional"}}},
	}
}

func TestBuildPlanSelectionPrecedenceAndDefaults(t *testing.T) {
	tests := []struct { name string; selection Selection; want []string }{
		{"only is exact", Selection{Only: []string{"desktop"}, With: []string{"optional"}, Without: []string{"desktop"}}, []string{"desktop"}},
		{"with enables and without removes", Selection{With: []string{"optional"}, Without: []string{"vicinae"}}, []string{"bootstrap", "apps", "desktop", "optional", "verify"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := BuildPlan(plannerModules(), test.selection)
			if err != nil { t.Fatal(err) }
			got := make([]string, len(plan.Steps))
			for i, step := range plan.Steps { got[i] = step.ID }
			if !reflect.DeepEqual(got, test.want) { t.Fatalf("step IDs = %v, want %v", got, test.want) }
		})
	}
}

func TestBuildPlanRejectsTypedErrors(t *testing.T) {
	tests := []struct {
		name string
		modules []Module
		selection Selection
		want any
	}{
		{"unknown selection", plannerModules(), Selection{Only: []string{"missing"}}, &UnknownSelectionError{}},
		{"duplicate modules", []Module{{Name: "a"}, {Name: "a"}}, Selection{}, &DuplicateModuleError{}},
		{"duplicate steps", []Module{{Name: "a", Steps: []Step{{ID: "same"}}}, {Name: "b", Steps: []Step{{ID: "same"}}}}, Selection{}, &DuplicateStepError{}},
		{"missing enabled dependency", []Module{{Name: "a", Enabled: true, DependsOn: []string{"b"}}, {Name: "b", Enabled: false}}, Selection{}, &MissingDependencyError{}},
		{"module cycle", []Module{{Name: "a", Enabled: true, DependsOn: []string{"b"}}, {Name: "b", Enabled: true, DependsOn: []string{"a"}}}, Selection{}, &CycleError{}},
		{"step cycle", []Module{{Name: "a", Enabled: true, Steps: []Step{{ID: "a", DependsOn: []string{"b"}}, {ID: "b", DependsOn: []string{"a"}}}}}, Selection{}, &CycleError{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildPlan(test.modules, test.selection)
			if err == nil || reflect.TypeOf(err) != reflect.TypeOf(test.want) { t.Fatalf("error = %v, want %T", err, test.want) }
		})
	}
}

func TestBuildPlanCopiesInputs(t *testing.T) {
	depends := []string{"prepare"}
	steps := []Step{{ID: "prepare", Module: "bootstrap"}, {ID: "apply", Module: "bootstrap", DependsOn: depends}}
	modules := []Module{{Name: "bootstrap", Enabled: true, Steps: steps}}
	selection := Selection{Only: []string{"bootstrap"}}
	plan, err := BuildPlan(modules, selection)
	if err != nil { t.Fatal(err) }
	modules[0].Steps[1].ID, depends[0], selection.Only[0] = "changed", "changed", "changed"
	if plan.Steps[1].ID != "apply" || plan.Steps[1].DependsOn[0] != "prepare" || plan.Selection.Only[0] != "bootstrap" {
		t.Fatalf("plan changed after input mutation: %#v", plan)
	}
}
