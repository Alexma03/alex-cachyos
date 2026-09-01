package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

func TestDevtoolsPlanMatchesEmbeddedInventoryAndMarkerBlocks(t *testing.T) {
	home := t.TempDir()
	observation := DevtoolsObservation{
		HomeRoot: home,
		ShellFiles: map[string]DevtoolsFileObservation{
			filepath.Join(home, ".zshrc"): {Exists: true, Content: []byte("export EDITOR=nvim\n")},
		},
	}

	requestPlan, err := BuildDevtoolsRequestPlan(observation)
	if err != nil {
		t.Fatalf("BuildDevtoolsRequestPlan() error = %v", err)
	}
	wantPaths := []string{
		filepath.Join(home, ".config", "mise", "config.toml"),
		filepath.Join(home, ".config", "pnpm", "config.yaml"),
		filepath.Join(home, ".npmrc"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".config", "fish", "config.fish"),
	}
	gotPaths := make([]string, 0, len(requestPlan.Files))
	for _, file := range requestPlan.Files {
		gotPaths = append(gotPaths, file.Path)
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("managed paths = %#v, want %#v", gotPaths, wantPaths)
	}
	if got := string(requestPlan.Files[1].Content); !strings.Contains(got, "globalBinDir: "+home+"/.local/bin") || strings.Contains(got, "@HOME@") {
		t.Fatalf("rendered pnpm config = %q", got)
	}
	for _, file := range requestPlan.Files[3:] {
		content := string(file.Content)
		if strings.Count(content, DevtoolsMarkerBegin) != 1 || strings.Count(content, DevtoolsMarkerEnd) != 1 {
			t.Fatalf("marker block for %q = %q", file.Path, content)
		}
	}

	pacman := requestByOperation(t, requestPlan.Requests, "devtools.mise.install")
	if pacman.Executable != "/usr/bin/pacman" || pacman.Scope != runner.ScopeSystem || pacman.Network != runner.NetworkRequired {
		t.Fatalf("mise request = %#v", pacman)
	}
	if !reflect.DeepEqual(pacman.Argv, []string{"-S", "--needed", "--noconfirm", "mise"}) {
		t.Fatalf("mise argv = %#v", pacman.Argv)
	}
	install := requestByOperation(t, requestPlan.Requests, "devtools.toolchain.install")
	if install.Executable != "/usr/bin/mise" || install.Scope != runner.ScopeUser || install.Network != runner.NetworkRequired {
		t.Fatalf("toolchain request = %#v", install)
	}
	for _, request := range requestPlan.Requests {
		if err := runner.ValidateCommandRequest(request); err != nil {
			t.Errorf("request %q invalid: %v", request.Operation, err)
		}
		if request.Executable == "/usr/bin/pkexec" || request.Executable == "sudo" || request.Shell {
			t.Errorf("factory embedded elevation or shell: %#v", request)
		}
	}

	plan, err := BuildDevtoolsPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	if digest, err := planner.Digest(plan); err != nil || digest == "" {
		t.Fatalf("plan digest = %q, error = %v", digest, err)
	}
}

func TestDevtoolsSecondPlanIsSatisfiedAndMarkerRewriteIdempotent(t *testing.T) {
	home := t.TempDir()
	initial := DevtoolsObservation{HomeRoot: home}
	first, err := BuildDevtoolsRequestPlan(initial)
	if err != nil {
		t.Fatal(err)
	}
	converged := DevtoolsObservation{
		HomeRoot:           home,
		InstalledPackages:  map[string]string{"mise": "2026.8.1"},
		ToolchainConverged: true,
		Files:              map[string]DevtoolsFileObservation{},
		ShellFiles:         map[string]DevtoolsFileObservation{},
	}
	for _, file := range first.Files {
		observation := DevtoolsFileObservation{Path: file.Path, Exists: true, SHA256: fileSHA(file.Content), Mode: file.Mode, Content: append([]byte(nil), file.Content...), Ownership: OwnershipCreated}
		if strings.HasSuffix(file.Path, ".zshrc") || strings.HasSuffix(file.Path, ".bashrc") || strings.HasSuffix(file.Path, "config.fish") {
			converged.ShellFiles[file.Path] = observation
		} else {
			converged.Files[file.Path] = observation
		}
	}
	second, err := BuildDevtoolsRequestPlan(converged)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Requests) != 0 {
		t.Fatalf("second requests = %#v", second.Requests)
	}
	for _, step := range second.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("step %q disposition = %s", step.ID, step.Disposition)
		}
	}
	repeated, err := BuildDevtoolsRequestPlan(converged)
	if err != nil || !reflect.DeepEqual(second, repeated) {
		t.Fatalf("repeated plan differs: error=%v", err)
	}
}

func TestDevtoolsGoldenPlan(t *testing.T) {
	plan, err := BuildDevtoolsPlan(DevtoolsObservation{HomeRoot: "/home/alex"})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := planner.Digest(plan)
	if err != nil {
		t.Fatal(err)
	}
	const golden = "010d5519a63cc75e3d35fefde625d798b5d269c3a8e4fbc662604c5b94251085"
	if digest != golden {
		t.Fatalf("devtools plan digest = %q, want golden %q", digest, golden)
	}
}

func fileSHA(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func requestByOperation(t *testing.T, requests []runner.CommandRequest, operation string) runner.CommandRequest {
	t.Helper()
	for _, request := range requests {
		if request.Operation == operation {
			return request
		}
	}
	t.Fatalf("missing request %q in %#v", operation, requests)
	return runner.CommandRequest{}
}
