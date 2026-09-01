package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const (
	testFingerprintCommit   = "8749008832ee1f313bfca4d3c04340df84b2bc27"
	testFingerprintPatch    = "dd248cb9225857385f32ce36da887cb039d5ec7bffd54f412a790e898349d03d"
	testFingerprintPKGBUILD = "094cdd3f61a0227c7eec5c2426a20885a35cf2bd23e39d713e903072a02797bf"
	testFingerprintArtifact = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestFingerprintBuildVerifiesResolvedPinsBeforeLocalPacmanInstall(t *testing.T) {
	observation := fingerprintFixture(t, false)
	plan, err := BuildFingerprintPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	wantOperations := []string{
		fingerprintBuildRootOperation,
		fingerprintPKGBUILDWriteOperation,
		fingerprintPatchWriteOperation,
		fingerprintInputsVerifyOperation,
		fingerprintBuildOperation,
		fingerprintArtifactVerifyOperation,
		fingerprintInstallOperation,
	}
	if got := fingerprintRequestOperations(plan.Requests); !equalStrings(got, wantOperations) {
		t.Fatalf("request operations = %#v, want %#v", got, wantOperations)
	}
	for _, request := range plan.Requests {
		if err := runner.ValidateCommandRequest(request); err != nil {
			t.Fatalf("invalid typed request %q: %v", request.Operation, err)
		}
		if request.Shell || strings.Contains(request.Executable, "sudo") || strings.Contains(request.Executable, "paru") {
			t.Fatalf("unsafe request escaped typed boundary: %#v", request)
		}
	}
	install := requestByOperation(t, plan.Requests, fingerprintInstallOperation)
	artifact := filepath.Join(observation.BuildRoot, observation.Pin.ArtifactName)
	if install.Executable != "/usr/bin/pacman" || !equalStrings(install.Argv, []string{"-U", "--needed", "--noconfirm", artifact}) {
		t.Fatalf("install request = %#v", install)
	}
	if install.Network != runner.NetworkNone || install.Scope != runner.ScopeSystem {
		t.Fatalf("install policy = scope %q network %q", install.Scope, install.Network)
	}
	installStep := stepByID(planner.Module{Steps: plan.Steps}, fingerprintInstallOperation)
	desired := decodeDesired(t, *installStep)
	if desired["sourceCommit"] != testFingerprintCommit || desired["pkgrel"] != float64(1) || desired["ownership"] != "explicit" || desired["transactionPolicy"] != "verified-local-package-only" || desired["implicitUpgradeDowngrade"] != false {
		t.Fatalf("install convergence metadata = %#v", desired)
	}
	versionChange, ok := desired["versionChange"].(map[string]any)
	if !ok || versionChange["to"] == nil {
		t.Fatalf("install version change = %#v", desired["versionChange"])
	}
	verifyIndex := operationIndex(plan.Requests, fingerprintArtifactVerifyOperation)
	installIndex := operationIndex(plan.Requests, fingerprintInstallOperation)
	if verifyIndex < 0 || installIndex != verifyIndex+1 {
		t.Fatalf("artifact verification must immediately precede install: %#v", fingerprintRequestOperations(plan.Requests))
	}
	build := requestByOperation(t, plan.Requests, fingerprintBuildOperation)
	if build.Executable != "/usr/bin/makepkg" || build.Env["SOURCE_DATE_EPOCH"] != observation.Pin.SourceDateEpoch {
		t.Fatalf("deterministic build request = %#v", build)
	}
	if got := string(requestByOperation(t, plan.Requests, fingerprintInputsVerifyOperation).Stdin); !strings.Contains(got, testFingerprintPKGBUILD+"  PKGBUILD") || !strings.Contains(got, testFingerprintPatch+"  "+fingerprintPatchName) {
		t.Fatalf("input checksum manifest = %q", got)
	}
	ordered, err := planner.BuildPlan([]planner.Module{{Name: fingerprintModuleName, Enabled: true, Steps: plan.Steps}}, planner.Selection{})
	if err != nil {
		t.Fatalf("fingerprint dependency graph is invalid: %v", err)
	}
	if got := ordered.Steps[len(ordered.Steps)-1].ID; got != "fingerprint.pam.system-local-login" {
		t.Fatalf("last ordered step = %q", got)
	}

	if len(plan.Files) != 7 {
		t.Fatalf("PAM file count = %d, want 7", len(plan.Files))
	}
	contents := make(map[string]string, len(plan.Files))
	for _, file := range plan.Files {
		contents[file.Path] = string(file.Content)
		if file.Mode != 0o644 {
			t.Fatalf("PAM mode for %s = %#o", file.Path, file.Mode)
		}
	}
	for path, content := range contents {
		if pamHasSufficientFingerprint(content) {
			continue
		}
		if !strings.Contains(content, "auth       include      system-local-login") || !pamHasSufficientFingerprint(contents["/etc/pam.d/system-local-login"]) {
			t.Fatalf("PAM overlay %s lacks direct or exact included sufficient fingerprint auth", path)
		}
	}
}

func TestFingerprintRefusesMissingOrUnsafeResolvedPins(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FingerprintObservation)
	}{
		{"missing pin", func(o *FingerprintObservation) { o.Pin = nil }},
		{"short source commit", func(o *FingerprintObservation) { o.Pin.SourceCommit = "deadbeef" }},
		{"wrong embedded source commit", func(o *FingerprintObservation) { o.Pin.SourceCommit = strings.Repeat("1", 40) }},
		{"zero pkgrel", func(o *FingerprintObservation) { o.Pin.Pkgrel = 0 }},
		{"wrong embedded patch checksum", func(o *FingerprintObservation) { o.Pin.PatchSHA256 = strings.Repeat("b", 64) }},
		{"unsafe artifact name", func(o *FingerprintObservation) { o.Pin.ArtifactName = "../driver.pkg.tar.zst" }},
		{"artifact shell metacharacter", func(o *FingerprintObservation) { o.Pin.ArtifactName = fingerprintPackageName + "-$(id).pkg.tar.zst" }},
		{"unsafe build root", func(o *FingerprintObservation) { o.BuildRoot = "/fixture/build/../escape" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observation := fingerprintFixture(t, false)
			test.mutate(&observation)
			if _, err := BuildFingerprintPlan(observation); !errors.Is(err, ErrInvalidFingerprintObservation) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidFingerprintObservation)
			}
		})
	}
}

func TestFingerprintConvergedPackageAndPAMProduceNoMutatorRequests(t *testing.T) {
	observation := fingerprintFixture(t, true)
	plan, err := BuildFingerprintPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Requests) != 0 {
		t.Fatalf("converged plan emitted requests: %#v", fingerprintRequestOperations(plan.Requests))
	}
	for _, step := range plan.Steps {
		if step.Disposition != planner.DispositionSatisfied {
			t.Fatalf("converged step %q disposition = %q", step.ID, step.Disposition)
		}
	}
}

func TestFingerprintDefaultDenyDoesNotConsultUnsafePin(t *testing.T) {
	policy := galaxyPolicy("fingerprint", catalog.RiskFingerprintPAM, false)
	evidence := readyEvidence()
	evidence.Capabilities[catalog.RiskFingerprintPAM] = CapabilityEvidence{State: EvidenceReady}
	evidence.Fingerprint = FingerprintObservation{Pin: &FingerprintResolvedPin{SourceCommit: "unsafe"}}
	modules, err := BuildModules(policy, evidence)
	if err != nil {
		t.Fatal(err)
	}
	module := moduleNamed(t, modules, "fingerprint")
	if len(module.Steps) != 0 {
		t.Fatalf("default-denied fingerprint emitted steps: %#v", module.Steps)
	}
}

func TestFingerprintPAMRollbackRequiresExactOwnershipEvidence(t *testing.T) {
	created := fingerprintFixture(t, false)
	created.PAM["/etc/pam.d/sudo"] = FingerprintFileObservation{Ownership: OwnershipCreated}
	plan, err := BuildFingerprintPlan(created)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepByID(planner.Module{Steps: plan.Steps}, "fingerprint.pam.sudo").Inverse.Operation; got != "remove-file" {
		t.Fatalf("created inverse = %q", got)
	}

	adopted := fingerprintFixture(t, false)
	adopted.PAM["/etc/pam.d/sudo"] = FingerprintFileObservation{Ownership: OwnershipAdopted, BackupPath: "/etc/pam.d/sudo.bak.alex-cachyos"}
	plan, err = BuildFingerprintPlan(adopted)
	if err != nil {
		t.Fatal(err)
	}
	if got := stepByID(planner.Module{Steps: plan.Steps}, "fingerprint.pam.sudo").Inverse.Operation; got != "restore-backup" {
		t.Fatalf("adopted inverse = %q", got)
	}

	invalid := fingerprintFixture(t, false)
	invalid.PAM["/etc/pam.d/sudo"] = FingerprintFileObservation{Ownership: OwnershipAdopted, BackupPath: "/etc/pam.d/other.bak"}
	if _, err := BuildFingerprintPlan(invalid); !errors.Is(err, ErrInvalidFileOwnership) {
		t.Fatalf("invalid rollback evidence error = %v", err)
	}
}

func TestFingerprintPackageReplacementRequiresVerifiedRollbackArtifact(t *testing.T) {
	observation := fingerprintFixture(t, false)
	observation.Package = FingerprintPackageObservation{Installed: true, Name: "libfprint", SourceCommit: "repository", Pkgrel: 2}
	if _, err := BuildFingerprintPlan(observation); !errors.Is(err, ErrInvalidFingerprintObservation) {
		t.Fatalf("missing package rollback evidence error = %v", err)
	}

	observation.Package.RollbackArtifactPath = "/fixture/cache/libfprint-1.94.9-2-x86_64.pkg.tar.zst"
	observation.Package.RollbackArtifactSHA256 = strings.Repeat("c", 64)
	plan, err := BuildFingerprintPlan(observation)
	if err != nil {
		t.Fatal(err)
	}
	step := stepByID(planner.Module{Steps: plan.Steps}, fingerprintInstallOperation)
	if step == nil || step.Inverse == nil || step.Inverse.Operation != "fingerprint.package.restore-local" {
		t.Fatalf("replacement inverse = %#v", step)
	}
	var inverse map[string]any
	if err := json.Unmarshal(step.Inverse.Value, &inverse); err != nil {
		t.Fatal(err)
	}
	if inverse["artifact"] != observation.Package.RollbackArtifactPath || inverse["sha256"] != observation.Package.RollbackArtifactSHA256 {
		t.Fatalf("replacement inverse evidence = %#v", inverse)
	}
}

func fingerprintFixture(t *testing.T, converged bool) FingerprintObservation {
	t.Helper()
	observation := FingerprintObservation{
		BuildRoot: "/fixture/build/fingerprint",
		Pin: &FingerprintResolvedPin{
			SourceCommit:    testFingerprintCommit,
			Pkgrel:          1,
			PKGBUILDSHA256:  testFingerprintPKGBUILD,
			PatchSHA256:     testFingerprintPatch,
			ArtifactName:    "libfprint-egismoc-sdcp-git-r100.8749008-1-x86_64.pkg.tar.zst",
			ArtifactSHA256:  testFingerprintArtifact,
			SourceDateEpoch: "1725148800",
		},
		PAM: make(map[string]FingerprintFileObservation),
	}
	for _, name := range fingerprintPAMNames {
		path := filepath.Join("/etc/pam.d", name)
		observation.PAM[path] = FingerprintFileObservation{Ownership: OwnershipCreated}
	}
	if !converged {
		return observation
	}
	observation.Package = FingerprintPackageObservation{Installed: true, Name: fingerprintPackageName, SourceCommit: testFingerprintCommit, Pkgrel: 1}
	for _, name := range fingerprintPAMNames {
		path := filepath.Join("/etc/pam.d", name)
		file := fingerprintPAMFile(t, name)
		digest := sha256.Sum256(file)
		observation.PAM[path] = FingerprintFileObservation{Path: path, Exists: true, SHA256: hex.EncodeToString(digest[:]), Mode: 0o644, Ownership: OwnershipAdopted, BackupPath: path + ".bak.alex-cachyos"}
	}
	return observation
}

func fingerprintPAMFile(t *testing.T, name string) []byte {
	t.Helper()
	for _, file := range mustFingerprintPAMFiles(t) {
		if filepath.Base(file.Path) == name {
			return file.Content
		}
	}
	t.Fatalf("missing fingerprint PAM fixture %q", name)
	return nil
}

func mustFingerprintPAMFiles(t *testing.T) []ManagedFileDescriptor {
	t.Helper()
	files, err := loadFingerprintPAMFiles()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func pamHasSufficientFingerprint(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "auth" && fields[1] == "sufficient" && fields[2] == "pam_fprintd.so" {
			return true
		}
	}
	return false
}

func fingerprintRequestOperations(requests []runner.CommandRequest) []string {
	operations := make([]string, len(requests))
	for index, request := range requests {
		operations[index] = request.Operation
	}
	return operations
}

func operationIndex(requests []runner.CommandRequest, operation string) int {
	for index, request := range requests {
		if request.Operation == operation {
			return index
		}
	}
	return -1
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func decodeDesired(t *testing.T, step planner.Step) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(step.Desired, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
