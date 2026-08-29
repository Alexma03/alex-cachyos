package cli

import "testing"

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
