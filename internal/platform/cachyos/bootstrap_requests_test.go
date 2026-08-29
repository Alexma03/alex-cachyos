package cachyos
import (
	"alex-cachyos/internal/runner"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)
func TestBootstrapRequestsUseEmbeddedListsAndExactDeltas(t *testing.T) {
	input := BootstrapInputs{InstalledPackages: []InstalledPackage{{"zsh", true}, {"paru", false}, {"firefox", true}, {"vim", true}, {"linux-cachyos-lts", true}, {"cachyos-zsh-config", true}}, FirefoxI18N: []string{"firefox-i18n-de"}, Services: []ServiceObservation{{Unit: "ananicy-cpp.service", Installed: true}}, Boot: BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true}}
	p := mustPlan(t, input)
	for _, tc := range []struct {
		op   string
		want []string
	}{{"bootstrap.packages.install", []string{"-Syu", "--needed", "--noconfirm", "cosmic-store", "flatpak", "nano"}}, {"bootstrap.packages.remove", []string{"-Rns", "--noconfirm", "cachyos-zsh-config", "firefox", "firefox-i18n-de", "vim"}}, {"bootstrap.packages.explicit", []string{"-D", "--asexplicit", "cosmic-store", "flatpak", "nano", "paru"}}, {"bootstrap.service.ananicy-cpp", []string{"enable", "--now", "ananicy-cpp.service"}}} {
		if got := request(t, p, tc.op).Argv; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %#v, want %#v", tc.op, got, tc.want)
		}
	}
	chrome := request(t, p, "bootstrap.chrome.install")
	if chrome.Executable != "/usr/bin/paru" || chrome.Scope != runner.ScopeUser || !reflect.DeepEqual(chrome.Argv, []string{"-S", "--needed", "--noconfirm", "google-chrome"}) {
		t.Fatalf("chrome = %#v", chrome)
	}
	boot := request(t, p, "bootstrap.boot.edit")
	if boot.Executable != "/usr/bin/python3" || !reflect.DeepEqual(boot.Argv, []string{"-", "/etc/mkinitcpio.conf", "/etc/default/grub"}) || len(boot.Stdin) == 0 || len(boot.Stdin) > runner.MaxStdinBytes {
		t.Fatalf("boot = %#v", boot)
	}
	if d := p.GRUBPublish; d == nil || d.SourcePath != "/boot/grub/.grub.cfg.alex-cachyos.staged" || d.DestinationPath != "/boot/grub/grub.cfg" || !d.SameDirectory || !d.NoDirectOverwrite || d.GenerationFailure != WarnOnly || d.Fallback != KeepExisting {
		t.Fatalf("GRUB descriptor = %#v", d)
	}
	if got := request(t, p, "bootstrap.boot.grub-generate").Argv; !reflect.DeepEqual(got, []string{"-o", "/boot/grub/.grub.cfg.alex-cachyos.staged"}) {
		t.Fatalf("GRUB = %#v", got)
	}
	if slices.Contains(p.RemovalDelta, "linux-cachyos-lts") || slices.Contains(p.RemovalDelta, "linux-cachyos-lts-headers") {
		t.Fatalf("LTS removal = %#v", p.RemovalDelta)
	}
	q := mustPlan(t, input)
	if !reflect.DeepEqual(p, q) {
		t.Fatal("repeated plans differ")
	}
	q.Requests[0].Argv[0], q.RemovalDelta[0] = "changed", "changed"
	if input.FirefoxI18N[0] != "firefox-i18n-de" || mustPlan(t, input).Requests[0].Argv[0] == "changed" {
		t.Fatal("request plan aliased input or prior output")
	}
	if _, err := BuildBootstrapRequestPlan(BootstrapInputs{FirefoxI18N: []string{"firefox-i18n-de;bad"}}); err == nil {
		t.Fatal("invalid dynamic package accepted")
	}
}
func TestBootScriptPreservesBytesAndIsIdempotent(t *testing.T) {
	mk := "# keep\r\nHOOKS=(base   udev plymouth autodetect) # keep plymouth word\r\nOTHER =  x  \r\n"
	grub := "# heading\nGRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\" # keep splash comment\n# GRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"
	wantMK := "# keep\r\nHOOKS=(base   udev autodetect) # keep plymouth word\r\nOTHER =  x  \r\n"
	wantGRUB := "# heading\nGRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet\" # keep splash comment\n# GRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"
	mk1, grub1, err := runBootScript(t, mk, grub, false)
	if err != nil || string(mk1) != wantMK || string(grub1) != wantGRUB {
		t.Fatalf("first edit = %q / %q, err=%v", mk1, grub1, err)
	}
	mk2, grub2, err := runBootScript(t, string(mk1), string(grub1), false)
	if err != nil || !bytes.Equal(mk1, mk2) || !bytes.Equal(grub1, grub2) {
		t.Fatalf("second edit changed bytes: %q / %q, err=%v", mk2, grub2, err)
	}
}
func TestBootScriptFailsClosedOnMissingOrAmbiguousAssignments(t *testing.T) {
	grub := "GRUB_CMDLINE_LINUX_DEFAULT=\"quiet\"\n"
	for _, tc := range []struct {
		name, mk string
		missing  bool
	}{{"missing assignment", "# HOOKS=(plymouth)\n", false}, {"ambiguous", "HOOKS=(plymouth)\nHOOKS=(base plymouth)\n", false}, {"missing file", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			before := []byte(tc.mk)
			got, _, err := runBootScript(t, tc.mk, grub, tc.missing)
			if err == nil {
				t.Fatal("script succeeded")
			}
			if !tc.missing && !bytes.Equal(got, before) {
				t.Fatalf("failed edit changed bytes: %q", got)
			}
		})
	}
}
func runBootScript(t *testing.T, mk, grub string, missingMk bool) ([]byte, []byte, error) {
	dir := t.TempDir()
	mkPath, grubPath := filepath.Join(dir, "mkinitcpio.conf"), filepath.Join(dir, "grub")
	if !missingMk {
		if err := os.WriteFile(mkPath, []byte(mk), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(grubPath, []byte(grub), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/python3", "-", mkPath, grubPath)
	cmd.Stdin = strings.NewReader(bootEditScript)
	err := cmd.Run()
	read := func(path string) []byte { value, _ := os.ReadFile(path); return value }
	return read(mkPath), read(grubPath), err
}
func mustPlan(t *testing.T, input BootstrapInputs) BootstrapRequestPlan {
	p, err := BuildBootstrapRequestPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func request(t *testing.T, p BootstrapRequestPlan, op string) runner.CommandRequest {
	return p.Requests[slices.IndexFunc(p.Requests, func(r runner.CommandRequest) bool { return r.Operation == op })]
}
func wantedPackages() []InstalledPackage {
	return []InstalledPackage{{"cosmic-store", true}, {"flatpak", true}, {"nano", true}, {"paru", true}, {"zsh", true}}
}
