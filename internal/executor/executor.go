package executor
import (
	"context"
	"errors"
	"fmt"
	"alex-cachyos/internal/planner"
)
type Observer interface {
	Observe(context.Context, planner.Step) ([]byte, error)
}
type Mutator interface {
	Mutate(context.Context, planner.Step) error
}
type ObserverFunc func(context.Context, planner.Step) ([]byte, error)
func (f ObserverFunc) Observe(ctx context.Context, step planner.Step) ([]byte, error) { return f(ctx, step) }
type MutatorFunc func(context.Context, planner.Step) error
func (f MutatorFunc) Mutate(ctx context.Context, step planner.Step) error { return f(ctx, step) }
type Options struct{ DryRun bool }
type Executor struct {
	Observer Observer
	Mutator  Mutator
}
func NewExecutor(observer Observer, mutator Mutator) *Executor { return &Executor{observer, mutator} }
func Execute(ctx context.Context, plan planner.Plan, observer Observer, mutator Mutator, options ...Options) (Result, error) { return NewExecutor(observer, mutator).Execute(ctx, plan, options...) }
const (
	OutcomeSatisfied, OutcomeApplied, OutcomeBlocked = "satisfied", "applied", "blocked"
	OutcomeProjected, OutcomeFailed                  = "projected", "failed"
)
type Outcome struct {
	Step                    planner.Step
	StepID, Module, Outcome string
	Disposition             planner.Disposition
	Desired, Observed       planner.JSONValue
	NoChange, Projected     bool
	ErrorCode               string
}
type Result struct {
	Outcomes []Outcome
	NoChange bool
}
type Stage string
type ErrorClass string
const (
	StageDisposition   Stage      = "disposition"
	StageObserve       Stage      = "observe"
	StageCompare       Stage      = "compare"
	StageReobserve     Stage      = "reobserve"
	StageApply         Stage      = "apply"
	StagePostObserve   Stage      = "postobserve"
	StagePostcondition Stage      = "postcondition"
	ClassDisposition   ErrorClass = "disposition"
	ClassObserver      ErrorClass = "observer"
	ClassComparison    ErrorClass = "comparison"
	ClassMutator       ErrorClass = "mutator"
	ClassPostcondition ErrorClass = "postcondition"
)
var (
	ErrStage                  = errors.New("executor stage failed")
	ErrUnsupportedDisposition = errors.New("unsupported step disposition")
	ErrObserve                = errors.New("observe stage failed")
	ErrCompare                = errors.New("compare stage failed")
	ErrReobserve              = errors.New("reobserve stage failed")
	ErrApply                  = errors.New("apply stage failed")
	ErrPostObserve            = errors.New("post-observe stage failed")
	ErrPostcondition          = errors.New("postcondition failed")
	ErrMissingPort            = errors.New("missing executor port")
	ErrMissingObserver        = errors.New("missing observer port")
	ErrMissingMutator         = errors.New("missing mutator port")
)
// StepError keeps Error safe while retaining typed causes for errors.Is/As.
type StepError struct {
	StepID                string
	Stage                 Stage
	Class, Classification ErrorClass
	cause                 error
}
type StageError = StepError
func (e *StepError) Error() string {
	if e == nil { return ErrStage.Error() }
	return fmt.Sprintf("step %q: %s stage failed", e.StepID, e.Stage)
}
func (e *StepError) Unwrap() error {
	if e == nil { return ErrStage }
	return errors.Join(ErrStage, stageSentinel(e.Stage), e.cause)
}
func stageSentinel(stage Stage) error {
	switch stage {
	case StageDisposition:
		return ErrUnsupportedDisposition
	case StageObserve:
		return ErrObserve
	case StageCompare:
		return ErrCompare
	case StageReobserve:
		return ErrReobserve
	case StageApply:
		return ErrApply
	case StagePostObserve:
		return ErrPostObserve
	case StagePostcondition:
		return ErrPostcondition
	default:
		return ErrStage
	}
}
func stepError(id string, stage Stage, class ErrorClass, cause error) *StepError { return &StepError{id, stage, class, class, cause} }
type Port string
const (
	PortObserver Port = "observer"
	PortMutator  Port = "mutator"
)
type MissingPortError struct {
	StepID string
	Port   Port
}
func (e *MissingPortError) Error() string {
	if e == nil { return ErrMissingPort.Error() }
	return fmt.Sprintf("step %q: missing %s port", e.StepID, e.Port)
}
func (e *MissingPortError) Is(target error) bool { return target == ErrMissingPort || e.Port == PortObserver && target == ErrMissingObserver || e.Port == PortMutator && target == ErrMissingMutator }
func (e *Executor) Execute(ctx context.Context, plan planner.Plan, options ...Options) (Result, error) {
	option := Options{}
	if len(options) > 0 { option = options[0] }
	result := Result{Outcomes: make([]Outcome, 0, len(plan.Steps)), NoChange: true}
	for _, input := range plan.Steps {
		step := cloneStep(input)
		outcome := newOutcome(step)
		switch step.Disposition {
		case planner.DispositionBlocked:
			outcome.Outcome, outcome.ErrorCode = OutcomeBlocked, string(planner.DispositionBlocked)
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		case planner.DispositionSatisfied, planner.DispositionApply, planner.DispositionRemove:
		default:
			return fail(result, outcome, stepError(step.ID, StageDisposition, ClassDisposition, ErrUnsupportedDisposition))
		}
		if e == nil || e.Observer == nil { return fail(result, outcome, &MissingPortError{step.ID, PortObserver}) }
		observed, err := e.Observer.Observe(ctx, cloneStep(step))
		if err != nil { return fail(result, outcome, stepError(step.ID, StageObserve, ClassObserver, err)) }
		setObserved(&outcome, observed)
		equal, err := compare(step.ID, StageCompare, observed, step.Desired)
		if err != nil { return fail(result, outcome, err) }
		if !equal {
			fresh := cloneStep(step)
			fresh.Observed = cloneJSON(observed)
			observed, err = e.Observer.Observe(ctx, fresh)
			if err != nil { return fail(result, outcome, stepError(step.ID, StageReobserve, ClassObserver, err)) }
			setObserved(&outcome, observed)
			equal, err = compare(step.ID, StageCompare, observed, step.Desired)
			if err != nil { return fail(result, outcome, err) }
		}
		if equal {
			step.Disposition = planner.DispositionSatisfied
			outcome = newOutcome(step)
			setObserved(&outcome, observed)
			outcome.Outcome, outcome.NoChange = OutcomeSatisfied, true
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		}
		if step.Disposition == planner.DispositionSatisfied { step.Disposition = planner.DispositionApply }
		step.Observed = cloneJSON(observed)
		outcome = newOutcome(step)
		result.NoChange = false
		if option.DryRun {
			outcome.Outcome, outcome.Projected = OutcomeProjected, true
			result.Outcomes = append(result.Outcomes, outcome)
			continue
		}
		if e.Mutator == nil { return fail(result, outcome, &MissingPortError{step.ID, PortMutator}) }
		if err := e.Mutator.Mutate(ctx, cloneStep(step)); err != nil { return fail(result, outcome, stepError(step.ID, StageApply, ClassMutator, err)) }
		post, err := e.Observer.Observe(ctx, cloneStep(step))
		if err != nil { return fail(result, outcome, stepError(step.ID, StagePostObserve, ClassObserver, err)) }
		setObserved(&outcome, post)
		equal, err = compare(step.ID, StagePostcondition, post, step.Desired)
		if err != nil { return fail(result, outcome, err) }
		if !equal { return fail(result, outcome, stepError(step.ID, StagePostcondition, ClassPostcondition, ErrJSONMismatch)) }
		outcome.Outcome = OutcomeApplied
		result.Outcomes = append(result.Outcomes, outcome)
	}
	return result, nil
}
func compare(id string, stage Stage, observed, desired []byte) (bool, error) {
	err := EqualJSON(observed, desired)
	if err == nil { return true, nil }
	if errors.Is(err, ErrJSONMismatch) { return false, nil }
	class := ClassComparison
	if stage == StagePostcondition { class = ClassPostcondition }
	return false, stepError(id, stage, class, err)
}
func newOutcome(step planner.Step) Outcome {
	copy := cloneStep(step)
	return Outcome{Step: copy, StepID: step.ID, Module: step.Module, Disposition: step.Disposition,
		Desired: cloneJSON(step.Desired), Observed: cloneJSON(step.Observed)}
}
func setObserved(outcome *Outcome, observed []byte) { outcome.Observed, outcome.Step.Observed = cloneJSON(observed), cloneJSON(observed) }
func fail(result Result, outcome Outcome, err error) (Result, error) {
	outcome.Outcome, outcome.NoChange = OutcomeFailed, false
	switch value := err.(type) {
	case *StepError:
		outcome.ErrorCode = string(value.Stage)
	case *MissingPortError:
		outcome.ErrorCode = string(value.Port)
	}
	result.Outcomes, result.NoChange = append(result.Outcomes, outcome), false
	return result, err
}
func cloneJSON(value planner.JSONValue) planner.JSONValue {
	if value == nil { return nil }
	out := make(planner.JSONValue, len(value))
	copy(out, value)
	return out
}
func cloneStrings(value []string) []string {
	if value == nil { return nil }
	out := make([]string, len(value))
	copy(out, value)
	return out
}
func cloneStep(value planner.Step) planner.Step {
	copy := value
	copy.DependsOn, copy.Desired, copy.Observed = cloneStrings(value.DependsOn), cloneJSON(value.Desired), cloneJSON(value.Observed)
	if value.Inverse != nil {
		inverse := *value.Inverse
		inverse.Value = cloneJSON(value.Inverse.Value)
		copy.Inverse = &inverse
	}
	return copy
}
