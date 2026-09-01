package executor

import (
	"context"
	"errors"
	"fmt"

	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

var (
	ErrCommandRegistry         = errors.New("invalid command registry")
	ErrCommandUnavailable      = errors.New("command request unavailable")
	ErrCommandMetadataMismatch = errors.New("command request metadata mismatch")
	ErrNetworkPortRequired     = errors.New("network authorization port required")
	ErrCommandExecution        = errors.New("command execution failed")
)

// NetworkPort is the explicit seam for permitting a network-classified
// mutation. Dry-run never reaches this port because it never invokes Mutate.
type NetworkPort interface {
	Authorize(context.Context, runner.CommandRequest) error
}

// CommandMutator maps a planner step ID to one prevalidated argv-only request.
// It is the production bridge from the convergent executor to Runner.
type CommandMutator struct {
	runner   runner.Runner
	requests map[string]runner.CommandRequest
	network  NetworkPort
}

func NewCommandMutator(run runner.Runner, requests []runner.CommandRequest, network NetworkPort) (*CommandMutator, error) {
	if run == nil {
		return nil, fmt.Errorf("%w: runner is nil", ErrCommandRegistry)
	}
	registry := make(map[string]runner.CommandRequest, len(requests))
	needsNetwork := false
	for _, input := range requests {
		request := cloneCommandRequest(input)
		if err := runner.ValidateCommandRequest(request); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCommandRegistry, err)
		}
		if _, exists := registry[request.Operation]; exists {
			return nil, fmt.Errorf("%w: duplicate operation", ErrCommandRegistry)
		}
		registry[request.Operation] = request
		needsNetwork = needsNetwork || request.Network == runner.NetworkRequired
	}
	if needsNetwork && network == nil {
		return nil, ErrNetworkPortRequired
	}
	return &CommandMutator{runner: run, requests: registry, network: network}, nil
}

func (m *CommandMutator) Mutate(ctx context.Context, step planner.Step) error {
	if m == nil || m.runner == nil {
		return ErrCommandUnavailable
	}
	request, ok := m.requests[step.ID]
	if !ok {
		return ErrCommandUnavailable
	}
	if string(step.Scope) != string(request.Scope) || string(step.Network) != string(request.Network) {
		return ErrCommandMetadataMismatch
	}
	request = cloneCommandRequest(request)
	if request.Network == runner.NetworkRequired {
		if m.network == nil {
			return ErrNetworkPortRequired
		}
		if err := m.network.Authorize(ctx, cloneCommandRequest(request)); err != nil {
			return fmt.Errorf("network authorization: %w", err)
		}
	}
	result, err := m.runner.Run(ctx, request)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCommandExecution, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: non-zero exit", ErrCommandExecution)
	}
	return nil
}

func cloneCommandRequest(input runner.CommandRequest) runner.CommandRequest {
	copy := input
	copy.Argv = append([]string(nil), input.Argv...)
	copy.Stdin = append([]byte(nil), input.Stdin...)
	if input.Env != nil {
		copy.Env = make(map[string]string, len(input.Env))
		for key, value := range input.Env {
			copy.Env[key] = value
		}
	}
	return copy
}
