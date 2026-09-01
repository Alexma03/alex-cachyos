package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

type Options struct {
	Host, IntegrationTarget     string
	Only, With, Without, Remove []string
	DryRun, Check, List, Help   bool
}

type csvFlag []string

func (v *csvFlag) String() string { return strings.Join(*v, ",") }
func (v *csvFlag) Set(s string) error {
	for _, item := range strings.Split(s, ",") {
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
	var o Options
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
	f.BoolVar(&o.Help, "h", false, "show help")
	f.BoolVar(&o.Help, "help", false, "show help")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			o.Help = true
			return o, nil
		}
		return o, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if f.NArg() != 0 {
		return o, fmt.Errorf("%w: unexpected argument %q", ErrUsage, f.Arg(0))
	}
	if o.Check {
		o.Only = []string{"verify"}
	}
	return o, nil
}

func Usage() string {
	return "Usage: alex-cachyos [options]\n\nOptions:\n  --host NAME\n  --integration-target HOST\n  --only MODULES\n  --with MODULES\n  --without MODULES\n  --remove MODULES\n  --dry-run\n  --check\n  --list\n  -h, --help\n"
}
