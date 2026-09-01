package cli

import (
	"strings"
	"testing"
)

func TestPreservedFlagsAndCheckSelection(t *testing.T) {
	for _, name := range []string{"--host", "--only", "--with", "--without", "--remove", "--dry-run", "--check", "--list", "-h", "--help"} {
		if !strings.Contains(Usage(), name) {
			t.Errorf("help omits %s", name)
		}
	}
	got, err := Parse([]string{"--check"})
	if err != nil || !got.Check || len(got.Only) != 1 || got.Only[0] != "verify" {
		t.Fatalf("--check = %#v, %v; want verify-only", got, err)
	}
	if _, err = Parse([]string{"--unknown"}); err == nil || ExitCode(err) != ExitUsage {
		t.Fatalf("unknown flag error = %v, code %d; want usage %d", err, ExitCode(err), ExitUsage)
	}
}

func TestParseIntegrationTargetSeparatelyFromHost(t *testing.T) {
	got, err := Parse([]string{"--host", "portable-synthetic", "--integration-target", "portable-synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "portable-synthetic" || got.IntegrationTarget != "portable-synthetic" {
		t.Fatalf("host/target = %q/%q", got.Host, got.IntegrationTarget)
	}
	if !strings.Contains(Usage(), "--integration-target HOST") {
		t.Fatalf("usage does not document integration target: %q", Usage())
	}
}
