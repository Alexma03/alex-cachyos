// Pure inverse-precondition planning for managed-file rollback. It converts
// recorded managed-file state plus a live observation into a typed inverse
// action or a typed conflict. It performs no filesystem or OS mutation and
// never reads file bytes: hashes are supplied by the caller.
package app

import (
	"errors"
	"fmt"
	"path/filepath"
)

type ManagedClass string

const (
	ManagedCreated      ManagedClass = "created"       // tool-created file
	ManagedAdopted      ManagedClass = "adopted"       // adopted user file
	ManagedPackageOwned ManagedClass = "package-owned" // package-owned file
)

type InverseAction string

const (
	InverseRemoveFile     InverseAction = "remove-file"
	InverseRestoreBackup  InverseAction = "restore-backup"
	InversePackageRestore InverseAction = "package-restore"
	InverseNoOp           InverseAction = "no-op"
)

type LiveFile struct {
	Exists bool
	Hash   string
}

type PackageRestore struct {
	Target string
}

type ManagedFileRollback struct {
	Class      ManagedClass
	Target     string
	AfterHash  string
	BackupPath string
	BackupHash string
	Live       LiveFile
}

type RollbackInverse struct {
	Action         InverseAction
	Target         string
	Backup         string
	PackageRestore *PackageRestore
}

type ConflictKind string

const (
	ConflictLiveMismatch     ConflictKind = "live-hash-mismatch"
	ConflictLiveMissing      ConflictKind = "live-target-missing"
	ConflictBackupMissing    ConflictKind = "backup-missing"
	ConflictInvalidBackup    ConflictKind = "invalid-backup"
	ConflictBackupNotAllowed ConflictKind = "backup-not-allowed"
	ConflictInvalidClass     ConflictKind = "invalid-class"
	ConflictInvalidPath      ConflictKind = "invalid-path"
	ConflictInvalidHash      ConflictKind = "invalid-hash"
)

var ErrRollbackConflict = errors.New("rollback precondition conflict")

// RollbackConflict reports a fail-closed refusal. It names the kind and target,
// never a file hash or file bytes.
type RollbackConflict struct {
	Kind   ConflictKind
	Target string
}

func (e *RollbackConflict) Error() string {
	if e == nil {
		return ErrRollbackConflict.Error()
	}
	msg := map[ConflictKind]string{
		ConflictLiveMismatch:     "live content differs from the recorded after-state",
		ConflictLiveMissing:      "live target is missing",
		ConflictBackupMissing:    "required one-time backup identity or hash is missing",
		ConflictInvalidBackup:    "backup path or hash is invalid",
		ConflictBackupNotAllowed: "package-owned file must not claim backup authority",
		ConflictInvalidClass:     "unknown managed-file class",
		ConflictInvalidPath:      "target path is not absolute",
		ConflictInvalidHash:      "recorded or observed hash is not a lowercase SHA-256 digest",
	}[e.Kind]
	if msg == "" {
		msg = string(e.Kind)
	}
	return fmt.Sprintf("rollback refused for %q: %s", e.Target, msg)
}

func (e *RollbackConflict) Unwrap() error { return ErrRollbackConflict }

// PlanRollbackInverse validates recorded class, target, hashes, and live state,
// returning a typed inverse action or a typed conflict before any mutation.
// Package-owned files always produce package-restore semantics and never use or
// claim backup authority.
func PlanRollbackInverse(in ManagedFileRollback) (RollbackInverse, error) {
	conflict := func(kind ConflictKind) (RollbackInverse, error) {
		return RollbackInverse{}, &RollbackConflict{Kind: kind, Target: in.Target}
	}
	if !validPath(in.Target) {
		return conflict(ConflictInvalidPath)
	}
	if !validSHA256(in.AfterHash) {
		return conflict(ConflictInvalidHash)
	}

	switch in.Class {
	case ManagedCreated:
		if !in.Live.Exists {
			return RollbackInverse{Action: InverseNoOp, Target: in.Target}, nil
		}
		if !validSHA256(in.Live.Hash) {
			return conflict(ConflictInvalidHash)
		}
		if in.Live.Hash != in.AfterHash {
			return conflict(ConflictLiveMismatch)
		}
		return RollbackInverse{Action: InverseRemoveFile, Target: in.Target}, nil

	case ManagedAdopted:
		if in.BackupPath == "" || in.BackupHash == "" {
			return conflict(ConflictBackupMissing)
		}
		if !validPath(in.BackupPath) || !validSHA256(in.BackupHash) {
			return conflict(ConflictInvalidBackup)
		}
		if !in.Live.Exists {
			return conflict(ConflictLiveMissing)
		}
		if !validSHA256(in.Live.Hash) {
			return conflict(ConflictInvalidHash)
		}
		if in.Live.Hash != in.AfterHash {
			return conflict(ConflictLiveMismatch)
		}
		return RollbackInverse{Action: InverseRestoreBackup, Target: in.Target, Backup: in.BackupPath}, nil

	case ManagedPackageOwned:
		if in.BackupPath != "" || in.BackupHash != "" {
			return conflict(ConflictBackupNotAllowed)
		}
		if !in.Live.Exists {
			return conflict(ConflictLiveMissing)
		}
		if !validSHA256(in.Live.Hash) {
			return conflict(ConflictInvalidHash)
		}
		if in.Live.Hash != in.AfterHash {
			return conflict(ConflictLiveMismatch)
		}
		return RollbackInverse{
			Action:         InversePackageRestore,
			Target:         in.Target,
			PackageRestore: &PackageRestore{Target: in.Target},
		}, nil

	default:
		return conflict(ConflictInvalidClass)
	}
}

func validPath(path string) bool { return path != "" && filepath.IsAbs(path) }

func validSHA256(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
