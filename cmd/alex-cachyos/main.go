package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"alex-cachyos/internal/cli"
	"alex-cachyos/internal/platform/cachyos"
	"alex-cachyos/internal/runner"
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
		fmt.Fprintln(stderr, err)
		return cli.ExitCode(err)
	}
	if o.Help {
		fmt.Fprint(stdout, cli.Usage())
	} else if o.List {
		fmt.Fprintln(stdout, strings.Join(cli.Modules(), ", "))
	}
	return 0
}
