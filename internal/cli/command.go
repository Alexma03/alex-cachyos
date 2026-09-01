package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type Command string

const (
	CommandApply      Command = "apply"
	CommandCheck      Command = "check"
	CommandAdopt      Command = "adopt"
	CommandRollback   Command = "rollback"
	CommandCheckpoint Command = "checkpoint"
	CommandReceipt    Command = "receipt"
	CommandStatus     Command = "status"
)

type Options struct {
	Command                     Command
	Action                      string
	Host, IntegrationTarget     string
	Only, With, Without, Remove []string
	DryRun, Check, List, Help   bool
	JSON                        bool
	Target, ReceiptID           string
	Tag, Message                string
}

type csvFlag []string

func (v *csvFlag) String() string { return strings.Join(*v, ",") }
func (v *csvFlag) Set(s string) error {
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			*v = append(*v, item)
		}
	}
	return nil
}

var ErrUsage = errors.New("usage error")
var modules = []string{"bootstrap", "fingerprint", "devtools", "apps", "vicinae", "desktop", "verify"}

func Modules() []string  { return append([]string(nil), modules...) }
func ListOutput() string { return strings.Join(modules, ", ") }

func Parse(args []string) (Options, error) {
	o := Options{Command: CommandApply}
	remaining := append([]string(nil), args...)
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		o.Command, remaining = Command(remaining[0]), remaining[1:]
		if !knownCommand(o.Command) {
			return Options{}, usageError("unknown command %q", o.Command)
		}
		if o.Command == CommandCheckpoint || o.Command == CommandReceipt {
			if len(remaining) == 0 || strings.HasPrefix(remaining[0], "-") {
				return Options{}, usageError("%s requires an action", o.Command)
			}
			o.Action, remaining = remaining[0], remaining[1:]
		}
	}

	f := flag.NewFlagSet("alex-cachyos", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Host, "host", "", "host name")
	f.StringVar(&o.IntegrationTarget, "integration-target", "", "authorize live verification only on this matching host")
	f.Var((*csvFlag)(&o.Only), "only", "comma-separated modules")
	f.Var((*csvFlag)(&o.With), "with", "modules to add")
	f.Var((*csvFlag)(&o.Without), "without", "modules to omit")
	f.Var((*csvFlag)(&o.Remove), "remove", "modules to remove")
	f.BoolVar(&o.DryRun, "dry-run", false, "plan without mutation")
	f.BoolVar(&o.Check, "check", false, "verify only")
	f.BoolVar(&o.List, "list", false, "list modules")
	f.BoolVar(&o.JSON, "json", false, "render machine-readable JSON")
	f.StringVar(&o.Target, "target", "", "managed path to adopt")
	f.StringVar(&o.ReceiptID, "receipt", "", "immutable receipt ID")
	f.StringVar(&o.Tag, "tag", "", "annotated catalog tag")
	f.StringVar(&o.Message, "m", "", "checkpoint message")
	f.BoolVar(&o.Help, "h", false, "show help")
	f.BoolVar(&o.Help, "help", false, "show help")
	if err := f.Parse(remaining); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.Help = true
			return o, nil
		}
		return Options{}, usageError("%v", err)
	}
	if f.NArg() != 0 {
		return Options{}, usageError("unexpected argument %q", f.Arg(0))
	}
	if o.Check {
		if o.Command != CommandApply {
			return Options{}, usageError("--check cannot be combined with %s", o.Command)
		}
		o.Command = CommandCheck
	}
	if o.Command == CommandCheck {
		o.Only = []string{"verify"}
	}
	if o.Help || o.List {
		return o, nil
	}
	if err := validateOptions(o); err != nil {
		return Options{}, err
	}
	return o, nil
}

func knownCommand(command Command) bool {
	switch command {
	case CommandApply, CommandCheck, CommandAdopt, CommandRollback, CommandCheckpoint, CommandReceipt, CommandStatus:
		return true
	default:
		return false
	}
}

func validateOptions(o Options) error {
	switch o.Command {
	case CommandApply:
		if o.Target != "" || o.ReceiptID != "" || o.Tag != "" || o.Message != "" || o.Action != "" {
			return usageError("apply received command-specific options")
		}
	case CommandCheck:
		if o.Target != "" || o.ReceiptID != "" || o.Tag != "" || o.Message != "" || o.Action != "" || len(o.With) != 0 || len(o.Without) != 0 || len(o.Remove) != 0 || o.DryRun {
			return usageError("check accepts only host and output options")
		}
	case CommandAdopt:
		if o.Target == "" {
			return usageError("adopt requires --target")
		}
		if o.ReceiptID != "" || o.Tag != "" || o.Message != "" || o.Action != "" || hasPlanOptions(o) {
			return usageError("adopt received incompatible options")
		}
	case CommandRollback:
		if (o.ReceiptID == "") == (o.Tag == "") {
			return usageError("rollback requires exactly one of --receipt or --tag")
		}
		if o.Target != "" || o.Message != "" || o.Action != "" || len(o.Only) != 0 || len(o.With) != 0 || len(o.Without) != 0 || o.DryRun {
			return usageError("rollback received incompatible options")
		}
	case CommandCheckpoint:
		if o.Action != "create" || o.Tag == "" || strings.TrimSpace(o.Message) == "" {
			return usageError("checkpoint create requires --tag and -m")
		}
		if o.Target != "" || o.ReceiptID != "" || hasPlanOptions(o) || o.Host != "" || o.IntegrationTarget != "" {
			return usageError("checkpoint received incompatible options")
		}
	case CommandReceipt:
		if o.Action != "show" {
			return usageError("receipt supports only the show action")
		}
		if o.Target != "" || o.ReceiptID != "" || o.Tag != "" || o.Message != "" || hasPlanOptions(o) || o.Host != "" || o.IntegrationTarget != "" {
			return usageError("receipt received incompatible options")
		}
	case CommandStatus:
		if o.Target != "" || o.ReceiptID != "" || o.Tag != "" || o.Message != "" || o.Action != "" || hasPlanOptions(o) || o.Host != "" || o.IntegrationTarget != "" {
			return usageError("status received incompatible options")
		}
	}
	return nil
}

func hasPlanOptions(o Options) bool {
	return len(o.Only) != 0 || len(o.With) != 0 || len(o.Without) != 0 || len(o.Remove) != 0 || o.DryRun || o.Check
}
func usageError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUsage, fmt.Sprintf(format, args...))
}

func Usage() string {
	return `Usage:
  alex-cachyos apply [options]
  alex-cachyos check [--host NAME] [--json]
  alex-cachyos adopt --target PATH [--host NAME] [--json]
  alex-cachyos rollback (--receipt ID | --tag catalog-vX.Y.Z) [--remove MODULES] [--json]
  alex-cachyos checkpoint create --tag catalog-vX.Y.Z -m MESSAGE [--json]
  alex-cachyos receipt show [--json]
  alex-cachyos status [--json]

Compatibility: omitting a subcommand selects apply; --check selects check.

Options:
  --host NAME
  --integration-target HOST
  --only MODULES
  --with MODULES
  --without MODULES
  --remove MODULES
  --dry-run
  --check
  --list
  --json
  -h, --help
`
}
