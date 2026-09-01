package app

import (
	"context"
	"errors"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/receipt"
)

type CommandName string

const (
	CommandApply      CommandName = "apply"
	CommandCheck      CommandName = "check"
	CommandAdopt      CommandName = "adopt"
	CommandRollback   CommandName = "rollback"
	CommandCheckpoint CommandName = "checkpoint"
	CommandReceipt    CommandName = "receipt"
	CommandStatus     CommandName = "status"
)

var (
	ErrCommandUnavailable = errors.New("command runtime unavailable")
	ErrUnknownCommand     = errors.New("unknown command")
)

// CommandRequest contains parsed intent only. It carries no argv, output,
// environment, or filesystem bytes across the application boundary.
type CommandRequest struct {
	Command           CommandName       `json:"command"`
	Host              string            `json:"host,omitempty"`
	IntegrationTarget string            `json:"integrationTarget,omitempty"`
	Selection         planner.Selection `json:"selection"`
	Remove            []string          `json:"remove"`
	DryRun            bool              `json:"dryRun"`
	Target            string            `json:"target,omitempty"`
	ReceiptID         string            `json:"receiptId,omitempty"`
	Tag               string            `json:"tag,omitempty"`
	Message           string            `json:"message,omitempty"`
}

// CommandResult is the privacy-safe result shared by human and JSON adapters.
type CommandResult struct {
	Command     CommandName       `json:"command"`
	Host        string            `json:"host,omitempty"`
	Message     string            `json:"message,omitempty"`
	Check       *CheckReport      `json:"check,omitempty"`
	Receipt     *receipt.Receipt  `json:"receipt,omitempty"`
	ReceiptPath string            `json:"receiptPath,omitempty"`
	Rollback    *RollbackResult   `json:"rollback,omitempty"`
	Checkpoint  *CheckpointResult `json:"checkpoint,omitempty"`
	Adoption    *AdoptionResult   `json:"adoption,omitempty"`
}

func (r CommandResult) ExitCode() int {
	if r.Check != nil {
		return r.Check.ExitCode()
	}
	return 0
}

type HostRepository interface {
	KnownHosts() []string
	Resolve(string) (catalog.ResolvedHostPolicy, error)
}

type CommandOperations interface {
	Execute(context.Context, CommandRequest, catalog.ResolvedHostPolicy) (CommandResult, error)
}

type CommandOperationsFunc func(context.Context, CommandRequest, catalog.ResolvedHostPolicy) (CommandResult, error)

func (f CommandOperationsFunc) Execute(ctx context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (CommandResult, error) {
	if f == nil {
		return CommandResult{}, ErrCommandUnavailable
	}
	return f(ctx, request, policy)
}

// CommandHandlers makes each use-case dependency explicit. A missing handler
// fails closed; commands are never silently redirected to a nearby operation.
type CommandHandlers struct {
	Apply      CommandOperations
	Check      CommandOperations
	Adopt      CommandOperations
	Rollback   CommandOperations
	Checkpoint CommandOperations
	Receipt    CommandOperations
	Status     CommandOperations
}

// CheckRequestFactory derives the offline inventory from resolved desired
// state and the parsed module selection. It must not observe the live host.
type CheckRequestFactory interface {
	Build(catalog.ResolvedHostPolicy, planner.Selection) (CheckRequest, error)
}

type CheckRequestFactoryFunc func(catalog.ResolvedHostPolicy, planner.Selection) (CheckRequest, error)

func (f CheckRequestFactoryFunc) Build(policy catalog.ResolvedHostPolicy, selection planner.Selection) (CheckRequest, error) {
	if f == nil {
		return CheckRequest{}, ErrCommandUnavailable
	}
	return f(policy, selection)
}

// NewCheckCommand connects host-derived inventory to the observation-only
// checker. Check never acquires a lock or receives network/mutation ports.
func NewCheckCommand(factory CheckRequestFactory, observer CheckObserver) CommandOperations {
	return CommandOperationsFunc(func(ctx context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (CommandResult, error) {
		if request.Command != CommandCheck || factory == nil || observer == nil {
			return CommandResult{}, ErrCommandUnavailable
		}
		inventory, err := factory.Build(policy, request.Selection)
		if err != nil {
			return CommandResult{}, err
		}
		report := Check(ctx, inventory, observer)
		return CommandResult{Command: CommandCheck, Host: policy.Name, Check: &report}, nil
	})
}

func (h CommandHandlers) Execute(ctx context.Context, request CommandRequest, policy catalog.ResolvedHostPolicy) (CommandResult, error) {
	var handler CommandOperations
	switch request.Command {
	case CommandApply:
		handler = h.Apply
	case CommandCheck:
		handler = h.Check
	case CommandAdopt:
		handler = h.Adopt
	case CommandRollback:
		handler = h.Rollback
	case CommandCheckpoint:
		handler = h.Checkpoint
	case CommandReceipt:
		handler = h.Receipt
	case CommandStatus:
		handler = h.Status
	default:
		return CommandResult{}, ErrUnknownCommand
	}
	if handler == nil {
		return CommandResult{}, ErrCommandUnavailable
	}
	return handler.Execute(ctx, request, policy)
}

// CommandService resolves host/catalog authority before invoking any command
// operation. Unknown hosts therefore fail before asset lookup, observation,
// locking, network, mutation, or receipt publication.
type CommandService struct {
	repository HostRepository
	host       HostPort
	operations CommandOperations
}

func NewCommandService(repository HostRepository, host HostPort, operations CommandOperations) *CommandService {
	return &CommandService{repository: repository, host: host, operations: operations}
}

func (s *CommandService) Execute(ctx context.Context, request CommandRequest) (CommandResult, error) {
	if s == nil || s.operations == nil {
		return CommandResult{}, ErrCommandUnavailable
	}
	request = cloneCommandRequest(request)
	if !knownCommandName(request.Command) {
		return CommandResult{}, ErrUnknownCommand
	}

	var policy catalog.ResolvedHostPolicy
	if hostScoped(request) {
		if s.repository == nil {
			return CommandResult{}, ErrCommandUnavailable
		}
		knownNames := s.repository.KnownHosts()
		known := make([]KnownHost, len(knownNames))
		for i, name := range knownNames {
			known[i] = KnownHost{Name: name}
		}
		resolved, err := ResolveHost(request.Host, known, s.host)
		if err != nil {
			return CommandResult{}, err
		}
		policy, err = s.repository.Resolve(resolved)
		if err != nil {
			return CommandResult{}, err
		}
		request.Host = resolved
	}

	result, err := s.operations.Execute(ctx, request, policy)
	if err != nil {
		return CommandResult{}, err
	}
	result.Command = request.Command
	if result.Host == "" {
		result.Host = request.Host
	}
	return cloneCommandResult(result)
}

func hostScoped(request CommandRequest) bool {
	if request.Host != "" {
		return true
	}
	switch request.Command {
	case CommandApply, CommandCheck, CommandAdopt:
		return true
	default:
		return false
	}
}

func knownCommandName(command CommandName) bool {
	switch command {
	case CommandApply, CommandCheck, CommandAdopt, CommandRollback, CommandCheckpoint, CommandReceipt, CommandStatus:
		return true
	default:
		return false
	}
}

func cloneCommandRequest(value CommandRequest) CommandRequest {
	value.Selection = planner.Selection{Only: append([]string(nil), value.Selection.Only...), With: append([]string(nil), value.Selection.With...), Without: append([]string(nil), value.Selection.Without...)}
	value.Remove = append([]string(nil), value.Remove...)
	return value
}

func cloneCommandResult(value CommandResult) (CommandResult, error) {
	result := value
	if value.Check != nil {
		copy := CloneCheckReport(*value.Check)
		result.Check = &copy
	}
	if value.Receipt != nil {
		data, err := receipt.CanonicalJSON(*value.Receipt)
		if err != nil {
			return CommandResult{}, err
		}
		copy, err := receipt.Parse(data)
		if err != nil {
			return CommandResult{}, err
		}
		result.Receipt = &copy
	}
	if value.Rollback != nil {
		copy := *value.Rollback
		result.Rollback = &copy
	}
	if value.Checkpoint != nil {
		copy := *value.Checkpoint
		result.Checkpoint = &copy
	}
	if value.Adoption != nil {
		copy := *value.Adoption
		result.Adoption = &copy
	}
	return result, nil
}
