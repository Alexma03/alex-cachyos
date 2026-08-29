package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func live(exists bool, hash string) LiveFile { return LiveFile{Exists: exists, Hash: hash} }

func TestCreatedRemoveNoOpAndMismatch(t *testing.T) {
	const target = "/etc/alex-cachyos/example.conf"
	after := hash("tool-written")

	t.Run("match removes", func(t *testing.T) {
		inv, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: after, Live: live(true, after)})
		if err != nil {
			t.Fatal(err)
		}
		if inv.Action != InverseRemoveFile || inv.Target != target || inv.Backup != "" || inv.PackageRestore != nil {
			t.Fatalf("inverse = %+v", inv)
		}
	})

	t.Run("absent is no-op", func(t *testing.T) {
		inv, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: after, Live: live(false, "")})
		if err != nil || inv.Action != InverseNoOp {
			t.Fatalf("inverse = %+v, %v", inv, err)
		}
	})

	t.Run("mismatch refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: after, Live: live(true, hash("edited"))})
		assertConflict(t, err, ConflictLiveMismatch)
	})
}

func TestCreatedRejectsInvalidInputs(t *testing.T) {
	const target = "/etc/example"
	after := hash("after")
	cases := []struct {
		name string
		in   ManagedFileRollback
		kind ConflictKind
	}{
		{"unknown class", ManagedFileRollback{Class: "other", Target: target, AfterHash: after, Live: live(true, after)}, ConflictInvalidClass},
		{"empty path", ManagedFileRollback{Class: ManagedCreated, AfterHash: after, Live: live(true, after)}, ConflictInvalidPath},
		{"relative path", ManagedFileRollback{Class: ManagedCreated, Target: "etc/example", AfterHash: after, Live: live(true, after)}, ConflictInvalidPath},
		{"empty after hash", ManagedFileRollback{Class: ManagedCreated, Target: target, Live: live(true, after)}, ConflictInvalidHash},
		{"short after hash", ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: "abc", Live: live(true, after)}, ConflictInvalidHash},
		{"upper after hash", ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: strings.ToUpper(after), Live: live(true, after)}, ConflictInvalidHash},
		{"invalid live hash", ManagedFileRollback{Class: ManagedCreated, Target: target, AfterHash: after, Live: live(true, "not-a-hash")}, ConflictInvalidHash},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PlanRollbackInverse(tc.in)
			assertConflict(t, err, tc.kind)
		})
	}
}

func TestAdoptedRestoresBackupOnlyUnderPreconditions(t *testing.T) {
	const target = "/etc/alex-cachyos/adopted.conf"
	const backup = "/etc/alex-cachyos/adopted.conf.bak.alex-cachyos"
	after := hash("tool-written")
	backupHash := hash("original")

	t.Run("valid restores", func(t *testing.T) {
		inv, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: backup, BackupHash: backupHash, Live: live(true, after)})
		if err != nil {
			t.Fatal(err)
		}
		if inv.Action != InverseRestoreBackup || inv.Target != target || inv.Backup != backup || inv.PackageRestore != nil {
			t.Fatalf("inverse = %+v", inv)
		}
	})

	t.Run("live mismatch refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: backup, BackupHash: backupHash, Live: live(true, hash("edited"))})
		assertConflict(t, err, ConflictLiveMismatch)
	})

	t.Run("live missing refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: backup, BackupHash: backupHash, Live: live(false, "")})
		assertConflict(t, err, ConflictLiveMissing)
	})

	for _, tc := range []struct {
		name string
		in   ManagedFileRollback
		kind ConflictKind
	}{
		{"missing backup path", ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupHash: backupHash, Live: live(true, after)}, ConflictBackupMissing},
		{"missing backup hash", ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: backup, Live: live(true, after)}, ConflictBackupMissing},
		{"invalid backup hash", ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: backup, BackupHash: "deadbeef", Live: live(true, after)}, ConflictInvalidBackup},
		{"invalid backup path", ManagedFileRollback{Class: ManagedAdopted, Target: target, AfterHash: after, BackupPath: "relative/backup", BackupHash: backupHash, Live: live(true, after)}, ConflictInvalidBackup},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PlanRollbackInverse(tc.in)
			assertConflict(t, err, tc.kind)
		})
	}
}

func TestPackageOwnedEmitsPackageRestoreNeverClaimsBackup(t *testing.T) {
	const target = "/usr/share/package-owned.conf"
	after := hash("package-owned")

	t.Run("match emits package restore", func(t *testing.T) {
		inv, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedPackageOwned, Target: target, AfterHash: after, Live: live(true, after)})
		if err != nil {
			t.Fatal(err)
		}
		if inv.Action != InversePackageRestore || inv.Backup != "" || inv.PackageRestore == nil || inv.PackageRestore.Target != target {
			t.Fatalf("inverse = %+v", inv)
		}
	})

	var wantConflict string
	for _, tc := range []struct {
		name, backupPath, backupHash string
	}{
		{"path only", target + ".bak", ""},
		{"hash only", "", hash("backup")},
		{"both malformed", "relative", "deadbeef"},
	} {
		t.Run(tc.name+" backup rejected", func(t *testing.T) {
			inv, err := PlanRollbackInverse(ManagedFileRollback{
				Class: ManagedPackageOwned, Target: target, AfterHash: after,
				BackupPath: tc.backupPath, BackupHash: tc.backupHash, Live: live(true, after),
			})
			if inv != (RollbackInverse{}) {
				t.Fatalf("inverse = %+v, want zero value", inv)
			}
			assertConflict(t, err, ConflictBackupNotAllowed)
			for _, value := range []string{tc.backupPath, tc.backupHash} {
				if value != "" && strings.Contains(err.Error(), value) {
					t.Fatalf("error %q leaks contradictory backup authority %q", err, value)
				}
			}
			if wantConflict == "" {
				wantConflict = err.Error()
			} else if err.Error() != wantConflict {
				t.Fatalf("error = %q, want deterministic %q", err, wantConflict)
			}
		})
	}

	t.Run("mismatch refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedPackageOwned, Target: target, AfterHash: after, Live: live(true, hash("edited"))})
		assertConflict(t, err, ConflictLiveMismatch)
	})

	t.Run("missing refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedPackageOwned, Target: target, AfterHash: after, Live: live(false, "")})
		assertConflict(t, err, ConflictLiveMissing)
	})

	t.Run("invalid live hash refuses", func(t *testing.T) {
		_, err := PlanRollbackInverse(ManagedFileRollback{Class: ManagedPackageOwned, Target: target, AfterHash: after, Live: live(true, "not-a-hash")})
		assertConflict(t, err, ConflictInvalidHash)
	})
}

func TestRollbackConflictIsPrivacySafeAndDeterministic(t *testing.T) {
	after := hash("recorded")
	liveHash := hash("live edited")
	in := ManagedFileRollback{Class: ManagedCreated, Target: "/etc/example", AfterHash: after, Live: live(true, liveHash)}

	first, err := PlanRollbackInverse(in)
	if first.Action != "" || err == nil {
		t.Fatalf("first = %+v, %v; want conflict", first, err)
	}
	var conflict *RollbackConflict
	if !errors.As(err, &conflict) || conflict.Kind != ConflictLiveMismatch {
		t.Fatalf("error = %v, want ConflictLiveMismatch RollbackConflict", err)
	}
	if msg := err.Error(); strings.Contains(msg, after) || strings.Contains(msg, liveHash) {
		t.Fatalf("error %q leaks a file hash", msg)
	}
	if !errors.Is(err, ErrRollbackConflict) {
		t.Fatalf("error = %v, want ErrRollbackConflict", err)
	}

	second, err2 := PlanRollbackInverse(in)
	if err2 == nil || err2.Error() != err.Error() || second.Action != first.Action {
		t.Fatalf("non-deterministic: first=%+v/%v second=%+v/%v", first, err, second, err2)
	}
}

func assertConflict(t *testing.T, err error, kind ConflictKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s conflict, got nil", kind)
	}
	var conflict *RollbackConflict
	if !errors.As(err, &conflict) || conflict.Kind != kind {
		t.Fatalf("conflict kind = %v, want %q (error %v)", err, kind, err)
	}
}
