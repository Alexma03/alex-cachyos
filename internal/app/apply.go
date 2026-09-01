package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	planexecutor "alex-cachyos/internal/executor"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/runner"
	"alex-cachyos/internal/statepath"
)

// Executor is the narrow seam used by apply orchestration. The concrete
// executor owns observation, comparison, and mutation ordering.
type Executor interface {
	Execute(context.Context, planner.Plan, ...planexecutor.Options) (planexecutor.Result, error)
}

type ReceiptStoreFactory func(statepath.Paths) *receipt.Store
type LockFactory func(statepath.Paths) (*Lock, error)

type ApplyInput struct {
	Envelope receipt.Receipt
	Plan     planner.Plan
	Paths    statepath.Paths
	DryRun   bool
}

type ApplyResult struct {
	Receipt       receipt.Receipt
	ReceiptPath   string
	NetworkGroups []planner.NetworkGroup
}

type Applier struct {
	executor     Executor
	storeFactory ReceiptStoreFactory
	lockFactory  LockFactory
}

func NewApplier(run Executor, stores ReceiptStoreFactory) *Applier {
	if stores == nil {
		stores = receipt.NewStoreFromPaths
	}
	return &Applier{executor: run, storeFactory: stores, lockFactory: AcquireLock}
}
func NewApplierWithStore(run Executor, store *receipt.Store) *Applier {
	return NewApplier(run, func(statepath.Paths) *receipt.Store { return store })
}

// NewCommandApplier connects the convergent executor to the typed command
// boundary. Tests inject a fake/elevation runner; production passes an
// ElevationRunner backed by ExecRunner.
func NewCommandApplier(observer planexecutor.Observer, commandRunner runner.Runner, requests []runner.CommandRequest, network planexecutor.NetworkPort, stores ReceiptStoreFactory) (*Applier, error) {
	mutator, err := planexecutor.NewCommandMutator(commandRunner, requests, network)
	if err != nil {
		return nil, err
	}
	return NewApplier(planexecutor.NewExecutor(observer, mutator), stores), nil
}

// NewProductionApplier is the production execution wiring: argv-only ExecRunner
// for user work and the pkexec elevation decorator for system work.
func NewProductionApplier(observer planexecutor.Observer, requests []runner.CommandRequest, network planexecutor.NetworkPort, stores ReceiptStoreFactory) (*Applier, error) {
	return NewCommandApplier(observer, runner.NewElevationRunner(runner.ExecRunner{}), requests, network, stores)
}

// Apply validates and isolates its inputs before any lock or executor call, then
// publishes one receipt for every runnable outcome.
func (a *Applier) Apply(ctx context.Context, input ApplyInput) (ApplyResult, error) {
	if a == nil {
		return ApplyResult{}, errors.New("nil applier")
	}
	envelope, err := cloneEnvelope(input.Envelope)
	if err != nil {
		return ApplyResult{}, err
	}
	plan, digest, err := clonePlan(input.Plan)
	if err != nil {
		return ApplyResult{}, err
	}
	groups := planner.NetworkGroups(plan)
	if groups == nil {
		groups = []planner.NetworkGroup{}
	}
	receiptValue := envelope
	receiptValue.Selection.DryRun = input.DryRun
	receiptValue.Plan.Digest, receiptValue.Plan.NetworkRequired = digest, networkOperations(groups)
	receiptValue.Steps = []receipt.Step{}

	if input.DryRun {
		executed, executeErr := a.execute(ctx, plan, true)
		receiptValue.Steps = mapOutcomes(executed.Outcomes, receiptValue.StartedAt, receiptValue.FinishedAt)
		setExecutionStatus(&receiptValue, executed, executeErr)
		result, publishErr := a.publish(input.Paths, receiptValue, groups)
		return result, joinErrors(publishErr, executeErr)
	}

	lockFactory := a.lockFactory
	if lockFactory == nil {
		lockFactory = AcquireLock
	}
	lock, lockErr := lockFactory(input.Paths)
	if lockErr != nil {
		code := "lock-failed"
		if errors.Is(lockErr, ErrLockContention) {
			code = "lock-contention"
		}
		markFailure(&receiptValue, code)
		receiptValue.Steps = blockedSteps(plan, receiptValue.StartedAt, receiptValue.FinishedAt, code)
		result, publishErr := a.publish(input.Paths, receiptValue, groups)
		return result, joinErrors(lockErr, publishErr)
	}

	executed, executeErr := a.execute(ctx, plan, false)
	receiptValue.Steps = mapOutcomes(executed.Outcomes, receiptValue.StartedAt, receiptValue.FinishedAt)
	setExecutionStatus(&receiptValue, executed, executeErr)
	result, publishErr := a.publish(input.Paths, receiptValue, groups)
	releaseErr := lock.Release()
	return result, joinErrors(publishErr, releaseErr, executeErr)
}

func cloneEnvelope(value receipt.Receipt) (receipt.Receipt, error) {
	data, err := receipt.CanonicalJSON(value)
	if err != nil {
		return receipt.Receipt{}, errors.New("invalid receipt envelope")
	}
	copy, err := receipt.Parse(data)
	if err != nil {
		return receipt.Receipt{}, errors.New("invalid receipt envelope")
	}
	return copy, nil
}
func clonePlan(value planner.Plan) (planner.Plan, string, error) {
	digest, err := planner.Digest(value)
	if err != nil {
		return planner.Plan{}, "", errors.New("invalid plan")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return planner.Plan{}, "", errors.New("invalid plan")
	}
	var copy planner.Plan
	if err := json.Unmarshal(data, &copy); err != nil {
		return planner.Plan{}, "", errors.New("invalid plan")
	}
	return copy, digest, nil
}

func (a *Applier) execute(ctx context.Context, plan planner.Plan, dryRun bool) (planexecutor.Result, error) {
	if a.executor == nil {
		return planexecutor.Result{}, errors.New("executor unavailable")
	}
	return a.executor.Execute(ctx, plan, planexecutor.Options{DryRun: dryRun})
}
func (a *Applier) publish(paths statepath.Paths, value receipt.Receipt, groups []planner.NetworkGroup) (ApplyResult, error) {
	result := ApplyResult{Receipt: value, NetworkGroups: groups}
	if a.storeFactory == nil {
		return result, errors.New("receipt publication unavailable")
	}
	store := a.storeFactory(paths)
	if store == nil {
		return result, errors.New("receipt publication unavailable")
	}
	path, err := store.Publish(value)
	if err != nil {
		return result, fmt.Errorf("publish receipt: %w", err)
	}
	result.ReceiptPath = path
	return result, nil
}

func setExecutionStatus(value *receipt.Receipt, result planexecutor.Result, executeErr error) {
	value.Status, value.NoChange = "success", executeErr == nil && result.NoChange
	if executeErr != nil {
		markFailure(value, "executor-failed")
		return
	}
	for _, outcome := range result.Outcomes {
		mapped := safeOutcome(outcome.Outcome)
		if mapped == planexecutor.OutcomeFailed || mapped == planexecutor.OutcomeBlocked {
			markFailure(value, "executor-blocked")
			return
		}
	}
}
func markFailure(value *receipt.Receipt, code string) {
	value.Status, value.NoChange = "failed", false
	value.Errors = append(append([]string{}, value.Errors...), code)
}
func networkOperations(groups []planner.NetworkGroup) []string {
	operations := make([]string, 0, len(groups))
	for _, group := range groups {
		operations = append(operations, string(group.Operation))
	}
	return operations
}

func mapOutcomes(outcomes []planexecutor.Outcome, started, finished string) []receipt.Step {
	steps := make([]receipt.Step, 0, len(outcomes))
	for _, outcome := range outcomes {
		step, mapped := outcome.Step, safeOutcome(outcome.Outcome)
		id, module, disposition := outcome.StepID, outcome.Module, outcome.Disposition
		if id == "" {
			id = step.ID
		}
		if module == "" {
			module = step.Module
		}
		if disposition == "" {
			disposition = step.Disposition
		}
		code := safeErrorCode(outcome.ErrorCode)
		if mapped == planexecutor.OutcomeFailed && code == "" {
			code = "executor-failed"
		}
		if mapped == planexecutor.OutcomeBlocked && code == "" {
			code = "executor-blocked"
		}
		steps = append(steps, receipt.Step{ID: id, Module: module, Scope: string(step.Scope), Network: string(step.Network), Disposition: string(disposition), Outcome: mapped, Timing: receipt.Timing{StartedAt: started, FinishedAt: finished}, ErrorCode: code})
	}
	return steps
}
func blockedSteps(plan planner.Plan, started, finished, code string) []receipt.Step {
	steps := make([]receipt.Step, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		steps = append(steps, receipt.Step{ID: step.ID, Module: step.Module, Scope: string(step.Scope), Network: string(step.Network), Disposition: string(step.Disposition), Outcome: planexecutor.OutcomeBlocked, Timing: receipt.Timing{StartedAt: started, FinishedAt: finished}, ErrorCode: code})
	}
	return steps
}
func safeOutcome(value string) string {
	switch value {
	case planexecutor.OutcomeSatisfied, planexecutor.OutcomeApplied, planexecutor.OutcomeBlocked, planexecutor.OutcomeProjected, planexecutor.OutcomeFailed:
		return value
	default:
		return planexecutor.OutcomeFailed
	}
}
func safeErrorCode(value string) string {
	switch value {
	case string(planexecutor.StageDisposition), string(planexecutor.StageObserve), string(planexecutor.StageCompare), string(planexecutor.StageReobserve), string(planexecutor.StageApply), string(planexecutor.StagePostObserve), string(planexecutor.StagePostcondition), string(planexecutor.PortObserver), string(planexecutor.PortMutator), "blocked", "executor-failed", "executor-blocked", "lock-contention", "lock-failed":
		return value
	default:
		return ""
	}
}
func joinErrors(values ...error) error {
	var nonnil []error
	for _, value := range values {
		if value != nil {
			nonnil = append(nonnil, value)
		}
	}
	return errors.Join(nonnil...)
}
