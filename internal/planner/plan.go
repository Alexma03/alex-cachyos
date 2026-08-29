package planner

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrUnknownSelection  = errors.New("unknown selection")
	ErrMissingDependency = errors.New("missing dependency")
	ErrDuplicateModule   = errors.New("duplicate module")
	ErrDuplicateModuleID = ErrDuplicateModule
	ErrDuplicateStep     = errors.New("duplicate step")
	ErrDuplicateStepID   = ErrDuplicateStep
	ErrCycle             = errors.New("dependency cycle")
	ErrDependencyCycle   = ErrCycle
)

// Module is a value input. BuildPlan copies its slices before using them.
type Module struct {
	Name string
	Enabled bool
	DependsOn []string
	Steps []Step
}

// Step is a value input and plan value.
type Step struct {
	ID string
	Module string
	DependsOn []string
}

type Selection struct {
	Only []string
	With []string
	Without []string
}

type Plan struct {
	Selection Selection
	Steps []Step
}

type UnknownSelectionError struct { Field, Name, Module string }
func (e *UnknownSelectionError) Error() string { return fmt.Sprintf("unknown %s selection %q", e.Field, e.Name) }
func (e *UnknownSelectionError) Unwrap() error { return ErrUnknownSelection }

type MissingDependencyError struct { Kind, Subject, Dependency, Module, Step, ID string }
func (e *MissingDependencyError) Error() string {
	return fmt.Sprintf("%s %q has missing %s dependency %q", e.Kind, e.Subject, e.Kind, e.Dependency)
}
func (e *MissingDependencyError) Unwrap() error { return ErrMissingDependency }

type DuplicateModuleError struct{ Name, ID string }
func (e *DuplicateModuleError) Error() string { return fmt.Sprintf("duplicate module ID %q", e.Name) }
func (e *DuplicateModuleError) Unwrap() error { return ErrDuplicateModule }
type DuplicateModuleIDError = DuplicateModuleError

type DuplicateStepError struct{ ID string }
func (e *DuplicateStepError) Error() string { return fmt.Sprintf("duplicate step ID %q", e.ID) }
func (e *DuplicateStepError) Unwrap() error { return ErrDuplicateStep }
type DuplicateStepIDError = DuplicateStepError

type CycleError struct{ Nodes, IDs, Path []string }
func (e *CycleError) Error() string { return fmt.Sprintf("dependency cycle involving %v", e.Nodes) }
func (e *CycleError) Unwrap() error { return ErrCycle }
type DependencyCycleError = CycleError

// BuildPlan selects modules and returns their steps in a stable topological
// order. It does not consult a catalog or perform I/O.
func BuildPlan(modules []Module, selection Selection) (Plan, error) {
	byName, allSteps, err := copyDefinitions(modules)
	if err != nil { return Plan{}, err }
	if err := validateSelections(selection, byName); err != nil { return Plan{}, err }
	selected, only, err := selectModules(byName, selection)
	if err != nil { return Plan{}, err }
	if cycle := moduleCycle(selected, byName); len(cycle) != 0 { return Plan{}, cycleError(cycle) }
	ordered, err := orderSteps(selected, byName, allSteps, only)
	if err != nil { return Plan{}, err }
	return Plan{Selection: cloneSelection(selection), Steps: cloneSteps(ordered)}, nil
}

// Build is a short alias for BuildPlan.
func Build(modules []Module, selection Selection) (Plan, error) { return BuildPlan(modules, selection) }

func copyDefinitions(inputs []Module) (map[string]Module, map[string]Step, error) {
	modules, steps := make(map[string]Module, len(inputs)), make(map[string]Step)
	for _, input := range inputs {
		if _, ok := modules[input.Name]; ok { return nil, nil, &DuplicateModuleError{Name: input.Name, ID: input.Name} }
		module := Module{Name: input.Name, Enabled: input.Enabled, DependsOn: cloneStrings(input.DependsOn), Steps: make([]Step, len(input.Steps))}
		for i, inputStep := range input.Steps {
			stepModule := inputStep.Module
			if stepModule == "" { stepModule = input.Name }
			step := Step{ID: inputStep.ID, Module: stepModule, DependsOn: cloneStrings(inputStep.DependsOn)}
			if _, ok := steps[step.ID]; ok { return nil, nil, &DuplicateStepError{ID: step.ID} }
			module.Steps[i], steps[step.ID] = step, step
		}
		modules[input.Name] = module
	}
	return modules, steps, nil
}

func validateSelections(selection Selection, modules map[string]Module) error {
	groups := []struct { name string; values []string }{
		{"only", selection.Only}, {"with", selection.With}, {"without", selection.Without},
	}
	for _, group := range groups {
		for _, name := range group.values {
			if _, ok := modules[name]; !ok { return &UnknownSelectionError{Field: group.name, Name: name, Module: name} }
		}
	}
	return nil
}

func selectModules(modules map[string]Module, selection Selection) (map[string]bool, bool, error) {
	only := len(selection.Only) != 0
	selected := make(map[string]bool)
	if only {
		for _, name := range selection.Only { selected[name] = true }
	} else {
		for name, module := range modules { if module.Enabled { selected[name] = true } }
		for _, name := range selection.With { selected[name] = true }
		for _, name := range selection.Without { delete(selected, name) }
		for {
			changed := false
			for _, name := range selectedNames(selected) {
				for _, dependency := range modules[name].DependsOn {
					dep, ok := modules[dependency]
					if !ok { return nil, false, missingModule(name, dependency) }
					if selected[dependency] { continue }
					if contains(selection.Without, dependency) || !dep.Enabled {
						return nil, false, missingModule(name, dependency)
					}
					selected[dependency], changed = true, true
				}
			}
			if !changed { break }
		}
	}
	for _, name := range selectedNames(selected) {
		for _, dependency := range modules[name].DependsOn {
			dep, ok := modules[dependency]
			if !ok { return nil, false, missingModule(name, dependency) }
			if !selected[dependency] && (!only || !dep.Enabled) {
				return nil, false, missingModule(name, dependency)
			}
		}
	}
	return selected, only, nil
}

func moduleCycle(selected map[string]bool, modules map[string]Module) []string {
	in, edges := make(map[string]int), make(map[string]map[string]bool)
	for name := range selected { in[name] = 0 }
	for name := range selected {
		for _, dependency := range modules[name].DependsOn {
			if !selected[dependency] { continue }
			if edges[dependency] == nil { edges[dependency] = make(map[string]bool) }
			if !edges[dependency][name] { edges[dependency][name], in[name] = true, in[name]+1 }
		}
	}
	ready := make([]string, 0)
	for name, degree := range in { if degree == 0 { ready = append(ready, name) } }
	processed := 0
	for len(ready) != 0 {
		sort.Slice(ready, func(i, j int) bool { return moduleLess(ready[i], ready[j]) })
		name := ready[0]; ready = ready[1:]; processed++
		for next := range edges[name] { in[next]--; if in[next] == 0 { ready = append(ready, next) } }
	}
	if processed == len(selected) { return nil }
	cycle := make([]string, 0)
	for name, degree := range in { if degree != 0 { cycle = append(cycle, name) } }
	sort.Strings(cycle)
	return cycle
}

func orderSteps(selected map[string]bool, modules map[string]Module, allSteps map[string]Step, only bool) ([]Step, error) {
	steps, edges, in := make(map[string]Step), make(map[string]map[string]bool), make(map[string]int)
	for name := range selected { for _, step := range modules[name].Steps { steps[step.ID], in[step.ID] = step, 0 } }
	add := func(from, to string) {
		if edges[from] == nil { edges[from] = make(map[string]bool) }
		if !edges[from][to] { edges[from][to], in[to] = true, in[to]+1 }
	}
	for name := range selected {
		for _, dependency := range modules[name].DependsOn {
			if !selected[dependency] { continue }
			for _, from := range modules[dependency].Steps { for _, to := range modules[name].Steps { add(from.ID, to.ID) } }
		}
	}
	for _, id := range sortedStepIDs(steps) {
		for _, dependency := range steps[id].DependsOn {
			if _, ok := allSteps[dependency]; !ok { return nil, missingStep(id, dependency) }
			if _, ok := steps[dependency]; !ok {
				if only { continue }
				return nil, missingStep(id, dependency)
			}
			add(dependency, id)
		}
	}
	ready := make([]string, 0)
	for id, degree := range in { if degree == 0 { ready = append(ready, id) } }
	ordered := make([]Step, 0, len(steps))
	for len(ready) != 0 {
		sort.Slice(ready, func(i, j int) bool { return stepLess(steps[ready[i]], steps[ready[j]]) })
		id := ready[0]; ready = ready[1:]; ordered = append(ordered, steps[id])
		for _, next := range sortedKeys(edges[id]) { in[next]--; if in[next] == 0 { ready = append(ready, next) } }
	}
	if len(ordered) != len(steps) {
		remaining := make([]string, 0)
		for id, degree := range in { if degree != 0 { remaining = append(remaining, id) } }
		sort.Strings(remaining)
		return nil, cycleError(remaining)
	}
	return ordered, nil
}

var canonicalRank = map[string]int{"bootstrap": 0, "fingerprint": 1, "devtools": 2, "apps": 3, "vicinae": 4, "desktop": 5, "verify": 7}
func moduleRank(name string) int { if rank, ok := canonicalRank[name]; ok { return rank }; return 6 }
func moduleLess(a, b string) bool { if moduleRank(a) != moduleRank(b) { return moduleRank(a) < moduleRank(b) }; return a < b }
func stepLess(a, b Step) bool { if moduleRank(a.Module) != moduleRank(b.Module) { return moduleRank(a.Module) < moduleRank(b.Module) }; return a.ID < b.ID }

func selectedNames(selected map[string]bool) []string {
	result := make([]string, 0, len(selected)); for name := range selected { result = append(result, name) }
	sort.Slice(result, func(i, j int) bool { return moduleLess(result[i], result[j]) }); return result
}
func sortedKeys(values map[string]bool) []string { result := make([]string, 0, len(values)); for value := range values { result = append(result, value) }; sort.Strings(result); return result }
func sortedStepIDs(values map[string]Step) []string { result := make([]string, 0, len(values)); for id := range values { result = append(result, id) }; sort.Strings(result); return result }
func cycleError(nodes []string) error {
	copy := cloneStrings(nodes)
	return &CycleError{Nodes: copy, IDs: cloneStrings(copy), Path: cloneStrings(copy)}
}
func missingModule(subject, dependency string) error {
	return &MissingDependencyError{Kind: "module", Subject: subject, Dependency: dependency, Module: subject, ID: dependency}
}
func missingStep(subject, dependency string) error {
	return &MissingDependencyError{Kind: "step", Subject: subject, Dependency: dependency, Step: subject, ID: dependency}
}
func contains(values []string, want string) bool { for _, value := range values { if value == want { return true } }; return false }
func cloneStrings(values []string) []string { if values == nil { return nil }; return append([]string{}, values...) }
func cloneSelection(value Selection) Selection { return Selection{cloneStrings(value.Only), cloneStrings(value.With), cloneStrings(value.Without)} }
func cloneSteps(values []Step) []Step {
	if values == nil { return nil }; result := make([]Step, len(values))
	for i, value := range values { result[i] = Step{value.ID, value.Module, cloneStrings(value.DependsOn)} }
	return result
}
