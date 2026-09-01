package cachyos

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const testVicinaeStock = "// preserve this comment\nLauncher: \"cosmic-launcher\",\nUnrelated: Keep,\n"

const (
	testVicinaeCommit   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testVicinaePatchSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestVicinaeFactoryUsesExactPackageAndUserServiceRequests(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	requestPlan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatalf("BuildVicinaeRequestPlan() error = %v", err)
	}

	if len(requestPlan.Requests) != 6 {
		t.Fatalf("requests = %#v, want pinned source, local install, and service requests", requestPlan.Requests)
	}
	packageRequest := vicinaeRequest(t, requestPlan.Requests, "vicinae.package.install")
	if packageRequest.Executable != "/usr/bin/paru" || packageRequest.Scope != runner.ScopeUser || packageRequest.Network != runner.NetworkRequired {
		t.Fatalf("package request = %#v", packageRequest)
	}
	sourceDir := filepath.Join(observation.HomeRoot, ".cache", "alex-cachyos", "aur", VicinaePackageName)
	if !reflect.DeepEqual(packageRequest.Argv, []string{"-B", "--install", "--needed", "--noconfirm", sourceDir}) {
		t.Fatalf("package argv = %#v", packageRequest.Argv)
	}
	if packageRequest.Argv[0] == "-S" || containsExactString(packageRequest.Argv, VicinaePackageName) {
		t.Fatalf("package request can resolve mutable AUR head: %#v", packageRequest)
	}
	checkout := vicinaeRequest(t, requestPlan.Requests, "vicinae.package.checkout")
	if checkout.Executable != "/usr/bin/git" || !reflect.DeepEqual(checkout.Argv, []string{"-C", sourceDir, "checkout", "--detach", testVicinaeCommit}) {
		t.Fatalf("checkout request = %#v", checkout)
	}
	verify := vicinaeRequest(t, requestPlan.Requests, "vicinae.package.patch.verify")
	if verify.Executable != "/usr/bin/sha256sum" || verify.Cwd != sourceDir || string(verify.Stdin) != testVicinaePatchSHA+"  .alex-cachyos-source.patch\n" {
		t.Fatalf("verify request = %#v", verify)
	}
	installStep := planStepByID(t, requestPlan.Steps, VicinaePackageInstallOperation)
	if !reflect.DeepEqual(installStep.DependsOn, []string{"vicinae.package.patch.verify"}) {
		t.Fatalf("install dependencies = %#v", installStep.DependsOn)
	}

	serviceRequest := vicinaeRequest(t, requestPlan.Requests, "vicinae.service.enable")
	if serviceRequest.Executable != "/usr/bin/systemctl" || serviceRequest.Scope != runner.ScopeUser || serviceRequest.Network != runner.NetworkNone {
		t.Fatalf("service request = %#v", serviceRequest)
	}
	if !reflect.DeepEqual(serviceRequest.Argv, []string{"--user", "enable", "--now", "vicinae.service"}) {
		t.Fatalf("service argv = %#v", serviceRequest.Argv)
	}
	for _, request := range requestPlan.Requests {
		if err := runner.ValidateCommandRequest(request); err != nil {
			t.Errorf("request %q failed validation: %v", request.Operation, err)
		}
		if request.Executable == "sudo" || strings.Contains(strings.Join(request.Argv, "\x00"), "sudo") || request.Shell {
			t.Errorf("unsafe request = %#v", request)
		}
	}

	module, err := BuildVicinaeModule(observation)
	if err != nil {
		t.Fatalf("BuildVicinaeModule() error = %v", err)
	}
	if module.Name != "vicinae" || !module.Enabled {
		t.Fatalf("module = %#v", module)
	}
	if got := vicinaeStep(t, module, "vicinae.package.install").Disposition; got != planner.DispositionApply {
		t.Fatalf("package disposition = %s", got)
	}
	if got := vicinaeStep(t, module, "vicinae.service.enable").Disposition; got != planner.DispositionApply {
		t.Fatalf("service disposition = %s", got)
	}
}

func TestVicinaePlanCarriesCompleteMetadataAndOwnershipBoundFileInverses(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	requestPlan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatalf("BuildVicinaeRequestPlan() error = %v", err)
	}

	wantEnvironment := filepath.Join("/etc", "environment.d", "99-vicinae-cosmic.conf")
	wantShortcutDir := filepath.Join(observation.HomeRoot, ".config", "cosmic", "com.system76.CosmicSettings.Shortcuts", "v1")
	wantCustom := filepath.Join(wantShortcutDir, "custom")
	wantSystemActions := filepath.Join(wantShortcutDir, "system_actions")
	if requestPlan.Environment.Path != wantEnvironment || requestPlan.CustomShortcut.Path != wantCustom || requestPlan.SystemActions.Path != wantSystemActions {
		t.Fatalf("desired files = environment %q, custom %q, system_actions %q", requestPlan.Environment.Path, requestPlan.CustomShortcut.Path, requestPlan.SystemActions.Path)
	}
	if requestPlan.Environment.Mode != 0644 || requestPlan.CustomShortcut.Mode != 0644 || requestPlan.SystemActions.Mode != 0644 {
		t.Fatalf("desired modes = %#v", requestPlan.Files)
	}
	if !bytes.Equal(requestPlan.Environment.Content, []byte("# Required for Vicinae clipboard history on COSMIC (wlr-data-control).\n# Takes effect after logout/login (or reboot).\nCOSMIC_DATA_CONTROL_ENABLED=1\n")) {
		t.Fatalf("environment bytes = %q", requestPlan.Environment.Content)
	}
	if !strings.Contains(string(requestPlan.CustomShortcut.Content), "Spawn(\"vicinae vicinae://launch/clipboard/history?toggle=true\")") {
		t.Fatalf("custom shortcut bytes = %q", requestPlan.CustomShortcut.Content)
	}
	if !strings.Contains(string(requestPlan.SystemActions.Content), "Launcher: \"vicinae toggle\",") || strings.Contains(string(requestPlan.SystemActions.Content), "Unrelated: Keep,") == false {
		t.Fatalf("rewritten system actions = %q", requestPlan.SystemActions.Content)
	}

	module := moduleFromRequestPlan(t, observation)
	for _, file := range requestPlan.Files {
		step := vicinaeStep(t, module, vicinaeFileStepID(file))
		if step.Inverse == nil {
			t.Fatalf("file %q has no inverse", file.Path)
		}
		if step.Inverse.Operation != planner.Operation("remove-file") {
			if step.Inverse.Operation != planner.Operation("restore-backup") {
				t.Fatalf("file %q inverse = %#v", file.Path, step.Inverse)
			}
		}
	}

	packageStep := vicinaeStep(t, module, "vicinae.package.install")
	var desired map[string]any
	if err := json.Unmarshal(packageStep.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	requestIdentity, ok := desired["request"].(map[string]any)
	if !ok {
		t.Fatalf("package desired metadata = %#v", desired)
	}
	for _, key := range []string{"operation", "executable", "argv", "cwd", "network", "scope", "timeout", "outputPolicy", "outputLimit", "stdinSha256"} {
		if _, ok := requestIdentity[key]; !ok {
			t.Errorf("package request identity lacks %q: %#v", key, requestIdentity)
		}
	}
	if _, ok := desired["file"]; ok {
		t.Fatalf("package step unexpectedly has file metadata: %#v", desired)
	}

	plan, err := BuildVicinaePlan(observation)
	if err != nil {
		t.Fatalf("BuildVicinaePlan() error = %v", err)
	}
	if len(plan.Steps) != len(module.Steps) {
		t.Fatalf("plan steps = %d, module steps = %d", len(plan.Steps), len(module.Steps))
	}
	if _, err := planner.Digest(plan); err != nil {
		t.Fatalf("planner digest error = %v", err)
	}
}

func TestVicinaeRejectsWrongPackageEvenWhenVicinaeIsInstalled(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	observation.Package = VicinaePackageObservation{Name: "vicinae", Version: "1.0", Installed: true}
	requestPlan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findVicinaeRequest(requestPlan.Requests, "vicinae.package.install"); err != nil {
		t.Fatalf("wrong package was treated as satisfying desired package: %v", err)
	}
}

func TestVicinaeRewriteIsStrictIdempotentAndBytePreserving(t *testing.T) {
	stock := []byte("prefix\r\nLauncher: \"cosmic-launcher\", // replace only this value\r\nOther: \"cosmic-launcher\",\r\n// Launcher: \"cosmic-launcher\",\r\nsuffix")
	got, err := RewriteCosmicSystemActions(stock)
	if err != nil {
		t.Fatalf("RewriteCosmicSystemActions() error = %v", err)
	}
	want := []byte("prefix\r\nLauncher: \"vicinae toggle\", // replace only this value\r\nOther: \"cosmic-launcher\",\r\n// Launcher: \"cosmic-launcher\",\r\nsuffix")
	if !bytes.Equal(got, want) {
		t.Fatalf("rewritten bytes = %q, want %q", got, want)
	}
	stock[0] = 'X'
	if got[0] != 'p' {
		t.Fatal("rewrite result aliases input bytes")
	}
	again, err := RewriteCosmicSystemActions(got)
	if err != nil || !bytes.Equal(again, got) {
		t.Fatalf("idempotent rewrite = %q, error = %v", again, err)
	}

	for name, input := range map[string][]byte{
		"missing":      []byte("Other: Keep,\n"),
		"duplicate":    []byte("Launcher: \"cosmic-launcher\",\nLauncher: \"cosmic-launcher\",\n"),
		"unsupported":  []byte("Launcher: \"other-launcher\",\n"),
		"malformed":    []byte("Launcher: cosmic-launcher,\n"),
		"invalid-utf8": []byte{0xff, 'L', 'a', 'u', 'n', 'c', 'h', 'e', 'r', ':', ' ', '"', 'x', '"', ','},
	} {
		t.Run(name, func(t *testing.T) {
			before := append([]byte(nil), input...)
			got, err := RewriteCosmicSystemActions(input)
			if err == nil {
				t.Fatal("invalid system_actions unexpectedly accepted")
			}
			if got != nil && !bytes.Equal(got, before) {
				t.Fatalf("failed rewrite changed bytes = %q", got)
			}
			if name == "missing" && !errors.Is(err, ErrVicinaeLauncherMissing) {
				t.Fatalf("missing error = %v", err)
			}
			if name == "duplicate" && !errors.Is(err, ErrVicinaeLauncherAmbiguous) {
				t.Fatalf("duplicate error = %v", err)
			}
		})
	}
}

func TestVicinaeRemovalRestoresSystemActionsEvenWhenStockIsMissing(t *testing.T) {
	observation := newVicinaeObservation(t, nil)
	observation.StockSystemActions = VicinaeStockSystemActionsObservation{}
	observation.Remove = true
	observation.SystemActions = VicinaeFileObservation{
		Path:      filepath.Join(observation.HomeRoot, ".config", "cosmic", "com.system76.CosmicSettings.Shortcuts", "v1", "system_actions"),
		Exists:    true,
		SHA256:    strings.Repeat("a", 64),
		Mode:      0644,
		Ownership: OwnershipCreated,
	}

	plan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatalf("missing stock removal plan error = %v", err)
	}
	if plan.SystemActions.Path == "" {
		t.Fatal("missing stock removal lost the system_actions target")
	}
	step := planStepByID(t, plan.Steps, "vicinae.shortcuts.system-actions")
	if step.Disposition != planner.DispositionRemove {
		t.Fatalf("system_actions disposition = %s, want remove", step.Disposition)
	}
	var desired map[string]any
	if err := json.Unmarshal(step.Desired, &desired); err != nil {
		t.Fatal(err)
	}
	if desired["removalOperation"] != "remove-file" {
		t.Fatalf("system_actions removal metadata = %#v", desired)
	}
}

func TestVicinaeMissingStockActionIsWarnOnly(t *testing.T) {
	observation := newVicinaeObservation(t, nil)
	observation.StockSystemActions = VicinaeStockSystemActionsObservation{}
	requestPlan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatalf("missing stock action should warn only: %v", err)
	}
	if len(requestPlan.Warnings) == 0 || !strings.Contains(strings.Join(requestPlan.Warnings, "\n"), "stock") {
		t.Fatalf("warnings = %#v", requestPlan.Warnings)
	}
	if requestPlan.SystemActions.Path != "" || strings.Contains(strings.Join(requestPlan.Warnings, "\n"), "blocked") {
		t.Fatalf("missing stock action produced a mutating target: %#v", requestPlan)
	}
	if _, err := findVicinaeRequest(requestPlan.Requests, "vicinae.package.install"); err != nil {
		t.Fatalf("other module work was lost with missing stock action: %v", err)
	}
}

func TestVicinaeRemovalRetainsPackageAndEnvironmentAndDistinguishesReceiptRollback(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	plan, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	observation.Remove = true
	observation.Package = VicinaePackageObservation{Name: VicinaePackageName, Version: "1.2.3", Installed: true}
	observation.Service = VicinaeServiceObservation{Unit: VicinaeServiceUnit, Exists: true, Enabled: true, Active: true}
	observation.Environment = observedVicinaeFile(plan.Environment, OwnershipCreated, "")
	observation.CustomShortcut = observedVicinaeFile(plan.CustomShortcut, OwnershipAdopted, plan.CustomShortcut.Path+".bak.alex-cachyos")
	observation.SystemActions = observedVicinaeFile(plan.SystemActions, OwnershipAdopted, plan.SystemActions.Path+".bak.alex-cachyos")

	removal, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatalf("removal plan error = %v", err)
	}
	if len(removal.Requests) != 1 {
		t.Fatalf("removal requests = %#v", removal.Requests)
	}
	service := vicinaeRequest(t, removal.Requests, "vicinae.service.disable")
	if !reflect.DeepEqual(service.Argv, []string{"--user", "disable", "--now", VicinaeServiceUnit}) {
		t.Fatalf("removal service argv = %#v", service.Argv)
	}
	if _, err := findVicinaeRequest(removal.Requests, "vicinae.package.remove"); err == nil {
		t.Fatal("module removal attempted to remove package")
	}
	if _, err := findVicinaeRequest(removal.Requests, "vicinae.environment.remove"); err == nil {
		t.Fatal("module removal attempted to remove environment file")
	}

	module, err := BuildVicinaeModule(observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"vicinae.package.install", "vicinae.file.environment"} {
		step := vicinaeStep(t, module, id)
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("retained %s disposition = %s", id, step.Disposition)
		}
	}
	for _, test := range []struct {
		id        string
		operation string
	}{
		{id: "vicinae.shortcuts.custom", operation: "restore-backup"},
		{id: "vicinae.shortcuts.system-actions", operation: "restore-backup"},
	} {
		step := vicinaeStep(t, module, test.id)
		if step.Disposition != planner.DispositionRemove {
			t.Fatalf("%s disposition = %s", test.id, step.Disposition)
		}
		var desired map[string]any
		if err := json.Unmarshal(step.Desired, &desired); err != nil {
			t.Fatal(err)
		}
		if desired["action"] != "module-remove" || desired["removalOperation"] != test.operation {
			t.Fatalf("%s desired removal metadata = %#v", test.id, desired)
		}
		if step.Inverse == nil || step.Inverse.Operation != planner.Operation("vicinae.file.reapply") {
			t.Fatalf("%s inverse = %#v", test.id, step.Inverse)
		}
		var inverse map[string]any
		if err := json.Unmarshal(step.Inverse.Value, &inverse); err != nil {
			t.Fatal(err)
		}
		if inverse["action"] != "receipt-rollback" {
			t.Fatalf("%s inverse action = %#v", test.id, inverse)
		}
	}
	serviceStep := vicinaeStep(t, module, "vicinae.service.disable")
	var inverse map[string]any
	if err := json.Unmarshal(serviceStep.Inverse.Value, &inverse); err != nil {
		t.Fatal(err)
	}
	if inverse["enabled"] != true || inverse["active"] != true || inverse["exists"] != true {
		t.Fatalf("service prior state inverse = %#v", inverse)
	}
}

func TestVicinaeFactoryConvergedSecondPlanHasNoMutations(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	first, err := BuildVicinaeRequestPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	converged := observation
	converged.Package = VicinaePackageObservation{Name: VicinaePackageName, Version: "1.2.3", Installed: true}
	converged.Service = VicinaeServiceObservation{Unit: VicinaeServiceUnit, Exists: true, Enabled: true, Active: true}
	converged.Environment = observedVicinaeFile(first.Environment, OwnershipCreated, "")
	converged.CustomShortcut = observedVicinaeFile(first.CustomShortcut, OwnershipCreated, "")
	converged.SystemActions = observedVicinaeFile(first.SystemActions, OwnershipCreated, "")
	second, err := BuildVicinaeRequestPlan(converged)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Requests) != 0 {
		t.Fatalf("converged requests = %#v", second.Requests)
	}
	module, err := BuildVicinaeModule(converged)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range module.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("converged step %s disposition = %s", step.ID, step.Disposition)
		}
	}
	repeated, err := BuildVicinaeRequestPlan(converged)
	if err != nil || !reflect.DeepEqual(second, repeated) {
		t.Fatalf("repeated converged plans differ: second=%#v repeated=%#v error=%v", second, repeated, err)
	}
}

func TestVicinaeFactoryRejectsInvalidObservationAndCopiesOutputs(t *testing.T) {
	observation := newVicinaeObservation(t, []byte(testVicinaeStock))
	observation.HomeRoot = "relative/home"
	if _, err := BuildVicinaeModule(observation); !errors.Is(err, ErrInvalidVicinaePaths) {
		t.Fatalf("invalid home error = %v", err)
	}

	observation = newVicinaeObservation(t, []byte(testVicinaeStock))
	observation.Environment = VicinaeFileObservation{Exists: true, SHA256: "not-a-sha", Mode: 0644, Ownership: OwnershipAdopted, BackupPath: "/tmp/backup"}
	if _, err := BuildVicinaeModule(observation); !errors.Is(err, ErrInvalidVicinaeObservation) {
		t.Fatalf("invalid target observation error = %v", err)
	}

	assets, err := LoadVicinaeAssets()
	if err != nil {
		t.Fatal(err)
	}
	assets.Environment[0] ^= 0xff
	fresh, err := LoadVicinaeAssets()
	if err != nil || fresh.Environment[0] == assets.Environment[0] {
		t.Fatal("embedded Vicinae assets are not copied")
	}
}

func newVicinaeObservation(t *testing.T, stock []byte) VicinaeObservation {
	t.Helper()
	return VicinaeObservation{
		HomeRoot: t.TempDir(),
		Catalog: &catalog.Catalog{Pins: &catalog.Pins{
			AURLocal: map[string]catalog.AURLocalPin{
				VicinaePackageName: {SourceCommit: testVicinaeCommit, PatchSHA256: testVicinaePatchSHA},
			},
		}},
		StockSystemActions: VicinaeStockSystemActionsObservation{
			Exists:  stock != nil,
			Content: append([]byte(nil), stock...),
			Mode:    0644,
		},
	}
}

func containsExactString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func observedVicinaeFile(file ManagedFileDescriptor, ownership FileOwnershipKind, backup string) VicinaeFileObservation {
	return VicinaeFileObservation{
		Path:       file.Path,
		Exists:     true,
		SHA256:     sha256Hex(file.Content),
		Mode:       file.Mode,
		Ownership:  ownership,
		BackupPath: backup,
	}
}

func vicinaeFileStepID(file ManagedFileDescriptor) string {
	switch file.Kind {
	case "environment":
		return "vicinae.file.environment"
	case "custom":
		return "vicinae.shortcuts.custom"
	case "system-actions":
		return "vicinae.shortcuts.system-actions"
	default:
		return ""
	}
}

func moduleFromRequestPlan(t *testing.T, observation VicinaeObservation) planner.Module {
	t.Helper()
	module, err := BuildVicinaeModule(observation)
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func vicinaeStep(t *testing.T, module planner.Module, id string) planner.Step {
	t.Helper()
	for _, step := range module.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("missing Vicinae step %q in %#v", id, module.Steps)
	return planner.Step{}
}

func planStepByID(t *testing.T, steps []planner.Step, id string) planner.Step {
	t.Helper()
	for _, step := range steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("missing plan step %q in %#v", id, steps)
	return planner.Step{}
}

func vicinaeRequest(t *testing.T, requests []runner.CommandRequest, operation string) runner.CommandRequest {
	t.Helper()
	request, err := findVicinaeRequest(requests, operation)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func findVicinaeRequest(requests []runner.CommandRequest, operation string) (runner.CommandRequest, error) {
	for _, request := range requests {
		if request.Operation == operation {
			return request, nil
		}
	}
	return runner.CommandRequest{}, errors.New("request not found: " + operation)
}
