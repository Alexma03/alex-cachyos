package cachyos

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"alex-cachyos/internal/assets"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/planner"
	"alex-cachyos/internal/runner"
)

const (
	fingerprintModuleName              = "fingerprint"
	fingerprintPackageName             = "libfprint-egismoc-sdcp-git"
	fingerprintPackagingRoot           = "data/packaging/libfprint-egismoc-sdcp-git"
	fingerprintPatchName               = "0001-egismoc-drop-sdcp-claim-on-close.patch"
	fingerprintBuildRootOperation      = "fingerprint.build.root"
	fingerprintPKGBUILDWriteOperation  = "fingerprint.build.pkgbuild.write"
	fingerprintPatchWriteOperation     = "fingerprint.build.patch.write"
	fingerprintInputsVerifyOperation   = "fingerprint.build.inputs.verify"
	fingerprintBuildOperation          = "fingerprint.package.build"
	fingerprintArtifactVerifyOperation = "fingerprint.package.artifact.verify"
	fingerprintInstallOperation        = "fingerprint.package.install"
	fingerprintPAMStep                 = "fingerprint.pam.sudo"
)

var (
	ErrInvalidFingerprintObservation = errors.New("invalid fingerprint observation")
	fingerprintPAMNames              = []string{"cosmic-greeter", "greetd", "polkit-1", "su", "su-l", "sudo", "system-local-login"}
)

// FingerprintResolvedPin is resolved authority supplied by a fixture or a
// future production catalog adapter. It binds build inputs and the exact local
// package artifact; runtime observations never fill missing fields.
type FingerprintResolvedPin struct {
	SourceCommit    string
	Pkgrel          int
	PKGBUILDSHA256  string
	PatchSHA256     string
	ArtifactName    string
	ArtifactSHA256  string
	SourceDateEpoch string
}

type FingerprintPackageObservation struct {
	Installed              bool
	Name                   string
	SourceCommit           string
	Pkgrel                 int
	RollbackArtifactPath   string
	RollbackArtifactSHA256 string
}

type FingerprintFileObservation struct {
	Path       string
	Exists     bool
	SHA256     string
	Mode       uint32
	Ownership  FileOwnershipKind
	BackupPath string
}

type FingerprintObservation struct {
	BuildRoot string
	Pin       *FingerprintResolvedPin
	Package   FingerprintPackageObservation
	PAM       map[string]FingerprintFileObservation
}

type FingerprintPlan struct {
	Requests []runner.CommandRequest
	Files    []ManagedFileDescriptor
	Steps    []planner.Step
}

func buildFingerprintModule(policy catalog.ResolvedHostPolicy, evidence PlatformEvidence) (planner.Module, error) {
	module := planner.Module{Name: fingerprintModuleName, Enabled: moduleEnabled(policy, fingerprintModuleName)}
	if !module.Enabled {
		return module, nil
	}
	observed, authorized, err := capabilityEvidence(policy, evidence, catalog.RiskFingerprintPAM)
	if err != nil || !authorized {
		return module, err
	}
	if observed.State != EvidenceReady {
		module.Steps = []planner.Step{blockedPolicyStep(module.Name, catalog.RiskFingerprintPAM, observed)}
		return module, nil
	}
	plan, err := BuildFingerprintPlan(evidence.Fingerprint)
	if err != nil {
		return module, err
	}
	module.Steps = append([]planner.Step(nil), plan.Steps...)
	return module, nil
}

// BuildFingerprintPlan constructs an argv-only build/install plan and typed PAM
// file descriptors. It is pure: execution stays behind runner and managed-file
// boundaries.
func BuildFingerprintPlan(observation FingerprintObservation) (FingerprintPlan, error) {
	pkgbuild, patch, pin, err := validateFingerprintObservation(observation)
	if err != nil {
		return FingerprintPlan{}, err
	}
	files, err := loadFingerprintPAMFiles()
	if err != nil {
		return FingerprintPlan{}, err
	}

	plan := FingerprintPlan{Files: cloneFingerprintFiles(files)}
	packageConverged := observation.Package.Installed &&
		observation.Package.Name == fingerprintPackageName &&
		strings.EqualFold(observation.Package.SourceCommit, pin.SourceCommit) &&
		observation.Package.Pkgrel == pin.Pkgrel
	if packageConverged {
		plan.Steps = append(plan.Steps, fingerprintPackageStep(pin, observation.Package, planner.DispositionSatisfied))
	} else {
		if err := validateFingerprintPackageRollback(observation.Package); err != nil {
			return FingerprintPlan{}, err
		}
		requests, err := fingerprintBuildRequests(observation.BuildRoot, pin, pkgbuild, patch)
		if err != nil {
			return FingerprintPlan{}, err
		}
		plan.Requests = append(plan.Requests, requests...)
		for index, request := range requests {
			var dependsOn []string
			if index > 0 {
				dependsOn = []string{requests[index-1].Operation}
			}
			step := fingerprintRequestStep(request, pin, observation.Package, dependsOn)
			if request.Operation == fingerprintInstallOperation {
				step.Inverse = fingerprintPackageInverse(observation.Package)
			}
			plan.Steps = append(plan.Steps, step)
		}
	}

	for _, file := range files {
		observed, ok := observation.PAM[file.Path]
		if !ok {
			return FingerprintPlan{}, fmt.Errorf("%w: missing ownership observation for %s", ErrInvalidFingerprintObservation, file.Path)
		}
		step, err := fingerprintPAMFileStep(file, observed)
		if err != nil {
			return FingerprintPlan{}, err
		}
		plan.Steps = append(plan.Steps, step)
	}
	return cloneFingerprintPlan(plan), nil
}

func validateFingerprintObservation(observation FingerprintObservation) ([]byte, []byte, FingerprintResolvedPin, error) {
	if observation.Pin == nil {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: resolved pin is required", ErrInvalidFingerprintObservation)
	}
	pin := *observation.Pin
	if err := catalog.ValidateAURLocalPin(fingerprintPackageName, catalog.AURLocalPin{SourceCommit: pin.SourceCommit, PatchSHA256: pin.PatchSHA256}); err != nil {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: %v", ErrInvalidFingerprintObservation, err)
	}
	if pin.Pkgrel <= 0 || !validFingerprintSHA(pin.PKGBUILDSHA256) || !validFingerprintSHA(pin.ArtifactSHA256) {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: pkgrel and build/artifact checksums must be exact", ErrInvalidFingerprintObservation)
	}
	if pin.ArtifactName == "" || filepath.Base(pin.ArtifactName) != pin.ArtifactName || filepath.Clean(pin.ArtifactName) != pin.ArtifactName ||
		!strings.HasPrefix(pin.ArtifactName, fingerprintPackageName+"-") || !strings.HasSuffix(pin.ArtifactName, ".pkg.tar.zst") || !safeFingerprintArgument(pin.ArtifactName) {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: artifact name is not a safe exact package basename", ErrInvalidFingerprintObservation)
	}
	if epoch, err := strconv.ParseInt(pin.SourceDateEpoch, 10, 64); err != nil || epoch <= 0 {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: source date epoch must be a positive integer", ErrInvalidFingerprintObservation)
	}
	if observation.BuildRoot == "" || !filepath.IsAbs(observation.BuildRoot) || filepath.Clean(observation.BuildRoot) != observation.BuildRoot || observation.BuildRoot == "/" || !utf8.ValidString(observation.BuildRoot) || !safeFingerprintArgument(observation.BuildRoot) {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: build root must be a clean absolute non-root path", ErrInvalidFingerprintObservation)
	}
	pkgbuild, err := fs.ReadFile(assets.FS, fingerprintPackagingRoot+"/PKGBUILD")
	if err != nil {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: read embedded PKGBUILD: %v", ErrInvalidFingerprintObservation, err)
	}
	patch, err := fs.ReadFile(assets.FS, fingerprintPackagingRoot+"/"+fingerprintPatchName)
	if err != nil {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: read embedded patch: %v", ErrInvalidFingerprintObservation, err)
	}
	if fingerprintSHA(pkgbuild) != strings.ToLower(pin.PKGBUILDSHA256) || fingerprintSHA(patch) != strings.ToLower(pin.PatchSHA256) {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: embedded packaging differs from resolved checksums", ErrInvalidFingerprintObservation)
	}
	text := string(pkgbuild)
	if !hasExactAssignment(text, "_commit", pin.SourceCommit) || !hasExactAssignment(text, "pkgrel", strconv.Itoa(pin.Pkgrel)) ||
		!strings.Contains(text, "'"+strings.ToLower(pin.PatchSHA256)+"'") ||
		!strings.Contains(text, "provides=(libfprint libfprint-2.so)") || !strings.Contains(text, "conflicts=(libfprint)") {
		return nil, nil, FingerprintResolvedPin{}, fmt.Errorf("%w: resolved pin does not match embedded package authority", ErrInvalidFingerprintObservation)
	}
	return append([]byte(nil), pkgbuild...), append([]byte(nil), patch...), pin, nil
}

func fingerprintBuildRequests(buildRoot string, pin FingerprintResolvedPin, pkgbuild, patch []byte) ([]runner.CommandRequest, error) {
	artifactPath := filepath.Join(buildRoot, pin.ArtifactName)
	specs := []struct {
		operation  string
		executable string
		argv       []string
		cwd        string
		scope      runner.Scope
		network    runner.NetworkPolicy
		stdin      []byte
		env        map[string]string
		discard    bool
	}{
		{fingerprintBuildRootOperation, "/usr/bin/mkdir", []string{"-p", "--", buildRoot}, "/", runner.ScopeUser, runner.NetworkNone, nil, nil, false},
		{fingerprintPKGBUILDWriteOperation, "/usr/bin/tee", []string{filepath.Join(buildRoot, "PKGBUILD")}, buildRoot, runner.ScopeUser, runner.NetworkNone, pkgbuild, nil, true},
		{fingerprintPatchWriteOperation, "/usr/bin/tee", []string{filepath.Join(buildRoot, fingerprintPatchName)}, buildRoot, runner.ScopeUser, runner.NetworkNone, patch, nil, true},
		{fingerprintInputsVerifyOperation, "/usr/bin/sha256sum", []string{"--check", "--strict", "-"}, buildRoot, runner.ScopeUser, runner.NetworkNone, []byte(strings.ToLower(pin.PKGBUILDSHA256) + "  PKGBUILD\n" + strings.ToLower(pin.PatchSHA256) + "  " + fingerprintPatchName + "\n"), nil, false},
		{fingerprintBuildOperation, "/usr/bin/makepkg", []string{"--cleanbuild", "--force", "--noconfirm"}, buildRoot, runner.ScopeUser, runner.NetworkRequired, nil, map[string]string{"SOURCE_DATE_EPOCH": pin.SourceDateEpoch, "LC_ALL": "C", "PKGDEST": buildRoot, "SRCDEST": filepath.Join(buildRoot, "sources"), "BUILDDIR": filepath.Join(buildRoot, "build")}, false},
		{fingerprintArtifactVerifyOperation, "/usr/bin/sha256sum", []string{"--check", "--strict", "-"}, buildRoot, runner.ScopeUser, runner.NetworkNone, []byte(strings.ToLower(pin.ArtifactSHA256) + "  " + pin.ArtifactName + "\n"), nil, false},
		{fingerprintInstallOperation, "/usr/bin/pacman", []string{"-U", "--needed", "--noconfirm", artifactPath}, "/", runner.ScopeSystem, runner.NetworkNone, nil, nil, false},
	}
	requests := make([]runner.CommandRequest, 0, len(specs))
	for _, spec := range specs {
		request, err := makeRequest(spec.operation, spec.executable, spec.argv, spec.scope, spec.network, spec.stdin)
		if err != nil {
			return nil, err
		}
		request.Cwd = spec.cwd
		request.Env = copyFingerprintStringMap(spec.env)
		if spec.discard {
			request.OutputPolicy = runner.OutputDiscard
		}
		if err := runner.ValidateCommandRequest(request); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func loadFingerprintPAMFiles() ([]ManagedFileDescriptor, error) {
	files := make([]ManagedFileDescriptor, 0, len(fingerprintPAMNames))
	contents := make(map[string]string, len(fingerprintPAMNames))
	for _, name := range fingerprintPAMNames {
		content, err := fs.ReadFile(assets.FS, "overlays/galaxy/etc/pam.d/"+name)
		if err != nil {
			return nil, fmt.Errorf("%w: read PAM overlay %s: %v", ErrInvalidFingerprintObservation, name, err)
		}
		contents[name] = string(content)
		files = append(files, ManagedFileDescriptor{Path: filepath.Join("/etc/pam.d", name), Content: append([]byte(nil), content...), Mode: 0o644, Kind: "fingerprint-pam"})
	}
	for name, content := range contents {
		if pamContainsSufficientFingerprint(content) {
			continue
		}
		if name != "cosmic-greeter" || !strings.Contains(content, "auth       include      system-local-login") || !pamContainsSufficientFingerprint(contents["system-local-login"]) {
			return nil, fmt.Errorf("%w: PAM overlay %s lacks sufficient fingerprint authentication", ErrInvalidFingerprintObservation, name)
		}
	}
	return files, nil
}

func fingerprintPAMFileStep(file ManagedFileDescriptor, observation FingerprintFileObservation) (planner.Step, error) {
	if observation.Path != "" && observation.Path != file.Path {
		return planner.Step{}, fmt.Errorf("%w: PAM observation path differs from target", ErrInvalidFingerprintObservation)
	}
	inverse, err := file.PlannerInverse(FileOwnershipObservation{Kind: observation.Ownership, BackupPath: observation.BackupPath})
	if err != nil {
		return planner.Step{}, err
	}
	wantSHA := fingerprintSHA(file.Content)
	converged := observation.Exists && strings.EqualFold(observation.SHA256, wantSHA) && observation.Mode == file.Mode
	return planner.Step{
		ID: "fingerprint.pam." + filepath.Base(file.Path), Module: fingerprintModuleName,
		DependsOn: []string{fingerprintInstallOperation}, Description: "publish exact fingerprint PAM overlay",
		Scope: planner.ScopeSystem, Network: planner.NetworkNone, Operation: "managed-file.publish", Disposition: convergedDisposition(converged),
		Desired:  mustJSON(map[string]any{"path": file.Path, "sha256": wantSHA, "mode": file.Mode, "authControl": "sufficient"}),
		Observed: mustJSON(map[string]any{"exists": observation.Exists, "sha256": observation.SHA256, "mode": observation.Mode, "ownership": observation.Ownership, "backup": observation.BackupPath}),
		Inverse:  &inverse,
	}, nil
}

func fingerprintRequestStep(request runner.CommandRequest, pin FingerprintResolvedPin, observed FingerprintPackageObservation, dependsOn []string) planner.Step {
	desired := map[string]any{"request": commandIdentity(request), "sourceCommit": pin.SourceCommit, "pkgrel": pin.Pkgrel, "ownership": "explicit", "transactionPolicy": "verified-local-package-only", "implicitUpgradeDowngrade": false}
	if request.Operation == fingerprintArtifactVerifyOperation || request.Operation == fingerprintInstallOperation {
		desired["artifactSHA256"] = strings.ToLower(pin.ArtifactSHA256)
		desired["artifactName"] = pin.ArtifactName
	}
	if request.Operation == fingerprintInstallOperation {
		desired["versionChange"] = map[string]any{
			"from": map[string]any{"name": observed.Name, "sourceCommit": observed.SourceCommit, "pkgrel": observed.Pkgrel},
			"to":   map[string]any{"name": fingerprintPackageName, "sourceCommit": pin.SourceCommit, "pkgrel": pin.Pkgrel},
		}
	}
	return planner.Step{ID: request.Operation, Module: fingerprintModuleName, DependsOn: append([]string(nil), dependsOn...), Description: request.Operation,
		Scope: planner.Scope(request.Scope), Network: planner.NetworkClass(request.Network), Operation: planner.Operation(request.Operation), Disposition: planner.DispositionApply,
		Desired: mustJSON(desired), Observed: mustJSON(observed)}
}

func fingerprintPackageStep(pin FingerprintResolvedPin, observed FingerprintPackageObservation, disposition planner.Disposition) planner.Step {
	desired := map[string]any{"name": fingerprintPackageName, "sourceCommit": pin.SourceCommit, "pkgrel": pin.Pkgrel, "artifactSHA256": strings.ToLower(pin.ArtifactSHA256), "ownership": "explicit", "transactionPolicy": "verified-local-package-only", "implicitUpgradeDowngrade": false}
	return planner.Step{ID: fingerprintInstallOperation, Module: fingerprintModuleName, Description: "install exact verified local fingerprint package", Scope: planner.ScopeSystem, Network: planner.NetworkNone, Operation: fingerprintInstallOperation, Disposition: disposition, Desired: mustJSON(desired), Observed: mustJSON(observed)}
}

func validateFingerprintPackageRollback(observed FingerprintPackageObservation) error {
	if !observed.Installed {
		return nil
	}
	if observed.Name == "" || observed.RollbackArtifactPath == "" || !filepath.IsAbs(observed.RollbackArtifactPath) || filepath.Clean(observed.RollbackArtifactPath) != observed.RollbackArtifactPath || !safeFingerprintArgument(observed.RollbackArtifactPath) || !validFingerprintSHA(observed.RollbackArtifactSHA256) {
		return fmt.Errorf("%w: replacing an installed package requires an exact local rollback artifact", ErrInvalidFingerprintObservation)
	}
	return nil
}

func fingerprintPackageInverse(observed FingerprintPackageObservation) *planner.InverseDescriptor {
	if !observed.Installed {
		return &planner.InverseDescriptor{Operation: "fingerprint.package.remove", Value: mustJSON(map[string]any{"name": fingerprintPackageName})}
	}
	return &planner.InverseDescriptor{Operation: "fingerprint.package.restore-local", Value: mustJSON(map[string]any{"name": observed.Name, "artifact": observed.RollbackArtifactPath, "sha256": strings.ToLower(observed.RollbackArtifactSHA256)})}
}

func fingerprintSHA(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func validFingerprintSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func safeFingerprintArgument(value string) bool {
	return utf8.ValidString(value) && !hasControl(value) && !strings.ContainsAny(value, ";|&`$()<>")
}

func hasExactAssignment(content, name, value string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == name+"="+value {
			return true
		}
	}
	return false
}

func pamContainsSufficientFingerprint(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "auth" && fields[1] == "sufficient" && fields[2] == "pam_fprintd.so" {
			return true
		}
	}
	return false
}

func cloneFingerprintFiles(files []ManagedFileDescriptor) []ManagedFileDescriptor {
	cloned := make([]ManagedFileDescriptor, len(files))
	for index := range files {
		cloned[index] = files[index].Clone()
	}
	return cloned
}

func cloneFingerprintPlan(plan FingerprintPlan) FingerprintPlan {
	cloned := FingerprintPlan{Files: cloneFingerprintFiles(plan.Files), Steps: append([]planner.Step(nil), plan.Steps...)}
	cloned.Requests = make([]runner.CommandRequest, len(plan.Requests))
	for index := range plan.Requests {
		cloned.Requests[index] = plan.Requests[index]
		cloned.Requests[index].Argv = append([]string(nil), plan.Requests[index].Argv...)
		cloned.Requests[index].Stdin = append([]byte(nil), plan.Requests[index].Stdin...)
		cloned.Requests[index].Env = copyFingerprintStringMap(plan.Requests[index].Env)
	}
	return cloned
}

func copyFingerprintStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
