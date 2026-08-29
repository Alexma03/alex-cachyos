package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	"alex-cachyos/internal/adopt"
)

// DriftClass is the safe, user-actionable classification of a check result.
type DriftClass string

const (
	DriftNone              DriftClass = "none"
	DriftDesiredState      DriftClass = "desired-state"
	DriftPostApply         DriftClass = "post-apply"
	DriftUnmanagedConflict DriftClass = "unmanaged-conflict"
	DriftUnknown           DriftClass = "unknown"
)

// CheckExitIntent describes the exit behavior a CLI adapter should apply to a
// report. Warnings, including fingerprint warnings, retain ExitIntentZero.
type CheckExitIntent string

type ExitIntent = CheckExitIntent

const (
	ExitIntentZero    CheckExitIntent = "success"
	ExitIntentNonZero CheckExitIntent = "nonzero"
	ExitIntentSuccess CheckExitIntent = ExitIntentZero
	ExitIntentDrift   CheckExitIntent = ExitIntentNonZero
)

// FindingKind identifies the privacy-safe inventory that produced a finding.
type FindingKind string

const (
	FindingManagedFile         FindingKind = "managed-file"
	FindingValidator           FindingKind = "validator"
	FindingPackage             FindingKind = "package"
	FindingService             FindingKind = "service"
	FindingBoot                FindingKind = "boot"
	FindingPAM                 FindingKind = "pam"
	FindingKindFailedUnit      FindingKind = "failed-unit"
	FindingKindPacmanIntegrity FindingKind = "pacman-integrity"
	FindingPacnew              FindingKind = "pacnew"
	FindingObservation         FindingKind = "observation"
)

// CheckFinding contains identifiers and classifications only. It deliberately
// has no file content, command output, environment, or biometric fields.
type CheckFinding struct {
	Kind     FindingKind `json:"kind"`
	Code     string      `json:"code"`
	Class    DriftClass  `json:"class"`
	Subject  string      `json:"subject,omitempty"`
	Path     string      `json:"path,omitempty"`
	Backup   string      `json:"backup,omitempty"`
	Severity string      `json:"severity"`
}

type Finding = CheckFinding

type FileDriftClass = DriftClass

const findingErrorSeverity = "error"

const (
	FindingValidatorFailed        = "validator-failed"
	FindingValidatorUnknown       = "validator-unknown"
	FindingPackageMissing         = "package-missing"
	FindingPackageVersion         = "package-version"
	FindingPackageUnknown         = "package-unknown"
	FindingServiceState           = "service-state"
	FindingServiceUnknown         = "service-unknown"
	FindingBootDrift              = "boot-drift"
	FindingBootUnknown            = "boot-unknown"
	FindingPAMDrift               = "pam-drift"
	FindingPAMUnknown             = "pam-unknown"
	FindingFailedUnit             = "failed-unit"
	FindingFailedUnitUnknown      = "failed-unit-unknown"
	FindingPacmanIntegrity        = "pacman-integrity"
	FindingPacmanIntegrityUnknown = "pacman-integrity-unknown"
	FindingPacnewUnknown          = "pacnew-unknown"
	FindingObservationUnavailable = "observer-unavailable"
)

// CheckReport is the complete offline check result. Findings cause a non-zero
// exit intent; warnings never do.
type CheckReport struct {
	Drift      bool            `json:"drift"`
	ExitIntent CheckExitIntent `json:"exitIntent"`
	Findings   []CheckFinding  `json:"findings"`
	Warnings   []string        `json:"warnings"`
}

type CheckResult = CheckReport

// ExitCode is a convenience for a CLI adapter. Check itself never exits or
// writes anything.
func (r CheckReport) ExitCode() int {
	if r.Drift {
		return 1
	}
	return 0
}

// CheckStatus is the allowlisted result of a typed observation.
type CheckStatus string

const (
	CheckPassed  CheckStatus = "passed"
	CheckFailed  CheckStatus = "failed"
	CheckUnknown CheckStatus = "unknown"

	ObservationPassed  CheckStatus = CheckPassed
	ObservationFailed  CheckStatus = CheckFailed
	ObservationUnknown CheckStatus = CheckUnknown
)

// FileObservationState distinguishes an explicit missing file from a state that
// cannot safely be inspected.
type FileObservationState string

const (
	FileObserved   FileObservationState = "observed"
	FileMissing    FileObservationState = "missing"
	FileUnsafe     FileObservationState = "unsafe"
	FileUnreadable FileObservationState = "unreadable"
	FileUnknown    FileObservationState = "unknown"

	FileObservationObserved   FileObservationState = FileObserved
	FileObservationMissing    FileObservationState = FileMissing
	FileObservationUnsafe     FileObservationState = FileUnsafe
	FileObservationUnreadable FileObservationState = FileUnreadable
	FileObservationUnknown    FileObservationState = FileUnknown
)

// FileOwnership identifies ownership already recorded by an adoption or apply
// receipt. Any non-empty value is considered owned by the checker.
type FileOwnership string

const (
	OwnershipNone    FileOwnership = ""
	OwnershipCreated FileOwnership = "created"
	OwnershipAdopted FileOwnership = "adopted"
	OwnershipManaged FileOwnership = "managed"
	OwnershipPackage FileOwnership = "package-owned"
)

// ManagedFileInventory is the desired and recorded identity of one managed
// path. Hashes are identifiers, never file bytes.
type ManagedFileInventory struct {
	Path               string        `json:"path"`
	DesiredHash        string        `json:"desiredHash"`
	ReceiptAfterHash   string        `json:"receiptAfterHash"`
	AfterHash          string        `json:"afterHash,omitempty"`
	ReceiptBackup      string        `json:"receiptBackup"`
	ReceiptBackupPath  string        `json:"receiptBackupPath,omitempty"`
	AdoptionBackup     string        `json:"adoptionBackup"`
	AdoptionBackupPath string        `json:"adoptionBackupPath,omitempty"`
	Ownership          FileOwnership `json:"ownership"`
	Owned              bool          `json:"owned,omitempty"`
}

// ManagedFileCheck and ManagedFile are compatibility aliases for callers that
// describe the same typed inventory with a check-oriented name.
type ManagedFileCheck = ManagedFileInventory
type ManagedFile = ManagedFileInventory

// ManagedFileObservation is the privacy-safe live result for one path.
type ManagedFileObservation struct {
	Path        string               `json:"path"`
	State       FileObservationState `json:"state"`
	Observation FileObservationState `json:"observation,omitempty"`
	LiveHash    string               `json:"liveHash"`
	Hash        string               `json:"hash,omitempty"`
	Exists      bool                 `json:"exists,omitempty"`
	Ownership   FileOwnership        `json:"ownership,omitempty"`
}

type FileObservation = ManagedFileObservation

// ValidatorInventory names a validator that must pass, such as niri or
// Noctalia validation.
type ValidatorInventory struct {
	Name string `json:"name"`
}

type ValidatorCheck = ValidatorInventory

// ValidatorObservation contains only an allowlisted status.
type ValidatorObservation struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
}

type ValidatorResult = ValidatorObservation

// PackageInventory describes a desired package name and, when non-empty, an
// exact desired version.
type PackageInventory struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type PackageCheck = PackageInventory

// PackageObservation contains package presence and a resolved version. The
// version is an identity, not command output.
type PackageObservation struct {
	Name    string      `json:"name"`
	Present bool        `json:"present"`
	Version string      `json:"version,omitempty"`
	Status  CheckStatus `json:"status,omitempty"`
}

// ServiceInventory describes the service state that is required. Enabled and
// Active set requirements directly; the Require fields make the distinction
// explicit for callers that need a false-valued desired state later.
type ServiceInventory struct {
	Unit           string `json:"unit"`
	Enabled        bool   `json:"enabled,omitempty"`
	Active         bool   `json:"active,omitempty"`
	RequireEnabled bool   `json:"requireEnabled,omitempty"`
	RequireActive  bool   `json:"requireActive,omitempty"`
}

type ServiceCheck = ServiceInventory

// ServiceObservation is a typed systemd state summary.
type ServiceObservation struct {
	Unit      string      `json:"unit"`
	Installed bool        `json:"installed"`
	Enabled   bool        `json:"enabled"`
	Active    bool        `json:"active"`
	Status    CheckStatus `json:"status,omitempty"`
}

// ConfigurationInventory is used for boot and PAM expected hashes. It carries
// no expected bytes.
type ConfigurationInventory struct {
	Name         string `json:"name"`
	DesiredHash  string `json:"desiredHash"`
	ExpectedHash string `json:"expectedHash,omitempty"`
}

type BootInventory = ConfigurationInventory
type PAMInventory = ConfigurationInventory

// ConfigurationObservation is a hash or allowlisted status for boot/PAM state.
type ConfigurationObservation struct {
	Name     string      `json:"name"`
	LiveHash string      `json:"liveHash,omitempty"`
	Hash     string      `json:"hash,omitempty"`
	Status   CheckStatus `json:"status,omitempty"`
}

// FailedUnitInventory enables the failed-unit inventory. An empty observed
// unit list is a known clean result; it is not command output.
type FailedUnitInventory struct {
	Check   bool `json:"check,omitempty"`
	Enabled bool `json:"enabled,omitempty"`
}

// PacmanIntegrityInventory names the typed database checks, normally database
// (-Dk) and files (-Qk).
type PacmanIntegrityInventory struct {
	Checks   []string `json:"checks"`
	Database bool     `json:"database,omitempty"`
	Files    bool     `json:"files,omitempty"`
}

const (
	PacmanDatabaseCheck = "database"
	PacmanFilesCheck    = "files"
)

// PacmanIntegrityObservation is a status for one named integrity check.
type PacmanIntegrityObservation struct {
	Name   string      `json:"name,omitempty"`
	Check  string      `json:"check,omitempty"`
	Clean  bool        `json:"clean"`
	Status CheckStatus `json:"status,omitempty"`
}

// PacnewInventory enables the safe warning inventory. Paths are reported as
// identifiers only.
type PacnewInventory struct {
	Check   bool `json:"check,omitempty"`
	Enabled bool `json:"enabled,omitempty"`
}

// FingerprintInventory is deliberately presence-only and warn-only.
type FingerprintInventory struct {
	Check   bool `json:"check,omitempty"`
	Enabled bool `json:"enabled,omitempty"`
}

type FingerprintPresence string

const (
	FingerprintPresent FingerprintPresence = "present"
	FingerprintAbsent  FingerprintPresence = "absent"
	FingerprintUnknown FingerprintPresence = "unknown"
)

// FingerprintObservation contains no enrollment, template, or command output.
type FingerprintObservation struct {
	Presence FingerprintPresence `json:"presence,omitempty"`
	Present  bool                `json:"present,omitempty"`
	Known    bool                `json:"known,omitempty"`
}

// CheckRequest contains all desired inventories needed by the pure check
// mapper. It does not contain ports, locks, paths to secrets, or file bytes.
type CheckRequest struct {
	ManagedFiles    []ManagedFileInventory   `json:"managedFiles"`
	Validators      []ValidatorInventory     `json:"validators"`
	Packages        []PackageInventory       `json:"packages"`
	Services        []ServiceInventory       `json:"services"`
	Boot            []ConfigurationInventory `json:"boot"`
	PAM             []ConfigurationInventory `json:"pam"`
	FailedUnits     FailedUnitInventory      `json:"failedUnits"`
	PacmanIntegrity PacmanIntegrityInventory `json:"pacmanIntegrity"`
	Pacnew          PacnewInventory          `json:"pacnew"`
	Fingerprint     FingerprintInventory     `json:"fingerprint"`
}

// CheckSnapshot is the single privacy-safe result returned by CheckObserver.
type CheckSnapshot struct {
	ManagedFiles    []ManagedFileObservation     `json:"managedFiles"`
	Validators      []ValidatorObservation       `json:"validators"`
	Packages        []PackageObservation         `json:"packages"`
	Services        []ServiceObservation         `json:"services"`
	Boot            []ConfigurationObservation   `json:"boot"`
	PAM             []ConfigurationObservation   `json:"pam"`
	FailedUnits     []string                     `json:"failedUnits"`
	PacmanIntegrity []PacmanIntegrityObservation `json:"pacmanIntegrity"`
	Pacnew          []string                     `json:"pacnew"`
	Fingerprint     FingerprintObservation       `json:"fingerprint"`
}

// CheckObserver is the only observation seam. Implementations belong outside
// this pure mapper and must return typed summaries rather than bytes/output.
type CheckObserver interface {
	Observe(context.Context, CheckRequest) (CheckSnapshot, error)
}

// CheckObserverFunc adapts a function to CheckObserver.
type CheckObserverFunc func(context.Context, CheckRequest) (CheckSnapshot, error)

func (f CheckObserverFunc) Observe(ctx context.Context, request CheckRequest) (CheckSnapshot, error) {
	if f == nil {
		return CheckSnapshot{}, ErrCheckObserverUnavailable
	}
	return f(ctx, request)
}

var ErrCheckObserverUnavailable = errors.New("check observer unavailable")

// Checker evaluates one isolated observer snapshot. It never acquires a lock,
// calls a runner, uses the network, or mutates a filesystem.
type Checker struct {
	observer CheckObserver
}

func NewChecker(observer CheckObserver) *Checker {
	return &Checker{observer: observer}
}

// Check is the top-level pure check entry point. Observer failures are converted
// into a privacy-safe unknown report rather than exposing the underlying error.
func Check(ctx context.Context, request CheckRequest, observer CheckObserver) CheckReport {
	return NewChecker(observer).Check(ctx, request)
}

// RunCheck is an explicit alias for adapters that prefer a verb at call sites.
func RunCheck(ctx context.Context, request CheckRequest, observer CheckObserver) CheckReport {
	return Check(ctx, request, observer)
}

func (c *Checker) Check(ctx context.Context, request CheckRequest) CheckReport {
	isolatedRequest := CloneCheckRequest(request)
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil || c.observer == nil {
		return finalizeCheck(failClosedReport(isolatedRequest))
	}
	snapshot, err := c.observer.Observe(ctx, isolatedRequest)
	if err != nil {
		return finalizeCheck(failClosedReport(isolatedRequest))
	}
	return finalizeCheck(evaluateCheck(isolatedRequest, CloneCheckSnapshot(snapshot)))
}

// EvaluateCheck maps a previously obtained snapshot without invoking any
// observer. It is useful for deterministic fixtures and remains side-effect
// free.
func EvaluateCheck(request CheckRequest, snapshot CheckSnapshot) CheckReport {
	return finalizeCheck(evaluateCheck(CloneCheckRequest(request), CloneCheckSnapshot(snapshot)))
}

// ClassifyManagedFile applies the documented precedence to one desired file and
// one live observation. DriftNone is returned only for an owned, observed file
// whose live hash equals the desired hash.
func ClassifyManagedFile(expected ManagedFileInventory, observed ManagedFileObservation) DriftClass {
	state := fileState(observed)
	receiptAfterHash := expected.ReceiptAfterHash
	if receiptAfterHash == "" {
		receiptAfterHash = expected.AfterHash
	}
	switch state {
	case FileUnsafe, FileUnreadable, FileUnknown:
		return DriftUnknown
	case FileMissing:
		if receiptAfterHash != "" {
			return DriftPostApply
		}
		if expected.DesiredHash == "" {
			return DriftUnknown
		}
		return DriftDesiredState
	case FileObserved:
		// A complete, safe live observation is required before ownership can
		// influence classification. In particular, an unowned file with no
		// usable live hash is unknown, not an unmanaged conflict.
		liveHash := observed.LiveHash
		if liveHash == "" {
			liveHash = observed.Hash
		}
		if liveHash == "" {
			return DriftUnknown
		}
		if expected.Ownership == OwnershipNone && !expected.Owned && observed.Ownership == OwnershipNone {
			return DriftUnmanagedConflict
		}
		if receiptAfterHash != "" && liveHash != receiptAfterHash {
			return DriftPostApply
		}
		if expected.DesiredHash == "" {
			return DriftUnknown
		}
		if liveHash != expected.DesiredHash {
			return DriftDesiredState
		}
		return DriftNone
	default:
		return DriftUnknown
	}
}

func evaluateCheck(request CheckRequest, snapshot CheckSnapshot) CheckReport {
	result := CheckReport{}
	evaluateManagedFiles(&result, request.ManagedFiles, snapshot.ManagedFiles)
	evaluateValidators(&result, request.Validators, snapshot.Validators)
	evaluatePackages(&result, request.Packages, snapshot.Packages)
	evaluateServices(&result, request.Services, snapshot.Services)
	evaluateConfigurations(&result, FindingBoot, FindingBootDrift, FindingBootUnknown, request.Boot, snapshot.Boot)
	evaluateConfigurations(&result, FindingPAM, FindingPAMDrift, FindingPAMUnknown, request.PAM, snapshot.PAM)
	evaluateFailedUnits(&result, request.FailedUnits, snapshot.FailedUnits)
	evaluatePacmanIntegrity(&result, request.PacmanIntegrity, snapshot.PacmanIntegrity)
	evaluatePacnew(&result, request.Pacnew, snapshot.Pacnew)
	evaluateFingerprint(&result, request.Fingerprint, snapshot.Fingerprint)
	return result
}

func evaluateManagedFiles(result *CheckReport, expected []ManagedFileInventory, observed []ManagedFileObservation) {
	index, duplicates := fileIndex(observed)
	for _, file := range expected {
		live, ok := index[file.Path]
		if !ok || duplicates[file.Path] {
			addFinding(result, CheckFinding{Kind: FindingManagedFile, Code: FindingObservationUnavailable, Class: DriftUnknown, Subject: file.Path, Path: file.Path, Backup: backupPath(file)})
			continue
		}
		class := ClassifyManagedFile(file, live)
		if class == DriftNone {
			continue
		}
		code := "managed-file-" + string(class)
		addFinding(result, CheckFinding{Kind: FindingManagedFile, Code: code, Class: class, Subject: file.Path, Path: file.Path, Backup: backupPath(file)})
	}
}

func evaluateValidators(result *CheckReport, expected []ValidatorInventory, observed []ValidatorObservation) {
	index, duplicates := validatorIndex(observed)
	names := validatorNames(expected, observed)
	for _, name := range names {
		value, ok := index[name]
		if !ok || duplicates[name] {
			addFinding(result, CheckFinding{Kind: FindingValidator, Code: FindingValidatorUnknown, Class: DriftUnknown, Subject: name})
			continue
		}
		switch normalizedStatus(value.Status) {
		case CheckPassed:
		case CheckFailed:
			addFinding(result, CheckFinding{Kind: FindingValidator, Code: FindingValidatorFailed, Class: DriftDesiredState, Subject: name})
		default:
			addFinding(result, CheckFinding{Kind: FindingValidator, Code: FindingValidatorUnknown, Class: DriftUnknown, Subject: name})
		}
	}
}

func evaluatePackages(result *CheckReport, expected []PackageInventory, observed []PackageObservation) {
	index, duplicates := packageIndex(observed)
	items := packageNames(expected, observed)
	for _, name := range items {
		value, ok := index[name]
		if !ok || duplicates[name] {
			addFinding(result, CheckFinding{Kind: FindingPackage, Code: FindingPackageUnknown, Class: DriftUnknown, Subject: name})
			continue
		}
		want, wanted := packageByName(expected, name)
		status := normalizedStatus(value.Status)
		if status == CheckUnknown {
			addFinding(result, CheckFinding{Kind: FindingPackage, Code: FindingPackageUnknown, Class: DriftUnknown, Subject: name})
			continue
		}
		if status == CheckFailed || !value.Present {
			if !value.Present {
				addFinding(result, CheckFinding{Kind: FindingPackage, Code: FindingPackageMissing, Class: DriftDesiredState, Subject: name})
				continue
			}
			addFinding(result, CheckFinding{Kind: FindingPackage, Code: FindingPackageVersion, Class: DriftDesiredState, Subject: name})
			continue
		}
		if wanted && want.Version != "" && value.Version != want.Version {
			addFinding(result, CheckFinding{Kind: FindingPackage, Code: FindingPackageVersion, Class: DriftDesiredState, Subject: name})
		}
	}
}

func evaluateServices(result *CheckReport, expected []ServiceInventory, observed []ServiceObservation) {
	index, duplicates := serviceIndex(observed)
	items := serviceNames(expected, observed)
	for _, unit := range items {
		value, ok := index[unit]
		if !ok || duplicates[unit] {
			addFinding(result, CheckFinding{Kind: FindingService, Code: FindingServiceUnknown, Class: DriftUnknown, Subject: unit})
			continue
		}
		if normalizedStatus(value.Status) == CheckUnknown {
			addFinding(result, CheckFinding{Kind: FindingService, Code: FindingServiceUnknown, Class: DriftUnknown, Subject: unit})
			continue
		}
		want, wanted := serviceByUnit(expected, unit)
		if !wanted {
			if !value.Installed || normalizedStatus(value.Status) == CheckFailed {
				addFinding(result, CheckFinding{Kind: FindingService, Code: FindingServiceState, Class: DriftDesiredState, Subject: unit})
			}
			continue
		}
		requireEnabled := want.RequireEnabled || want.Enabled
		requireActive := want.RequireActive || want.Active
		if !value.Installed || normalizedStatus(value.Status) == CheckFailed || requireEnabled && !value.Enabled || requireActive && !value.Active {
			addFinding(result, CheckFinding{Kind: FindingService, Code: FindingServiceState, Class: DriftDesiredState, Subject: unit})
		}
	}
}

func evaluateConfigurations(result *CheckReport, kind FindingKind, driftCode, unknownCode string, expected []ConfigurationInventory, observed []ConfigurationObservation) {
	index, duplicates := configurationIndex(observed)
	items := configurationNames(expected, observed)
	for _, name := range items {
		value, ok := index[name]
		if !ok || duplicates[name] {
			addFinding(result, CheckFinding{Kind: kind, Code: unknownCode, Class: DriftUnknown, Subject: name})
			continue
		}
		status := normalizedStatus(value.Status)
		if status == CheckUnknown {
			addFinding(result, CheckFinding{Kind: kind, Code: unknownCode, Class: DriftUnknown, Subject: name})
			continue
		}
		if status == CheckFailed {
			addFinding(result, CheckFinding{Kind: kind, Code: driftCode, Class: DriftDesiredState, Subject: name})
			continue
		}
		want, wanted := configurationByName(expected, name)
		liveHash := value.LiveHash
		if liveHash == "" {
			liveHash = value.Hash
		}
		desiredHash := want.DesiredHash
		if desiredHash == "" {
			desiredHash = want.ExpectedHash
		}
		if wanted {
			if desiredHash == "" || liveHash == "" {
				addFinding(result, CheckFinding{Kind: kind, Code: unknownCode, Class: DriftUnknown, Subject: name})
				continue
			}
			if liveHash != desiredHash {
				addFinding(result, CheckFinding{Kind: kind, Code: driftCode, Class: DriftDesiredState, Subject: name})
			}
			continue
		}
		if status != CheckPassed {
			addFinding(result, CheckFinding{Kind: kind, Code: unknownCode, Class: DriftUnknown, Subject: name})
		}
	}
}

func evaluateFailedUnits(result *CheckReport, expected FailedUnitInventory, observed []string) {
	if !expected.Check && !expected.Enabled && len(observed) == 0 {
		return
	}
	for _, unit := range sortedUnique(observed) {
		if unit == "" {
			addFinding(result, CheckFinding{Kind: FindingKindFailedUnit, Code: FindingFailedUnitUnknown, Class: DriftUnknown, Subject: "failed-unit"})
			continue
		}
		addFinding(result, CheckFinding{Kind: FindingKindFailedUnit, Code: FindingFailedUnit, Class: DriftDesiredState, Subject: unit})
	}
}

func evaluatePacmanIntegrity(result *CheckReport, expected PacmanIntegrityInventory, observed []PacmanIntegrityObservation) {
	index, duplicates := pacmanIndex(observed)
	checks := pacmanChecks(expected)
	if len(checks) == 0 {
		checks = pacmanObservationNames(observed)
	}
	for _, name := range checks {
		value, ok := index[name]
		if !ok || duplicates[name] {
			addFinding(result, CheckFinding{Kind: FindingKindPacmanIntegrity, Code: FindingPacmanIntegrityUnknown, Class: DriftUnknown, Subject: name})
			continue
		}
		status := normalizedStatus(value.Status)
		if status == CheckUnknown {
			addFinding(result, CheckFinding{Kind: FindingKindPacmanIntegrity, Code: FindingPacmanIntegrityUnknown, Class: DriftUnknown, Subject: name})
			continue
		}
		if status == CheckFailed || status == "" && !value.Clean {
			addFinding(result, CheckFinding{Kind: FindingKindPacmanIntegrity, Code: FindingPacmanIntegrity, Class: DriftDesiredState, Subject: name})
		}
	}
}

func evaluatePacnew(result *CheckReport, expected PacnewInventory, observed []string) {
	if !expected.Check && !expected.Enabled && len(observed) == 0 {
		return
	}
	for _, path := range sortedUnique(observed) {
		if path == "" {
			addWarning(result, "pacnew:unknown")
			continue
		}
		addWarning(result, "pacnew:"+path)
	}
}

func evaluateFingerprint(result *CheckReport, expected FingerprintInventory, observed FingerprintObservation) {
	if !expected.Check && !expected.Enabled && observed.Presence == "" && !observed.Known {
		return
	}
	presence := observed.Presence
	if presence == "" && observed.Known {
		if observed.Present {
			presence = FingerprintPresent
		} else {
			presence = FingerprintAbsent
		}
	}
	switch presence {
	case FingerprintPresent:
	case FingerprintAbsent:
		addWarning(result, "fingerprint-enrollment-absent")
	default:
		addWarning(result, "fingerprint-enrollment-unknown")
	}
}

func failClosedReport(request CheckRequest) CheckReport {
	result := CheckReport{}
	for _, file := range request.ManagedFiles {
		addFinding(&result, CheckFinding{Kind: FindingManagedFile, Code: FindingObservationUnavailable, Class: DriftUnknown, Subject: file.Path, Path: file.Path, Backup: backupPath(file)})
	}
	for _, validator := range request.Validators {
		addFinding(&result, CheckFinding{Kind: FindingValidator, Code: FindingValidatorUnknown, Class: DriftUnknown, Subject: validator.Name})
	}
	for _, packageValue := range request.Packages {
		addFinding(&result, CheckFinding{Kind: FindingPackage, Code: FindingPackageUnknown, Class: DriftUnknown, Subject: packageValue.Name})
	}
	for _, service := range request.Services {
		addFinding(&result, CheckFinding{Kind: FindingService, Code: FindingServiceUnknown, Class: DriftUnknown, Subject: service.Unit})
	}
	for _, boot := range request.Boot {
		addFinding(&result, CheckFinding{Kind: FindingBoot, Code: FindingBootUnknown, Class: DriftUnknown, Subject: boot.Name})
	}
	for _, pam := range request.PAM {
		addFinding(&result, CheckFinding{Kind: FindingPAM, Code: FindingPAMUnknown, Class: DriftUnknown, Subject: pam.Name})
	}
	if request.FailedUnits.Check || request.FailedUnits.Enabled {
		addFinding(&result, CheckFinding{Kind: FindingKindFailedUnit, Code: FindingFailedUnitUnknown, Class: DriftUnknown, Subject: "failed-units"})
	}
	for _, name := range pacmanChecks(request.PacmanIntegrity) {
		addFinding(&result, CheckFinding{Kind: FindingKindPacmanIntegrity, Code: FindingPacmanIntegrityUnknown, Class: DriftUnknown, Subject: name})
	}
	if request.Pacnew.Check || request.Pacnew.Enabled {
		addFinding(&result, CheckFinding{Kind: FindingPacnew, Code: FindingPacnewUnknown, Class: DriftUnknown, Subject: "pacnew"})
	}
	if request.Fingerprint.Check || request.Fingerprint.Enabled {
		addWarning(&result, "fingerprint-enrollment-unknown")
	}
	if len(result.Findings) == 0 && len(result.Warnings) == 0 {
		addFinding(&result, CheckFinding{Kind: FindingObservation, Code: FindingObservationUnavailable, Class: DriftUnknown, Subject: "check-observer"})
	}
	return result
}

func BackupPath(file ManagedFileInventory) string {
	return backupPath(file)
}

func backupPath(file ManagedFileInventory) string {
	if strings.TrimSpace(file.ReceiptBackup) != "" {
		return file.ReceiptBackup
	}
	if strings.TrimSpace(file.ReceiptBackupPath) != "" {
		return file.ReceiptBackupPath
	}
	if strings.TrimSpace(file.AdoptionBackup) != "" {
		return file.AdoptionBackup
	}
	if strings.TrimSpace(file.AdoptionBackupPath) != "" {
		return file.AdoptionBackupPath
	}
	if file.Path == "" {
		return ""
	}
	return file.Path + adopt.BackupSuffix
}

func fileState(observed ManagedFileObservation) FileObservationState {
	state := observed.State
	if state == "" {
		state = observed.Observation
	}
	switch strings.ToLower(string(state)) {
	case string(FileObserved), "present", "ok":
		return FileObserved
	case string(FileMissing), "absent":
		return FileMissing
	case string(FileUnsafe):
		return FileUnsafe
	case string(FileUnreadable), "error":
		return FileUnreadable
	case string(FileUnknown):
		return FileUnknown
	case "":
		if observed.LiveHash != "" || observed.Hash != "" || observed.Exists {
			return FileObserved
		}
		return FileMissing
	default:
		return FileUnknown
	}
}

func normalizedStatus(status CheckStatus) CheckStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case "pass", "passed", "ok", "valid", "clean", "satisfied":
		return CheckPassed
	case "fail", "failed", "invalid", "dirty", "drift":
		return CheckFailed
	case "unknown", "unavailable", "unsafe", "unreadable", "":
		if strings.TrimSpace(string(status)) == "" {
			return ""
		}
		return CheckUnknown
	default:
		return CheckUnknown
	}
}

func addFinding(result *CheckReport, finding CheckFinding) {
	if finding.Severity == "" {
		finding.Severity = findingErrorSeverity
	}
	result.Findings = append(result.Findings, finding)
}

func addWarning(result *CheckReport, warning string) {
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}
}

func finalizeCheck(result CheckReport) CheckReport {
	sort.SliceStable(result.Findings, func(i, j int) bool { return findingKey(result.Findings[i]) < findingKey(result.Findings[j]) })
	result.Warnings = sortedUnique(result.Warnings)
	result.Drift = len(result.Findings) != 0
	if result.Drift {
		result.ExitIntent = ExitIntentNonZero
	} else {
		result.ExitIntent = ExitIntentZero
	}
	return result
}

func findingKey(finding CheckFinding) string {
	return string(finding.Kind) + "\x00" + finding.Path + "\x00" + finding.Subject + "\x00" + string(finding.Class) + "\x00" + finding.Code + "\x00" + finding.Backup + "\x00" + finding.Severity
}

func fileIndex(values []ManagedFileObservation) (map[string]ManagedFileObservation, map[string]bool) {
	index := make(map[string]ManagedFileObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		if _, exists := index[value.Path]; exists {
			duplicates[value.Path] = true
		}
		index[value.Path] = value
	}
	return index, duplicates
}

func validatorIndex(values []ValidatorObservation) (map[string]ValidatorObservation, map[string]bool) {
	index := make(map[string]ValidatorObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		if _, exists := index[value.Name]; exists {
			duplicates[value.Name] = true
		}
		index[value.Name] = value
	}
	return index, duplicates
}

func packageIndex(values []PackageObservation) (map[string]PackageObservation, map[string]bool) {
	index := make(map[string]PackageObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		if _, exists := index[value.Name]; exists {
			duplicates[value.Name] = true
		}
		index[value.Name] = value
	}
	return index, duplicates
}

func serviceIndex(values []ServiceObservation) (map[string]ServiceObservation, map[string]bool) {
	index := make(map[string]ServiceObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		if _, exists := index[value.Unit]; exists {
			duplicates[value.Unit] = true
		}
		index[value.Unit] = value
	}
	return index, duplicates
}

func configurationIndex(values []ConfigurationObservation) (map[string]ConfigurationObservation, map[string]bool) {
	index := make(map[string]ConfigurationObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		if _, exists := index[value.Name]; exists {
			duplicates[value.Name] = true
		}
		index[value.Name] = value
	}
	return index, duplicates
}

func pacmanIndex(values []PacmanIntegrityObservation) (map[string]PacmanIntegrityObservation, map[string]bool) {
	index := make(map[string]PacmanIntegrityObservation, len(values))
	duplicates := make(map[string]bool)
	for _, value := range values {
		name := value.Name
		if name == "" {
			name = value.Check
		}
		if _, exists := index[name]; exists {
			duplicates[name] = true
		}
		index[name] = value
	}
	return index, duplicates
}

func validatorNames(expected []ValidatorInventory, observed []ValidatorObservation) []string {
	if len(expected) != 0 {
		values := make([]string, 0, len(expected))
		for _, value := range expected {
			values = append(values, value.Name)
		}
		return sortedUnique(values)
	}
	values := make([]string, 0, len(observed))
	for _, value := range observed {
		values = append(values, value.Name)
	}
	return sortedUnique(values)
}

func packageNames(expected []PackageInventory, observed []PackageObservation) []string {
	if len(expected) != 0 {
		values := make([]string, 0, len(expected))
		for _, value := range expected {
			values = append(values, value.Name)
		}
		return sortedUnique(values)
	}
	values := make([]string, 0, len(observed))
	for _, value := range observed {
		values = append(values, value.Name)
	}
	return sortedUnique(values)
}

func serviceNames(expected []ServiceInventory, observed []ServiceObservation) []string {
	if len(expected) != 0 {
		values := make([]string, 0, len(expected))
		for _, value := range expected {
			values = append(values, value.Unit)
		}
		return sortedUnique(values)
	}
	values := make([]string, 0, len(observed))
	for _, value := range observed {
		values = append(values, value.Unit)
	}
	return sortedUnique(values)
}

func configurationNames(expected []ConfigurationInventory, observed []ConfigurationObservation) []string {
	if len(expected) != 0 {
		values := make([]string, 0, len(expected))
		for _, value := range expected {
			values = append(values, value.Name)
		}
		return sortedUnique(values)
	}
	values := make([]string, 0, len(observed))
	for _, value := range observed {
		values = append(values, value.Name)
	}
	return sortedUnique(values)
}

func pacmanChecks(expected PacmanIntegrityInventory) []string {
	values := append([]string(nil), expected.Checks...)
	if expected.Database {
		values = append(values, PacmanDatabaseCheck)
	}
	if expected.Files {
		values = append(values, PacmanFilesCheck)
	}
	return sortedUnique(values)
}

func pacmanObservationNames(values []PacmanIntegrityObservation) []string {
	names := make([]string, 0, len(values))
	for _, value := range values {
		if value.Name != "" {
			names = append(names, value.Name)
		} else {
			names = append(names, value.Check)
		}
	}
	return sortedUnique(names)
}

func packageByName(values []PackageInventory, name string) (PackageInventory, bool) {
	for _, value := range values {
		if value.Name == name {
			return value, true
		}
	}
	return PackageInventory{}, false
}

func serviceByUnit(values []ServiceInventory, unit string) (ServiceInventory, bool) {
	for _, value := range values {
		if value.Unit == unit {
			return value, true
		}
	}
	return ServiceInventory{}, false
}

func configurationByName(values []ConfigurationInventory, name string) (ConfigurationInventory, bool) {
	for _, value := range values {
		if value.Name == name {
			return value, true
		}
	}
	return ConfigurationInventory{}, false
}

func sortedUnique(values []string) []string {
	if values == nil {
		return nil
	}
	copyValues := make([]string, len(values))
	copy(copyValues, values)
	sort.Strings(copyValues)
	out := copyValues[:0]
	for _, value := range copyValues {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

// CloneCheckRequest returns an isolated request while preserving nil versus
// empty slices. It is exported so adapters can make the same boundary explicit.
func CloneCheckRequest(value CheckRequest) CheckRequest {
	return CheckRequest{
		ManagedFiles:    cloneManagedFileInventories(value.ManagedFiles),
		Validators:      cloneValidatorInventories(value.Validators),
		Packages:        clonePackageInventories(value.Packages),
		Services:        cloneServiceInventories(value.Services),
		Boot:            cloneConfigurationInventories(value.Boot),
		PAM:             cloneConfigurationInventories(value.PAM),
		FailedUnits:     value.FailedUnits,
		PacmanIntegrity: PacmanIntegrityInventory{Checks: cloneStrings(value.PacmanIntegrity.Checks), Database: value.PacmanIntegrity.Database, Files: value.PacmanIntegrity.Files},
		Pacnew:          value.Pacnew,
		Fingerprint:     value.Fingerprint,
	}
}

// CloneCheckSnapshot returns an isolated privacy-safe snapshot while preserving
// nil versus empty slices.
func CloneCheckSnapshot(value CheckSnapshot) CheckSnapshot {
	return CheckSnapshot{
		ManagedFiles:    cloneManagedFileObservations(value.ManagedFiles),
		Validators:      cloneValidatorObservations(value.Validators),
		Packages:        clonePackageObservations(value.Packages),
		Services:        cloneServiceObservations(value.Services),
		Boot:            cloneConfigurationObservations(value.Boot),
		PAM:             cloneConfigurationObservations(value.PAM),
		FailedUnits:     cloneStrings(value.FailedUnits),
		PacmanIntegrity: clonePacmanObservations(value.PacmanIntegrity),
		Pacnew:          cloneStrings(value.Pacnew),
		Fingerprint:     value.Fingerprint,
	}
}

// CloneCheckReport returns a defensive report copy.
func CloneCheckReport(value CheckReport) CheckReport {
	result := value
	if value.Findings != nil {
		result.Findings = make([]CheckFinding, len(value.Findings))
		copy(result.Findings, value.Findings)
	}
	result.Warnings = cloneStrings(value.Warnings)
	return result
}

func cloneManagedFileInventories(values []ManagedFileInventory) []ManagedFileInventory {
	if values == nil {
		return nil
	}
	result := make([]ManagedFileInventory, len(values))
	copy(result, values)
	return result
}
func cloneValidatorInventories(values []ValidatorInventory) []ValidatorInventory {
	if values == nil {
		return nil
	}
	result := make([]ValidatorInventory, len(values))
	copy(result, values)
	return result
}
func clonePackageInventories(values []PackageInventory) []PackageInventory {
	if values == nil {
		return nil
	}
	result := make([]PackageInventory, len(values))
	copy(result, values)
	return result
}
func cloneServiceInventories(values []ServiceInventory) []ServiceInventory {
	if values == nil {
		return nil
	}
	result := make([]ServiceInventory, len(values))
	copy(result, values)
	return result
}
func cloneConfigurationInventories(values []ConfigurationInventory) []ConfigurationInventory {
	if values == nil {
		return nil
	}
	result := make([]ConfigurationInventory, len(values))
	copy(result, values)
	return result
}
func cloneManagedFileObservations(values []ManagedFileObservation) []ManagedFileObservation {
	if values == nil {
		return nil
	}
	result := make([]ManagedFileObservation, len(values))
	copy(result, values)
	return result
}
func cloneValidatorObservations(values []ValidatorObservation) []ValidatorObservation {
	if values == nil {
		return nil
	}
	result := make([]ValidatorObservation, len(values))
	copy(result, values)
	return result
}
func clonePackageObservations(values []PackageObservation) []PackageObservation {
	if values == nil {
		return nil
	}
	result := make([]PackageObservation, len(values))
	copy(result, values)
	return result
}
func cloneServiceObservations(values []ServiceObservation) []ServiceObservation {
	if values == nil {
		return nil
	}
	result := make([]ServiceObservation, len(values))
	copy(result, values)
	return result
}
func cloneConfigurationObservations(values []ConfigurationObservation) []ConfigurationObservation {
	if values == nil {
		return nil
	}
	result := make([]ConfigurationObservation, len(values))
	copy(result, values)
	return result
}
func clonePacmanObservations(values []PacmanIntegrityObservation) []PacmanIntegrityObservation {
	if values == nil {
		return nil
	}
	result := make([]PacmanIntegrityObservation, len(values))
	copy(result, values)
	return result
}
func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	result := make([]string, len(values))
	copy(result, values)
	return result
}
