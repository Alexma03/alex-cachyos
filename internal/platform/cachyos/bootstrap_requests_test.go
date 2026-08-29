package cachyos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"alex-cachyos/internal/runner"
)

func TestBootstrapRequestsUseEmbeddedListsAndExactDeltas(t *testing.T) {
	input := BootstrapInputs{
		InstalledPackages: []InstalledPackage{{"zsh", true}, {"paru", false}, {"firefox", true}, {"vim", true}, {"linux-cachyos-lts", true}, {"cachyos-zsh-config", true}, {"ananicy-cpp", true}},
		FirefoxI18N:       []string{"firefox-i18n-de"},
		Services:          []ServiceObservation{{Unit: "ananicy-cpp.service", Installed: true}},
		Boot:              BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
	}
	p := mustPlan(t, input)
	for _, tc := range []struct {
		op   string
		want []string
	}{
		{"bootstrap.packages.install", []string{"-Syu", "--needed", "--noconfirm", "cosmic-store", "flatpak", "nano"}},
		{"bootstrap.packages.remove", []string{"-Rns", "--noconfirm", "cachyos-zsh-config", "firefox", "firefox-i18n-de", "vim"}},
		{"bootstrap.packages.explicit", []string{"-D", "--asexplicit", "cosmic-store", "flatpak", "nano", "paru"}},
		{"bootstrap.service.ananicy-cpp", []string{"enable", "--now", "ananicy-cpp.service"}},
	} {
		if got := request(t, p, tc.op).Argv; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %#v, want %#v", tc.op, got, tc.want)
		}
	}
	chrome := request(t, p, "bootstrap.chrome.install")
	if chrome.Executable != "/usr/bin/paru" || chrome.Scope != runner.ScopeUser || !reflect.DeepEqual(chrome.Argv, []string{"-S", "--needed", "--noconfirm", "google-chrome"}) {
		t.Fatalf("chrome = %#v", chrome)
	}
	plymouth := request(t, p, bootstrapBootPlymouthEdit)
	if plymouth.Executable != "/usr/bin/python3" || !reflect.DeepEqual(plymouth.Argv, []string{"-", "/etc/mkinitcpio.conf"}) || len(plymouth.Stdin) == 0 || len(plymouth.Stdin) > runner.MaxStdinBytes {
		t.Fatalf("Plymouth edit = %#v", plymouth)
	}
	grubEdit := request(t, p, bootstrapBootGRUBEdit)
	if grubEdit.Executable != "/usr/bin/python3" || !reflect.DeepEqual(grubEdit.Argv, []string{"-", "/etc/default/grub"}) || len(grubEdit.Stdin) == 0 || len(grubEdit.Stdin) > runner.MaxStdinBytes {
		t.Fatalf("GRUB edit = %#v", grubEdit)
	}
	if d := p.GRUBPublish; d == nil || d.SourcePath != "/boot/grub/.grub.cfg.alex-cachyos.staged" || d.DestinationPath != "/boot/grub/grub.cfg" || !d.SameDirectory || !d.NoDirectOverwrite || d.GenerationFailure != WarnOnly || d.Fallback != KeepExisting {
		t.Fatalf("GRUB descriptor = %#v", d)
	}
	if got := request(t, p, bootstrapBootGRUBGenerate).Argv; !reflect.DeepEqual(got, []string{"-o", "/boot/grub/.grub.cfg.alex-cachyos.staged"}) {
		t.Fatalf("GRUB = %#v", got)
	}
	if slices.Contains(plymouth.Argv, "/etc/default/grub") || slices.Contains(grubEdit.Argv, "/etc/mkinitcpio.conf") {
		t.Fatal("boot edit request crossed file boundaries")
	}
	if slices.Contains(p.RemovalDelta, "linux-cachyos-lts") || slices.Contains(p.RemovalDelta, "linux-cachyos-lts-headers") {
		t.Fatalf("LTS removal = %#v", p.RemovalDelta)
	}
	q := mustPlan(t, input)
	if !reflect.DeepEqual(p, q) {
		t.Fatal("repeated plans differ")
	}
	if _, err := BuildBootstrapRequestPlan(BootstrapInputs{FirefoxI18N: []string{"firefox-i18n-de;bad"}}); err == nil {
		t.Fatal("invalid dynamic package accepted")
	}
}

func TestBootstrapRequestsSplitOneSidedBootDeltas(t *testing.T) {
	tests := []struct {
		name    string
		boot    BootObservation
		present []string
		absent  []string
	}{
		{
			name:    "Plymouth only",
			boot:    BootObservation{MkinitcpioHasPlymouth: true},
			present: []string{bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio},
			absent:  []string{bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate},
		},
		{
			name:    "GRUB only",
			boot:    BootObservation{GrubHasSplash: true, GrubGeneratorAvailable: true},
			present: []string{bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate},
			absent:  []string{bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio},
		},
		{
			name:   "neither",
			boot:   BootObservation{},
			absent: []string{bootstrapBootPlymouthEdit, bootstrapBootMkinitcpio, bootstrapBootGRUBEdit, bootstrapBootGRUBGenerate},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := BuildBootstrapRequestPlan(BootstrapInputs{InstalledPackages: wantedPackages(), ChromeInstalled: true, Boot: test.boot})
			if err != nil {
				t.Fatal(err)
			}
			operations := make([]string, len(plan.Requests))
			for i, request := range plan.Requests {
				operations[i] = request.Operation
			}
			for _, operation := range test.present {
				if !slices.Contains(operations, operation) {
					t.Fatalf("missing %s in %v", operation, operations)
				}
			}
			for _, operation := range test.absent {
				if slices.Contains(operations, operation) {
					t.Fatalf("unexpected %s in %v", operation, operations)
				}
			}
			if test.name == "Plymouth only" {
				edit := request(t, plan, bootstrapBootPlymouthEdit)
				if !reflect.DeepEqual(edit.Argv, []string{"-", "/etc/mkinitcpio.conf"}) || plan.GRUBPublish != nil {
					t.Fatalf("Plymouth-only plan crossed into GRUB: request=%#v descriptor=%#v", edit, plan.GRUBPublish)
				}
			} else if test.name == "GRUB only" {
				edit := request(t, plan, bootstrapBootGRUBEdit)
				if !reflect.DeepEqual(edit.Argv, []string{"-", "/etc/default/grub"}) {
					t.Fatalf("GRUB-only edit = %#v", edit)
				}
				if plan.GRUBPublish == nil {
					t.Fatal("GRUB-only plan omitted staged publication descriptor")
				}
			}
		})
	}
}

func TestBootstrapRequestsBlockGRUBPublicationWithoutGenerator(t *testing.T) {
	plan, err := BuildBootstrapRequestPlan(BootstrapInputs{InstalledPackages: wantedPackages(), ChromeInstalled: true, Boot: BootObservation{GrubHasSplash: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(requestOperations(plan), bootstrapBootGRUBEdit) || slices.Contains(requestOperations(plan), bootstrapBootGRUBGenerate) {
		t.Fatalf("GRUB unavailable requests = %v", requestOperations(plan))
	}
	if plan.GRUBPublish == nil || plan.GRUBPublish.GeneratorAvailable {
		t.Fatalf("GRUB unavailable descriptor = %#v", plan.GRUBPublish)
	}
}

func TestBootstrapRequestsPreserveBootScriptBytesAndIsolation(t *testing.T) {
	mk := "# keep\r\nHOOKS=(base   udev plymouth autodetect) # keep plymouth word\r\nOTHER =  x  \r\n"
	grub := "# heading\nGRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\" # keep splash comment\n# GRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"
	mkEdited, err := runBootScript(t, plymouthEditScript, "/etc/mkinitcpio.conf", mk)
	if err != nil {
		t.Fatal(err)
	}
	wantMK := "# keep\r\nHOOKS=(base   udev autodetect) # keep plymouth word\r\nOTHER =  x  \r\n"
	if string(mkEdited) != wantMK {
		t.Fatalf("Plymouth edit = %q, want %q", mkEdited, wantMK)
	}
	grubEdited, err := runBootScript(t, grubEditScript, "/etc/default/grub", grub)
	if err != nil {
		t.Fatal(err)
	}
	wantGRUB := "# heading\nGRUB_TIMEOUT=5\nGRUB_CMDLINE_LINUX=\"\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"quiet\" # keep splash comment\n# GRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"
	if string(grubEdited) != wantGRUB {
		t.Fatalf("GRUB edit = %q, want %q", grubEdited, wantGRUB)
	}
	mkAgain, err := runBootScript(t, plymouthEditScript, "/etc/mkinitcpio.conf", string(mkEdited))
	if err != nil || !bytes.Equal(mkEdited, mkAgain) {
		t.Fatalf("second Plymouth edit changed bytes: %q, err=%v", mkAgain, err)
	}
	grubAgain, err := runBootScript(t, grubEditScript, "/etc/default/grub", string(grubEdited))
	if err != nil || !bytes.Equal(grubEdited, grubAgain) {
		t.Fatalf("second GRUB edit changed bytes: %q, err=%v", grubAgain, err)
	}
}

func TestBootstrapBootRequestLeavesUnrelatedBootFileUntouched(t *testing.T) {
	dir := t.TempDir()
	mkPath := filepath.Join(dir, "mkinitcpio.conf")
	grubPath := filepath.Join(dir, "grub")
	mkBefore := []byte("HOOKS=(base udev plymouth)\n")
	grubBefore := []byte("GRUB_CMDLINE_LINUX_DEFAULT=\"quiet splash\"\n")
	if err := os.WriteFile(mkPath, mkBefore, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grubPath, grubBefore, 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/python3", "-", mkPath)
	cmd.Stdin = strings.NewReader(plymouthEditScript)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	gotGRUB, err := os.ReadFile(grubPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotGRUB, grubBefore) {
		t.Fatalf("Plymouth request touched GRUB file: %q", gotGRUB)
	}
	gotMK, err := os.ReadFile(mkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotMK) != "HOOKS=(base udev)\n" {
		t.Fatalf("Plymouth request output = %q", gotMK)
	}
	cmd = exec.Command("/usr/bin/python3", "-", grubPath)
	cmd.Stdin = strings.NewReader(grubEditScript)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	gotMK, err = os.ReadFile(mkPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotMK) != "HOOKS=(base udev)\n" {
		t.Fatalf("GRUB request touched mkinitcpio file: %q", gotMK)
	}
}

func TestBootstrapBootScriptsFailClosedWithoutTouchingOtherFile(t *testing.T) {
	tests := []struct {
		name   string
		script string
		path   string
		input  string
	}{
		{"Plymouth missing assignment", plymouthEditScript, "/etc/mkinitcpio.conf", "# HOOKS=(plymouth)\n"},
		{"Plymouth ambiguous assignment", plymouthEditScript, "/etc/mkinitcpio.conf", "HOOKS=(plymouth)\nHOOKS=(base plymouth)\n"},
		{"GRUB missing assignment", grubEditScript, "/etc/default/grub", "# GRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"},
		{"GRUB ambiguous assignment", grubEditScript, "/etc/default/grub", "GRUB_CMDLINE_LINUX=\"splash\"\nGRUB_CMDLINE_LINUX_DEFAULT=\"splash\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := []byte(test.input)
			got, err := runBootScript(t, test.script, test.path, test.input)
			if err == nil {
				t.Fatal("script succeeded")
			}
			if !bytes.Equal(got, before) {
				t.Fatalf("failed edit changed bytes: %q", got)
			}
		})
	}
}

func TestBootstrapRequestServiceObservationsRejectDuplicatesAndUsePackageState(t *testing.T) {
	_, err := BuildBootstrapRequestPlan(BootstrapInputs{
		InstalledPackages: append(wantedPackages(), InstalledPackage{Name: "ufw"}),
		Services: []ServiceObservation{
			{Unit: "ufw", Enabled: false, Active: false},
			{Unit: "ufw.service", Enabled: true, Active: false},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "service") {
		t.Fatalf("contradictory service observations error = %v", err)
	}
	plan, err := BuildBootstrapRequestPlan(BootstrapInputs{
		InstalledPackages: append(wantedPackages(), InstalledPackage{Name: "ufw"}),
		Services:          []ServiceObservation{{Unit: "ufw.service", Installed: false, Enabled: false, Active: false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(requestOperations(plan), "bootstrap.service.ufw") {
		t.Fatalf("package-authoritative installed service was not planned: %v", requestOperations(plan))
	}
	plan, err = BuildBootstrapRequestPlan(BootstrapInputs{
		InstalledPackages: wantedPackages(),
		Services:          []ServiceObservation{{Unit: "ufw.service", Installed: true, Enabled: false, Active: false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(requestOperations(plan), "bootstrap.service.ufw") {
		t.Fatalf("absent package service was mutated: %v", requestOperations(plan))
	}
}

func TestBootstrapRequestPlanDeepCopiesMutableResults(t *testing.T) {
	input := BootstrapInputs{
		InstalledPackages: []InstalledPackage{{Name: "zsh", Explicit: true}},
		Boot:              BootObservation{GrubHasSplash: true, GrubGeneratorAvailable: true},
	}
	plan := mustPlan(t, input)
	original := mustPlan(t, input)
	plan.Requests[0].Argv[0] = "changed"
	editIndex := slices.IndexFunc(plan.Requests, func(request runner.CommandRequest) bool { return request.Operation == bootstrapBootGRUBEdit })
	if editIndex < 0 || len(plan.Requests[editIndex].Stdin) == 0 {
		t.Fatal("GRUB edit request did not carry a mutable script buffer")
	}
	plan.Requests[editIndex].Stdin[0] = 'x'
	plan.MissingWanted[0] = "changed"
	plan.GRUBPublish.SourcePath = "changed"
	fresh := mustPlan(t, input)
	if !reflect.DeepEqual(fresh, original) {
		t.Fatalf("fresh request plan changed after returned-value mutation: %#v", fresh)
	}
	if input.InstalledPackages[0].Name != "zsh" {
		t.Fatalf("input package slice was changed: %#v", input.InstalledPackages)
	}
}

func runBootScript(t *testing.T, script, target, content string) ([]byte, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.Base(target))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/usr/bin/python3", "-", path)
	cmd.Stdin = strings.NewReader(script)
	err := cmd.Run()
	read, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, err
	}
	return read, err
}

func mustPlan(t *testing.T, input BootstrapInputs) BootstrapRequestPlan {
	t.Helper()
	plan, err := BuildBootstrapRequestPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func request(t *testing.T, plan BootstrapRequestPlan, operation string) runner.CommandRequest {
	t.Helper()
	index := slices.IndexFunc(plan.Requests, func(request runner.CommandRequest) bool { return request.Operation == operation })
	if index < 0 {
		t.Fatalf("missing request %q in %#v", operation, plan.Requests)
	}
	return plan.Requests[index]
}

func requestOperations(plan BootstrapRequestPlan) []string {
	operations := make([]string, len(plan.Requests))
	for i, request := range plan.Requests {
		operations[i] = request.Operation
	}
	return operations
}

func wantedPackages() []InstalledPackage {
	return []InstalledPackage{{"cosmic-store", true}, {"flatpak", true}, {"nano", true}, {"paru", true}, {"zsh", true}}
}
