package executor
import (
	"context"
	"errors"
	"strings"
	"testing"
	"alex-cachyos/internal/planner"
)
type queueObserver struct {
	values [][]byte
	errs   []error
	seen   []planner.Step
}
func (o *queueObserver) Observe(_ context.Context, step planner.Step) ([]byte, error) {
	o.seen = append(o.seen, cloneStep(step))
	i := len(o.seen) - 1
	if i < len(o.errs) && o.errs[i] != nil { return nil, o.errs[i] }
	if i >= len(o.values) { return nil, errors.New("unexpected observation") }
	return cloneJSON(o.values[i]), nil
}
type recordingMutator struct {
	seen []planner.Step
	err  error
}
func (m *recordingMutator) Mutate(_ context.Context, step planner.Step) error {
	m.seen = append(m.seen, cloneStep(step))
	return m.err
}
func step(disposition planner.Disposition) planner.Step {
	return planner.Step{ID: "config", Module: "desktop", Disposition: disposition, Network: planner.NetworkNone,
		Desired: []byte(`{"value":1}`), Observed: []byte(`{"value":0}`), DependsOn: []string{"prepare"},
		Inverse: &planner.InverseDescriptor{Value: []byte(`{"old":true}`)}}
}
func execute(t *testing.T, disposition planner.Disposition, values []string, dry bool) (Result, *queueObserver, *recordingMutator, error) {
	t.Helper()
	observer := &queueObserver{}
	for _, value := range values { observer.values = append(observer.values, []byte(value)) }
	mutator := &recordingMutator{}
	result, err := NewExecutor(observer, mutator).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step(disposition)}}, Options{DryRun: dry})
	return result, observer, mutator, err
}
func TestExecuteLifecycleAndNoChange(t *testing.T) {
	result, observer, mutator, err := execute(t, planner.DispositionApply, []string{`{"value":0}`, `{"value":0}`, `{"value":1}`}, false)
	if err != nil || result.NoChange || len(observer.seen) != 3 || len(mutator.seen) != 1 || result.Outcomes[0].Outcome != OutcomeApplied { t.Fatalf("apply: %#v %v", result, err) }
	result, observer, mutator, err = execute(t, planner.DispositionApply, []string{`{"value":1}`}, false)
	if err != nil || !result.NoChange || len(observer.seen) != 1 || len(mutator.seen) != 0 || result.Outcomes[0].Outcome != OutcomeSatisfied { t.Fatalf("second: %#v %v", result, err) }
	result, observer, mutator, err = execute(t, planner.DispositionSatisfied, []string{`{"value":0}`, `{"value":1}`}, false)
	if err != nil || !result.NoChange || len(observer.seen) != 2 || len(mutator.seen) != 0 { t.Fatalf("converged stale plan: %#v %v", result, err) }
}
func TestExecuteDispositionSwitchAndEffectiveDisposition(t *testing.T) {
	blocked, unsupported := step(planner.DispositionBlocked), step(planner.Disposition("future"))
	unsupported.ID = "future"
	result, err := (&Executor{}).Execute(context.Background(), planner.Plan{Steps: []planner.Step{blocked, unsupported}})
	var stepErr *StepError
	if !errors.Is(err, ErrUnsupportedDisposition) || !errors.As(err, &stepErr) || stepErr.Stage != StageDisposition || len(result.Outcomes) != 2 || result.Outcomes[0].Outcome != OutcomeBlocked { t.Fatalf("unsupported: %#v %v", result, err) }
	for _, dry := range []bool{true, false} {
		values := []string{`{"value":0}`, `{"value":0}`}
		if !dry { values = append(values, `{"value":1}`) }
		result, observer, mutator, err := execute(t, planner.DispositionRemove, values, dry)
		if err != nil || result.Outcomes[0].Disposition != planner.DispositionRemove || result.Outcomes[0].Step.Disposition != planner.DispositionRemove { t.Fatalf("remove dry=%v: %#v %v", dry, result, err) }
		if dry && (result.Outcomes[0].Outcome != OutcomeProjected || len(mutator.seen) != 0) { t.Fatal("remove dry-run mutated") }
		if !dry && (len(mutator.seen) != 1 || mutator.seen[0].Disposition != planner.DispositionRemove || observer.seen[2].Disposition != planner.DispositionRemove) { t.Fatal("remove replay lost disposition") }
	}
	result, _, mutator, err := execute(t, planner.DispositionSatisfied, []string{`{"value":0}`, `{"value":0}`}, true)
	if err != nil || result.Outcomes[0].Disposition != planner.DispositionApply || result.Outcomes[0].Step.Disposition != planner.DispositionApply { t.Fatalf("stale projection: %#v %v", result, err) }
	result, observer, mutator, err := execute(t, planner.DispositionSatisfied, []string{`{"value":0}`, `{"value":0}`, `{"value":1}`}, false)
	if err != nil || len(mutator.seen) != 1 || mutator.seen[0].Disposition != planner.DispositionApply || observer.seen[2].Disposition != planner.DispositionApply { t.Fatalf("stale apply: %#v %v", result, err) }
}
type callbackCause struct{ text string }
func (e *callbackCause) Error() string { return e.text }
func TestExecutePreservesCauseClassificationWithoutLeaking(t *testing.T) {
	const secret = "callback-secret"
	cause, marker := &callbackCause{secret}, errors.New("callback marker")
	_, err := NewExecutor(&queueObserver{errs: []error{errors.Join(marker, cause)}}, nil).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step(planner.DispositionApply)}})
	var stepErr *StepError
	var gotCause *callbackCause
	if !errors.Is(err, marker) || !errors.As(err, &gotCause) || !errors.As(err, &stepErr) || stepErr.Stage != StageObserve || strings.Contains(err.Error(), secret) { t.Fatalf("callback error: %v", err) }
	_, err = NewExecutor(&queueObserver{values: [][]byte{[]byte(`{"secret":`)}}, nil).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step(planner.DispositionApply)}})
	var comparison *ComparisonError
	if !errors.As(err, &comparison) || !errors.Is(err, ErrInvalidObserved) || strings.Contains(err.Error(), "secret") { t.Fatalf("comparison error: %v", err) }
}
func TestExecuteIsolatesEveryCallbackAndOutput(t *testing.T) {
	planned := step(planner.DispositionSatisfied)
	calls := 0
	observer := ObserverFunc(func(_ context.Context, got planner.Step) ([]byte, error) {
		calls++
		want := planner.DispositionSatisfied
		if calls == 3 { want = planner.DispositionApply }
		if got.Disposition != want || string(got.Desired) != `{"value":1}` || got.DependsOn[0] != "prepare" || string(got.Inverse.Value) != `{"old":true}` { t.Errorf("callback %d got mutated step %#v", calls, got) }
		got.Desired[0], got.DependsOn[0], got.Inverse.Value[0] = 'x', "changed", 'x'
		if calls < 3 { return []byte(`{"value":0}`), nil }
		return []byte(`{"value":1}`), nil
	})
	mutator := MutatorFunc(func(_ context.Context, got planner.Step) error {
		if got.Disposition != planner.DispositionApply || string(got.Desired) != `{"value":1}` { t.Errorf("mutator got %#v", got) }
		got.Desired[0], got.DependsOn[0], got.Inverse.Value[0] = 'x', "changed", 'x'
		return nil
	})
	result, err := NewExecutor(observer, mutator).Execute(context.Background(), planner.Plan{Steps: []planner.Step{planned}})
	got := result.Outcomes[0]
	if err != nil || string(planned.Desired) != `{"value":1}` || string(got.Desired) != `{"value":1}` || string(got.Step.Desired) != `{"value":1}` || got.Step.DependsOn[0] != "prepare" || string(got.Step.Inverse.Value) != `{"old":true}` { t.Fatalf("isolation: %#v %v", result, err) }
}
func TestExecutePreservesNilVersusEmptyAndMissingPorts(t *testing.T) {
	nilStep, emptyStep := step(planner.DispositionBlocked), step(planner.DispositionBlocked)
	nilStep.ID, emptyStep.ID = "nil", "empty"
	nilStep.Desired, nilStep.Observed, nilStep.DependsOn, nilStep.Inverse.Value = nil, nil, nil, nil
	emptyStep.Desired, emptyStep.Observed, emptyStep.DependsOn, emptyStep.Inverse.Value = []byte{}, []byte{}, []string{}, []byte{}
	result, err := (&Executor{}).Execute(context.Background(), planner.Plan{Steps: []planner.Step{nilStep, emptyStep}})
	if err != nil || result.Outcomes[0].Step.Desired != nil || result.Outcomes[0].Step.DependsOn != nil || result.Outcomes[0].Step.Inverse.Value != nil || result.Outcomes[1].Step.Desired == nil || result.Outcomes[1].Step.DependsOn == nil || result.Outcomes[1].Step.Inverse.Value == nil { t.Fatalf("nil/empty: %#v %v", result, err) }
	_, err = NewExecutor(nil, nil).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step(planner.DispositionApply)}}, Options{DryRun: true})
	if !errors.Is(err, ErrMissingObserver) { t.Fatalf("observer: %v", err) }
	observer := &queueObserver{values: [][]byte{[]byte(`{"value":0}`), []byte(`{"value":0}`)}}
	_, err = NewExecutor(observer, nil).Execute(context.Background(), planner.Plan{Steps: []planner.Step{step(planner.DispositionApply)}})
	if !errors.Is(err, ErrMissingMutator) { t.Fatalf("mutator: %v", err) }
}
