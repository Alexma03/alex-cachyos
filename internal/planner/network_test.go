package planner

import (
	"encoding/json"
	"testing"
)

func TestBuildPlanCopiesTypedStepMetadata(t *testing.T) {
	desired := json.RawMessage(`{"packages":["curl"]}`)
	observed := json.RawMessage(`{"installed":false}`)
	inverseValue := json.RawMessage(`{"packages":["curl"]}`)
	inverse := &InverseDescriptor{Operation: Operation("remove-packages"), Value: inverseValue}
	modules := []Module{{
		Name: "bootstrap", Enabled: true, Steps: []Step{{
			ID: "install-packages", Description: "run pacman -S curl", Scope: ScopeSystem,
			Network: NetworkRequired, Operation: Operation("install-packages"),
			Disposition: DispositionApply, Desired: desired, Observed: observed, Inverse: inverse,
		}},
	}}

	plan, err := BuildPlan(modules, Selection{})
	if err != nil {
		t.Fatal(err)
	}
	modules[0].Steps[0].Description = "changed"
	desired[2], observed[2], inverseValue[2] = 'X', 'X', 'X'
	inverse.Value[2] = 'X'
	modules[0].Steps[0].Inverse.Operation = Operation("changed")

	step := plan.Steps[0]
	if step.Description != "run pacman -S curl" || step.Scope != ScopeSystem || step.Network != NetworkRequired || step.Operation != Operation("install-packages") || step.Disposition != DispositionApply {
		t.Fatalf("plan metadata changed: %#v", step)
	}
	if string(step.Desired) != `{"packages":["curl"]}` || string(step.Observed) != `{"installed":false}` {
		t.Fatalf("plan values changed: desired=%s observed=%s", step.Desired, step.Observed)
	}
	if step.Inverse == nil || step.Inverse.Operation != Operation("remove-packages") || string(step.Inverse.Value) != `{"packages":["curl"]}` {
		t.Fatalf("plan inverse changed: %#v", step.Inverse)
	}
}

func TestNetworkGroupsUseExplicitClassificationAndPlanOrder(t *testing.T) {
	plan := Plan{Steps: []Step{
		{ID: "render-config", Description: "run curl to render config", Network: NetworkNone, Operation: Operation("render-config")},
		{ID: "fetch-icon", Description: "fetch https://icons.example/icon.svg", Network: NetworkRequired, Operation: Operation("fetch-icon")},
		{ID: "offline-template", Description: "pacman -S should not run", Network: NetworkNone, Operation: Operation("render-template")},
		{ID: "install-packages", Network: NetworkRequired, Operation: Operation("install-packages")},
		{ID: "fetch-icon-again", Network: NetworkRequired, Operation: Operation("fetch-icon")},
	}}

	groups := NetworkGroups(plan)
	if len(groups) != 2 {
		t.Fatalf("group count = %d, want 2: %#v", len(groups), groups)
	}
	if groups[0].Operation != Operation("fetch-icon") || len(groups[0].Steps) != 2 || groups[0].Steps[0].ID != "fetch-icon" || groups[0].Steps[1].ID != "fetch-icon-again" {
		t.Fatalf("icon group = %#v", groups[0])
	}
	if groups[1].Operation != Operation("install-packages") || len(groups[1].Steps) != 1 || groups[1].Steps[0].ID != "install-packages" {
		t.Fatalf("package group = %#v", groups[1])
	}
}
