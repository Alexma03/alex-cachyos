package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestListAndExitCodes(t *testing.T) {
	o, err := Parse([]string{"--list"})
	if err != nil || !o.List {
		t.Fatalf("--list = %#v, %v", o, err)
	}
	if got := ListOutput(); got != "bootstrap, fingerprint, devtools, apps, vicinae, desktop, verify" {
		t.Fatalf("module list = %q", got)
	}
	codes := ExitCodes()
	if codes.Usage != 2 || codes.LockContention != 75 {
		t.Fatalf("exit codes = %#v", codes)
	}
}

func TestParseCommandSurfacesAndArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want Options
	}{
		{name: "default apply", args: []string{"--host", "portable", "--only", "bootstrap,apps", "--dry-run"}, want: Options{Command: CommandApply, Host: "portable", Only: []string{"bootstrap", "apps"}, DryRun: true}},
		{name: "explicit apply", args: []string{"apply", "--host", "portable", "--with", "vicinae", "--without", "fingerprint", "--remove", "apps"}, want: Options{Command: CommandApply, Host: "portable", With: []string{"vicinae"}, Without: []string{"fingerprint"}, Remove: []string{"apps"}}},
		{name: "check", args: []string{"check", "--host", "portable", "--json"}, want: Options{Command: CommandCheck, Host: "portable", Only: []string{"verify"}, JSON: true}},
		{name: "compatibility check", args: []string{"--host", "portable", "--check"}, want: Options{Command: CommandCheck, Host: "portable", Only: []string{"verify"}, Check: true}},
		{name: "adopt", args: []string{"adopt", "--host", "portable", "--target", "/home/user/.config/example"}, want: Options{Command: CommandAdopt, Host: "portable", Target: "/home/user/.config/example"}},
		{name: "rollback receipt", args: []string{"rollback", "--receipt", "run-123"}, want: Options{Command: CommandRollback, ReceiptID: "run-123"}},
		{name: "rollback tag", args: []string{"rollback", "--tag", "catalog-v1.2.3", "--remove", "desktop"}, want: Options{Command: CommandRollback, Tag: "catalog-v1.2.3", Remove: []string{"desktop"}}},
		{name: "checkpoint", args: []string{"checkpoint", "create", "--tag", "catalog-v1.2.3", "-m", "known good"}, want: Options{Command: CommandCheckpoint, Action: "create", Tag: "catalog-v1.2.3", Message: "known good"}},
		{name: "receipt", args: []string{"receipt", "show"}, want: Options{Command: CommandReceipt, Action: "show"}},
		{name: "status", args: []string{"status", "--json"}, want: Options{Command: CommandStatus, JSON: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseRejectsAmbiguousAndInvalidCommandForms(t *testing.T) {
	for _, args := range [][]string{
		{"unknown"},
		{"rollback"},
		{"rollback", "--receipt", "run-1", "--tag", "catalog-v1.0.0"},
		{"checkpoint", "--tag", "catalog-v1.0.0", "-m", "missing create"},
		{"checkpoint", "create", "--tag", "catalog-v1.0.0"},
		{"receipt", "delete"},
		{"receipt", "show", "--receipt", "run-1"},
		{"status", "extra"},
	} {
		if _, err := Parse(args); err == nil || !strings.Contains(err.Error(), ErrUsage.Error()) {
			t.Errorf("Parse(%q) error = %v, want usage error", args, err)
		}
	}
}

func TestUsageDocumentsCommandsAndMachineOutput(t *testing.T) {
	usage := Usage()
	for _, text := range []string{"apply", "check", "adopt", "rollback", "checkpoint create", "receipt show", "status", "--json"} {
		if !strings.Contains(usage, text) {
			t.Errorf("usage omits %q: %s", text, usage)
		}
	}
}
