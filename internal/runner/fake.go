package runner

import (
	"context"
	"fmt"
	"sync"
)

// FixtureState contains only typed, in-memory observations available to fake callbacks.
type FixtureState struct {
	Files map[string][]byte
	Packages map[string]string
	Services map[string]bool
	Values map[string]string
	Count int
}

func NewFixtureState() FixtureState {
	return FixtureState{Files: map[string][]byte{}, Packages: map[string]string{}, Services: map[string]bool{}, Values: map[string]string{}}
}

type Expectation struct {
	Operation string
	Argv []string
	Result CommandResult
	Err error
	Mutate func(*FixtureState)
	Callback func(*FixtureState)
	Apply func(*FixtureState)
}
type FakeExpectation = Expectation
type ScriptedExpectation = Expectation

type FakeRunner struct {
	mu sync.Mutex
	expectations []Expectation
	next int
	requests []CommandRequest
	unmatched []CommandRequest
	state FixtureState
}

func NewFakeRunner(expectations ...Expectation) *FakeRunner {
	return NewFakeRunnerWithState(NewFixtureState(), expectations...)
}
func NewFakeRunnerWithState(state FixtureState, expectations ...Expectation) *FakeRunner {
	copied := make([]Expectation, len(expectations))
	for i := range expectations { copied[i] = cloneExpectation(expectations[i]) }
	return &FakeRunner{expectations: copied, state: cloneFixtureState(state)}
}
func (f *FakeRunner) Expect(expectation Expectation) {
	if f == nil { return }
	f.mu.Lock(); defer f.mu.Unlock()
	f.expectations = append(f.expectations, cloneExpectation(expectation))
}

func (f *FakeRunner) Run(_ context.Context, request CommandRequest) (CommandResult, error) {
	if f == nil { return CommandResult{ExitCode: -1}, fmt.Errorf("nil fake runner") }
	request = cloneRequest(request)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request)
	if f.next >= len(f.expectations) { return f.unmatchedRequest(request, "has no scripted expectation") }
	expected := f.expectations[f.next]
	if expected.Operation != request.Operation || !sameArgv(expected.Argv, request.Argv) {
		return f.unmatchedRequest(request, fmt.Sprintf("want operation %q argv %#v", expected.Operation, expected.Argv))
	}
	f.next++
	if expected.Mutate != nil || expected.Callback != nil || expected.Apply != nil {
		state := cloneFixtureState(f.state)
		for _, mutate := range []func(*FixtureState){expected.Mutate, expected.Callback, expected.Apply} { if mutate != nil { mutate(&state) } }
		f.state = state
	}
	return cloneResult(expected.Result), expected.Err
}

func (f *FakeRunner) unmatchedRequest(request CommandRequest, detail string) (CommandResult, error) {
	f.unmatched = append(f.unmatched, cloneRequest(request))
	return CommandResult{ExitCode: -1}, fmt.Errorf("%w: operation %q argv %#v %s", ErrUnmatchedExpectation, request.Operation, request.Argv, detail)
}

func (f *FakeRunner) Requests() []CommandRequest {
	if f == nil { return nil }
	f.mu.Lock(); defer f.mu.Unlock()
	out := make([]CommandRequest, len(f.requests))
	for i := range out { out[i] = cloneRequest(f.requests[i]) }
	return out
}

func (f *FakeRunner) State() FixtureState {
	if f == nil { return NewFixtureState() }
	f.mu.Lock(); defer f.mu.Unlock()
	return cloneFixtureState(f.state)
}

func (f *FakeRunner) UnmatchedRequests() []CommandRequest {
	if f == nil { return nil }
	f.mu.Lock(); defer f.mu.Unlock()
	out := make([]CommandRequest, len(f.unmatched))
	for i := range out { out[i] = cloneRequest(f.unmatched[i]) }
	return out
}

func (f *FakeRunner) UnconsumedExpectations() []Expectation {
	if f == nil { return nil }
	f.mu.Lock(); defer f.mu.Unlock()
	out := make([]Expectation, len(f.expectations)-f.next)
	for i := range out { out[i] = cloneExpectation(f.expectations[f.next+i]) }
	return out
}

func (f *FakeRunner) Verify() error {
	if f == nil { return fmt.Errorf("nil fake runner") }
	f.mu.Lock(); defer f.mu.Unlock()
	if f.next == len(f.expectations) { return nil }
	return fmt.Errorf("%w: %d remain, next operation %q", ErrUnconsumedExpectations, len(f.expectations)-f.next, f.expectations[f.next].Operation)
}
func (f *FakeRunner) AssertExpectations() error { return f.Verify() }

func cloneExpectation(in Expectation) Expectation { in.Argv = append([]string(nil), in.Argv...); in.Result = cloneResult(in.Result); return in }
func cloneResult(in CommandResult) CommandResult { in.Stdout = append([]byte(nil), in.Stdout...); in.Stderr = append([]byte(nil), in.Stderr...); return in }
func cloneRequest(in CommandRequest) CommandRequest {
	in.Argv = append([]string(nil), in.Argv...); in.Stdin = append([]byte(nil), in.Stdin...)
	if in.Env != nil { env := make(map[string]string, len(in.Env)); for key, value := range in.Env { env[key] = value }; in.Env = env }
	return in
}
func cloneFixtureState(in FixtureState) FixtureState {
	out := NewFixtureState()
	for key, value := range in.Files { out.Files[key] = append([]byte(nil), value...) }
	for key, value := range in.Packages { out.Packages[key] = value }
	for key, value := range in.Services { out.Services[key] = value }
	for key, value := range in.Values { out.Values[key] = value }
	out.Count = in.Count
	return out
}
func sameArgv(left, right []string) bool {
	if len(left) != len(right) { return false }; for i := range left { if left[i] != right[i] { return false } }; return true
}
