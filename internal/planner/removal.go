package planner

import (
	"errors"
	"fmt"
)

var ErrMissingInverse = errors.New("selected step has no inverse")

// BuildRemovalPlan maps selected module steps to their typed inverses in
// reverse application order. It never guesses an inverse for a step that did
// not declare one.
func BuildRemovalPlan(plan Plan, modules []string) (Plan, error) {
	selected := make(map[string]bool, len(modules))
	for _, module := range modules {
		if module == "" {
			return Plan{}, &UnknownSelectionError{Field: "remove", Name: module}
		}
		selected[module] = true
	}
	known := make(map[string]bool)
	for _, step := range plan.Steps {
		known[step.Module] = true
	}
	for module := range selected {
		if !known[module] {
			return Plan{}, &UnknownSelectionError{Field: "remove", Name: module, Module: module}
		}
	}
	steps := make([]Step, 0)
	for i := len(plan.Steps) - 1; i >= 0; i-- {
		step := plan.Steps[i]
		if !selected[step.Module] {
			continue
		}
		if step.Inverse == nil || step.Inverse.Operation == "" {
			return Plan{}, fmt.Errorf("%w: %s", ErrMissingInverse, step.ID)
		}
		steps = append(steps, Step{
			ID: step.ID, Module: step.Module, Description: step.Description,
			Scope: step.Scope, Network: step.Network, Operation: step.Inverse.Operation,
			Disposition: DispositionRemove, Desired: cloneJSONValue(step.Inverse.Value), Observed: cloneJSONValue(step.Desired),
		})
	}
	return Plan{Selection: cloneSelection(plan.Selection), Steps: steps}, nil
}
