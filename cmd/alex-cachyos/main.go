package main

import (
	"fmt"
	"os"
	"strings"

	"alex-cachyos/internal/cli"
)

var (
	Version = "dev"
	Commit  = "unknown"
)

func main() {
	o, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
	if o.Help {
		fmt.Print(cli.Usage())
	} else if o.List {
		fmt.Println(strings.Join(cli.Modules(), ", "))
	}
}
