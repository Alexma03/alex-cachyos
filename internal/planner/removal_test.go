package planner

import (
	"errors"
	"testing"
)

func TestBuildRemovalPlanMapsSelectedStepsToInversesInReverseOrder(t *testing.T) {
	plan := Plan{Steps: []Step{
		{ID: "apps.install", Module: "apps", Operation: "install", Desired: []byte(`{"package":"app"}`), Inverse: &InverseDescriptor{Operation: "remove-package", Value: []byte(`{"package":"app"}`)}},
		{ID: "desktop.file", Module: "desktop", Operation: "publish", Desired: []byte(`{"path":"/etc/example"}`), Inverse: &InverseDescriptor{Operation: "remove-file", Value: []byte(`{"path":"/etc/example"}`)}},
		{ID: "desktop.validate", Module: "desktop", Operation: "validate", Inverse: &InverseDescriptor{Operation: "validate", Value: []byte(`{}`)}},
	}}
	got, err := BuildRemovalPlan(plan, []string{"desktop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 2 || got.Steps[0].ID != "desktop.validate" || got.Steps[1].ID != "desktop.file" || got.Steps[1].Operation != "remove-file" || got.Steps[1].Disposition != DispositionRemove {
		t.Fatalf("removal plan = %#v", got)
	}
	if _, err := BuildRemovalPlan(plan, []string{"missing"}); !errors.Is(err, ErrUnknownSelection) {
		t.Fatalf("unknown removal error = %v", err)
	}
	plan.Steps[1].Inverse = nil
	if _, err := BuildRemovalPlan(plan, []string{"desktop"}); !errors.Is(err, ErrMissingInverse) {
		t.Fatalf("missing inverse error = %v", err)
	}
}
