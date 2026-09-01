package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/cli"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/platform/cachyos"
	"alex-cachyos/internal/receipt"
	"alex-cachyos/internal/runner"
	"alex-cachyos/internal/statepath"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, defaultSelfHelperRuntime()))
}

type selfHelperRuntime struct {
	effectiveUID func() int
	policy       func() (runner.SelfHelperPolicy, error)
	execute      func(io.Reader, runner.SelfHelperPolicy) error
}

func defaultSelfHelperRuntime() selfHelperRuntime {
	return selfHelperRuntime{
		effectiveUID: os.Geteuid,
		policy: func() (runner.SelfHelperPolicy, error) {
			value, err := strconv.ParseUint(os.Getenv("PKEXEC_UID"), 10, 32)
			if err != nil || value == 0 {
				return runner.SelfHelperPolicy{}, fmt.Errorf("invalid pkexec source identity")
			}
			return cachyos.PrivilegedHelperPolicy(uint32(value)), nil
		},
		execute: runner.ExecuteSelfHelperInput,
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, helper selfHelperRuntime) int {
	return runWithRuntime(args, stdin, stdout, stderr, helper, defaultCommandRuntime())
}

type commandRuntime interface {
	Execute(context.Context, app.CommandRequest) (app.CommandResult, error)
}

// localCommandRuntime intentionally implements only the production-independent
// receipt/status reader. Apply, check, adopt, rollback, and checkpoint require
// an injected validated catalog and their platform ports; this repository has
// no production host catalog yet, so the binary fails closed rather than
// inventing one.
type localCommandRuntime struct{}

func defaultCommandRuntime() commandRuntime { return localCommandRuntime{} }

func (localCommandRuntime) Execute(_ context.Context, request app.CommandRequest) (app.CommandResult, error) {
	switch request.Command {
	case app.CommandReceipt, app.CommandStatus:
		if request.ReceiptID != "" {
			return app.CommandResult{}, app.ErrCommandUnavailable
		}
		paths, err := statepath.Resolve()
		if err != nil {
			return app.CommandResult{}, app.ErrCommandUnavailable
		}
		value, path, err := receipt.NewStoreFromPaths(paths).Current()
		if err != nil {
			return app.CommandResult{}, err
		}
		return app.CommandResult{Command: request.Command, Host: value.Host.Resolved, Receipt: &value, ReceiptPath: path}, nil
	default:
		return app.CommandResult{}, app.ErrCommandUnavailable
	}
}

func runWithRuntime(args []string, stdin io.Reader, stdout, stderr io.Writer, helper selfHelperRuntime, runtime commandRuntime) int {
	if len(args) > 0 && args[0] == runner.SelfHelperArgument {
		if len(args) != 1 || helper.effectiveUID == nil || helper.effectiveUID() != 0 || helper.policy == nil || helper.execute == nil {
			fmt.Fprintln(stderr, "privileged helper failed")
			return 1
		}
		policy, err := helper.policy()
		if err != nil || helper.execute(stdin, policy) != nil {
			fmt.Fprintln(stderr, "privileged helper failed")
			return 1
		}
		return 0
	}

	o, err := cli.Parse(args)
	if err != nil {
		return cli.RenderCommandError(hasJSONFlag(args), err, stdout, stderr)
	}
	if o.Help {
		fmt.Fprint(stdout, cli.Usage())
	} else if o.List {
		fmt.Fprintln(stdout, strings.Join(cli.Modules(), ", "))
		return 0
	}
	if o.Help {
		return 0
	}
	if runtime == nil {
		return cli.RenderCommandError(o.JSON, app.ErrCommandUnavailable, stdout, stderr)
	}
	request := app.CommandRequest{
		Command:           app.CommandName(o.Command),
		Host:              o.Host,
		IntegrationTarget: o.IntegrationTarget,
		Selection: planner.Selection{
			Only: append([]string(nil), o.Only...), With: append([]string(nil), o.With...), Without: append([]string(nil), o.Without...),
		},
		Remove: append([]string(nil), o.Remove...), DryRun: o.DryRun,
		Target: o.Target, ReceiptID: o.ReceiptID, Tag: o.Tag, Message: o.Message,
	}
	result, err := runtime.Execute(context.Background(), request)
	if err != nil {
		return cli.RenderCommandError(o.JSON, err, stdout, stderr)
	}
	if result.Command == "" {
		result.Command = request.Command
	}
	if result.Host == "" {
		result.Host = request.Host
	}
	if err := cli.RenderCommandResult(o.JSON, result, stdout); err != nil {
		return cli.RenderCommandError(o.JSON, err, stdout, stderr)
	}
	return result.ExitCode()
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}
