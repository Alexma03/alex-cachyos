package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type checkIsolationSpy struct {
	observations int
	networkCalls int
	mutations    int
}

func (s *checkIsolationSpy) Observe(context.Context, CheckRequest) (CheckSnapshot, error) {
	s.observations++
	return CheckSnapshot{}, nil
}

func (s *checkIsolationSpy) AuthorizeNetwork() { s.networkCalls++ }
func (s *checkIsolationSpy) Mutate()           { s.mutations++ }

func TestCheckUsesOnlyTheObservationPort(t *testing.T) {
	spy := &checkIsolationSpy{}
	report := NewChecker(spy).Check(context.Background(), CheckRequest{})
	if report.Drift || spy.observations != 1 || spy.networkCalls != 0 || spy.mutations != 0 {
		t.Fatalf("check escaped isolation: report=%#v spy=%#v", report, spy)
	}
}

func TestClassifyManagedFileChecksEvidenceBeforeOwnership(t *testing.T) {
	expected := ManagedFileInventory{Path: "/etc/example.conf", DesiredHash: "desired"}
	cases := []struct {
		name     string
		observed ManagedFileObservation
		want     DriftClass
	}{
		{
			name:     "unowned missing live hash is unknown",
			observed: ManagedFileObservation{Path: expected.Path, State: FileObserved},
			want:     DriftUnknown,
		},
		{
			name:     "unowned unsafe observation is unknown",
			observed: ManagedFileObservation{Path: expected.Path, State: FileUnsafe},
			want:     DriftUnknown,
		},
		{
			name:     "complete unowned observation is a conflict",
			observed: ManagedFileObservation{Path: expected.Path, State: FileObserved, LiveHash: "other"},
			want:     DriftUnmanagedConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyManagedFile(expected, tc.observed); got != tc.want {
				t.Fatalf("classification = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCheckClassifiesManagedFilesWithStablePrecedenceAndBackups(t *testing.T) {
	request := CheckRequest{
		ManagedFiles: []ManagedFileInventory{
			{Path: "/etc/desired.conf", DesiredHash: "desired", Ownership: OwnershipCreated},
			{Path: "/etc/post.conf", DesiredHash: "new", ReceiptAfterHash: "after", ReceiptBackup: "/state/receipt.bak", AdoptionBackup: "/state/adoption.bak", Ownership: OwnershipAdopted},
			{Path: "/etc/conflict.conf", DesiredHash: "desired", AdoptionBackup: "/state/adoption-conflict.bak"},
			{Path: "/etc/unsafe.conf", DesiredHash: "desired", Ownership: OwnershipCreated},
			{Path: "/etc/unreadable.conf", DesiredHash: "desired", Ownership: OwnershipCreated},
			{Path: "/etc/missing.conf", DesiredHash: "desired", Ownership: OwnershipCreated},
			{Path: "/etc/equal.conf", DesiredHash: "same", Ownership: OwnershipCreated},
		},
	}
	var observedRequest CheckRequest
	observer := CheckObserverFunc(func(_ context.Context, observed CheckRequest) (CheckSnapshot, error) {
		observedRequest = observed
		observed.ManagedFiles[0].DesiredHash = "callback-must-not-alias"
		return CheckSnapshot{ManagedFiles: []ManagedFileObservation{
			{Path: "/etc/equal.conf", State: FileObserved, LiveHash: "same"},
			{Path: "/etc/missing.conf", State: FileMissing},
			{Path: "/etc/unreadable.conf", State: FileUnreadable},
			{Path: "/etc/conflict.conf", State: FileObserved, LiveHash: "other"},
			{Path: "/etc/post.conf", State: FileObserved, LiveHash: "changed"},
			{Path: "/etc/unsafe.conf", State: FileUnsafe},
			{Path: "/etc/desired.conf", State: FileObserved, LiveHash: "old"},
		}}, nil
	})

	report := NewChecker(observer).Check(context.Background(), request)
	if observedRequest.ManagedFiles[0].DesiredHash != "callback-must-not-alias" {
		t.Fatal("observer did not receive an isolated request")
	}
	if request.ManagedFiles[0].DesiredHash != "desired" {
		t.Fatal("observer mutation aliased the caller request")
	}
	if !report.Drift || report.ExitIntent != ExitIntentNonZero {
		t.Fatalf("report = %#v, want drift and nonzero exit intent", report)
	}

	got := map[string]CheckFinding{}
	for _, finding := range report.Findings {
		got[finding.Path] = finding
	}
	want := map[string]struct {
		class  DriftClass
		backup string
	}{
		"/etc/conflict.conf":   {DriftUnmanagedConflict, "/state/adoption-conflict.bak"},
		"/etc/desired.conf":    {DriftDesiredState, "/etc/desired.conf" + backupSuffixForTest},
		"/etc/missing.conf":    {DriftDesiredState, "/etc/missing.conf" + backupSuffixForTest},
		"/etc/post.conf":       {DriftPostApply, "/state/receipt.bak"},
		"/etc/unreadable.conf": {DriftUnknown, "/etc/unreadable.conf" + backupSuffixForTest},
		"/etc/unsafe.conf":     {DriftUnknown, "/etc/unsafe.conf" + backupSuffixForTest},
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %#v, want %d managed-file findings", report.Findings, len(want))
	}
	for path, expected := range want {
		finding, ok := got[path]
		if !ok || finding.Class != expected.class || finding.Backup != expected.backup {
			t.Errorf("finding[%q] = %#v, want class=%q backup=%q", path, finding, expected.class, expected.backup)
		}
	}
	if _, ok := got["/etc/equal.conf"]; ok {
		t.Fatal("equal managed file was reported as drift")
	}
	for i := 1; i < len(report.Findings); i++ {
		if testFindingKey(report.Findings[i-1]) > testFindingKey(report.Findings[i]) {
			t.Fatalf("findings are not deterministic: %#v", report.Findings)
		}
	}
}

func TestCheckAggregatesInventoriesAndKeepsFingerprintWarnOnly(t *testing.T) {
	request := CheckRequest{
		Validators:      []ValidatorInventory{{Name: "niri"}, {Name: "noctalia"}},
		Packages:        []PackageInventory{{Name: "niri"}, {Name: "noctalia", Version: "2.0"}},
		Services:        []ServiceInventory{{Unit: "greetd.service", RequireEnabled: true, RequireActive: true}},
		Boot:            []ConfigurationInventory{{Name: "grub", DesiredHash: "boot-good"}},
		PAM:             []ConfigurationInventory{{Name: "polkit", DesiredHash: "pam-good"}},
		FailedUnits:     FailedUnitInventory{Check: true},
		PacmanIntegrity: PacmanIntegrityInventory{Checks: []string{PacmanDatabaseCheck, PacmanFilesCheck}},
		Pacnew:          PacnewInventory{Check: true},
		Fingerprint:     FingerprintInventory{Check: true},
	}
	snapshot := CheckSnapshot{
		Validators:      []ValidatorObservation{{Name: "noctalia", Status: CheckFailed}, {Name: "niri", Status: CheckPassed}},
		Packages:        []PackageObservation{{Name: "noctalia", Present: true, Version: "1.0"}, {Name: "niri", Present: false}},
		Services:        []ServiceObservation{{Unit: "greetd.service", Installed: true, Enabled: false, Active: true}},
		Boot:            []ConfigurationObservation{{Name: "grub", Status: CheckFailed}},
		PAM:             []ConfigurationObservation{{Name: "polkit", Status: CheckFailed}},
		FailedUnits:     []string{"broken.service"},
		PacmanIntegrity: []PacmanIntegrityObservation{{Name: PacmanFilesCheck, Status: CheckFailed}, {Name: PacmanDatabaseCheck, Status: CheckPassed}},
		Pacnew:          []string{"/etc/example.pacnew"},
		Fingerprint:     FingerprintObservation{Presence: FingerprintAbsent},
	}

	report := NewChecker(CheckObserverFunc(func(context.Context, CheckRequest) (CheckSnapshot, error) {
		return snapshot, nil
	})).Check(context.Background(), request)
	if !report.Drift || report.ExitIntent != ExitIntentNonZero {
		t.Fatalf("report = %#v, want nonzero drift", report)
	}
	for _, code := range []string{
		FindingValidatorFailed, FindingPackageMissing, FindingPackageVersion,
		FindingServiceState, FindingBootDrift, FindingPAMDrift, FindingFailedUnit,
		FindingPacmanIntegrity,
	} {
		if !hasFindingCode(report.Findings, code) {
			t.Errorf("missing aggregate finding code %q in %#v", code, report.Findings)
		}
	}
	if !hasWarning(report.Warnings, "fingerprint-enrollment-absent") || !hasWarning(report.Warnings, "pacnew:/etc/example.pacnew") {
		t.Fatalf("warnings = %#v", report.Warnings)
	}

	fingerprintOnly := NewChecker(CheckObserverFunc(func(context.Context, CheckRequest) (CheckSnapshot, error) {
		return CheckSnapshot{Fingerprint: FingerprintObservation{Presence: FingerprintAbsent}}, nil
	})).Check(context.Background(), CheckRequest{Fingerprint: FingerprintInventory{Check: true}})
	if fingerprintOnly.Drift || fingerprintOnly.ExitIntent != ExitIntentZero || len(fingerprintOnly.Findings) != 0 {
		t.Fatalf("fingerprint-only report = %#v, want warning-only success", fingerprintOnly)
	}
}

func TestCheckObserverErrorsFailClosedWithoutErrorOrByteContent(t *testing.T) {
	const secret = "observer-secret-output"
	report := NewChecker(CheckObserverFunc(func(context.Context, CheckRequest) (CheckSnapshot, error) {
		return CheckSnapshot{}, errors.New(secret)
	})).Check(context.Background(), CheckRequest{
		ManagedFiles: []ManagedFileInventory{{Path: "/etc/example", DesiredHash: "expected", Ownership: OwnershipCreated}},
		Validators:   []ValidatorInventory{{Name: "niri"}},
		Fingerprint:  FingerprintInventory{Check: true},
	})
	if !report.Drift || report.ExitIntent != ExitIntentNonZero {
		t.Fatalf("observer error report = %#v", report)
	}
	if !hasFindingClass(report.Findings, DriftUnknown) {
		t.Fatalf("observer error did not fail closed unknown: %#v", report.Findings)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "stdout") || strings.Contains(string(encoded), "stderr") {
		t.Fatalf("privacy-unsafe report: %s", encoded)
	}
	if !hasWarning(report.Warnings, "fingerprint-enrollment-unknown") {
		t.Fatalf("fingerprint observer error was not warning-only: %#v", report.Warnings)
	}
}

func TestCheckOutputIsDeterministicAndCopiesNilAndEmptyInventories(t *testing.T) {
	request := CheckRequest{
		ManagedFiles: []ManagedFileInventory{
			{Path: "/etc/z", DesiredHash: "z", Ownership: OwnershipCreated},
			{Path: "/etc/a", DesiredHash: "a", Ownership: OwnershipCreated},
		},
		Pacnew:      PacnewInventory{Check: true},
		Fingerprint: FingerprintInventory{Check: true},
	}
	firstSnapshot := CheckSnapshot{
		ManagedFiles: []ManagedFileObservation{
			{Path: "/etc/z", State: FileObserved, LiveHash: "old"},
			{Path: "/etc/a", State: FileObserved, LiveHash: "old"},
		},
		Pacnew:      []string{"/etc/z.pacnew", "/etc/a.pacnew"},
		Fingerprint: FingerprintObservation{Presence: FingerprintAbsent},
	}
	secondSnapshot := CheckSnapshot{
		ManagedFiles: []ManagedFileObservation{
			{Path: "/etc/a", State: FileObserved, LiveHash: "old"},
			{Path: "/etc/z", State: FileObserved, LiveHash: "old"},
		},
		Pacnew:      []string{"/etc/a.pacnew", "/etc/z.pacnew"},
		Fingerprint: FingerprintObservation{Presence: FingerprintAbsent},
	}
	makeObserver := func(snapshot CheckSnapshot) CheckObserver {
		return CheckObserverFunc(func(_ context.Context, observed CheckRequest) (CheckSnapshot, error) {
			if len(observed.ManagedFiles) != 2 || observed.ManagedFiles[0].Path != "/etc/z" {
				t.Fatalf("observer received unexpected request: %#v", observed)
			}
			return snapshot, nil
		})
	}
	first, second := NewChecker(makeObserver(firstSnapshot)).Check(context.Background(), request), NewChecker(makeObserver(secondSnapshot)).Check(context.Background(), request)
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("permuted observations changed report:\n%s\n%s", firstJSON, secondJSON)
	}

	var nilRequest CheckRequest
	nilClone := CloneCheckRequest(nilRequest)
	if nilClone.ManagedFiles != nil || nilClone.Validators != nil || nilClone.Packages != nil || nilClone.Services != nil || nilClone.Boot != nil || nilClone.PAM != nil || nilClone.FailedUnits.Check || nilClone.PacmanIntegrity.Checks != nil || nilClone.Pacnew.Check || nilClone.Fingerprint.Check {
		t.Fatalf("nil request was not preserved: %#v", nilClone)
	}
	emptyRequest := CheckRequest{ManagedFiles: []ManagedFileInventory{}, Validators: []ValidatorInventory{}, Boot: []ConfigurationInventory{}}
	emptyClone := CloneCheckRequest(emptyRequest)
	if emptyClone.ManagedFiles == nil || emptyClone.Validators == nil || emptyClone.Boot == nil {
		t.Fatalf("empty request slices were collapsed to nil: %#v", emptyClone)
	}
}

func TestCheckPublicInventoriesCarryNoBytesOrCommandOutput(t *testing.T) {
	for _, value := range []any{CheckRequest{}, CheckSnapshot{}, CheckReport{}, ManagedFileInventory{}, ManagedFileObservation{}, ValidatorObservation{}, PackageObservation{}, ServiceObservation{}, ConfigurationObservation{}, FingerprintObservation{}} {
		if typeContainsByteSlice(reflect.TypeOf(value), map[reflect.Type]bool{}) {
			t.Fatalf("%T exposes byte content", value)
		}
	}
}

func typeContainsByteSlice(typ reflect.Type, seen map[reflect.Type]bool) bool {
	if typ == nil || seen[typ] {
		return false
	}
	seen[typ] = true
	switch typ.Kind() {
	case reflect.Slice:
		return typ.Elem().Kind() == reflect.Uint8 || typeContainsByteSlice(typ.Elem(), seen)
	case reflect.Pointer:
		return typeContainsByteSlice(typ.Elem(), seen)
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if typeContainsByteSlice(typ.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

func hasFindingCode(findings []CheckFinding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}

func hasFindingClass(findings []CheckFinding, class DriftClass) bool {
	for _, finding := range findings {
		if finding.Class == class {
			return true
		}
	}
	return false
}

func hasWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

func testFindingKey(finding CheckFinding) string {
	return string(finding.Kind) + "\x00" + finding.Path + "\x00" + finding.Subject + "\x00" + string(finding.Class) + "\x00" + finding.Code + "\x00" + finding.Backup
}

const backupSuffixForTest = ".bak.alex-cachyos"
