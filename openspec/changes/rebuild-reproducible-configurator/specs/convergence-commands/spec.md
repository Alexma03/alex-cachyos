# Convergence Commands Specification

## Purpose

Semantics of the Go CLI command surface: `apply` (idempotent convergence),
`check` (non-mutating drift detection), `adopt` (detect-and-adopt),
`rollback` (inverse plan), `checkpoint` (annotated catalog tags), plus locking,
elevation, offline, and dry-run behavior. The Bash CLI surface is preserved during
the staged migration defined in `staged-migration`; worktree guarantees are defined
in `worktree-safety`; receipt persistence in `receipts`.

## Requirements

### Requirement: Preserved CLI surface

The Go CLI MUST provide `--host` (replacing `--profile`), `--only`, `--with`,
`--without`, `--remove`, `--dry-run`, `--check`, `--list`, and `-h/--help`.
`--check` MUST be equivalent to restricting execution to the verify module and MUST
NOT mutate the system.

#### Scenario: Help enumerates the preserved surface

- GIVEN the compiled binary
- WHEN `-h` is invoked
- THEN all preserved options are documented

#### Scenario: Check aliases verify-only and is non-mutating

- GIVEN a converged host
- WHEN `alex-cachyos --check` runs
- THEN only the verify module executes and no managed file, package, or service changes

### Requirement: Idempotent apply

Every mutating step MUST guard on a target-state check (installed-package query,
version compare, file hash compare, or service active check) and MUST be skipped
when the target state already holds. Re-running `apply` with no catalog change MUST
be a no-op that still emits a receipt marked `noChange: true`. The fingerprint
build MUST be skipped when the pinned commit and pkgrel are already installed.

#### Scenario: Second apply is a no-op

- GIVEN a host converged by a previous apply and an unchanged catalog
- WHEN `apply` runs again
- THEN no mutating step executes and the emitted receipt records `noChange: true`

#### Scenario: Satisfied package step is skipped

- GIVEN a catalog step installing a package that is already installed at the pinned version
- WHEN `apply` runs
- THEN the step is reported as satisfied and skipped without invoking the package manager

### Requirement: Single-instance lock

A mutating run MUST hold a `flock` lock under `$XDG_RUNTIME_DIR` scoped to the user
for the duration of the run. A concurrent invocation MUST exit with code 75 without
performing any mutation.

#### Scenario: Concurrent invocation exits 75

- GIVEN an `apply` run in progress holding the lock
- WHEN a second `apply` is invoked
- THEN the second invocation exits 75 and performs no mutation

### Requirement: Least-privilege elevation

Runs MUST execute as the normal user, and only system-mutating steps MUST be
elevated via `pkexec` (fingerprint polkit agent). The system MUST NOT invoke
`sudo`. User-scope steps MUST run unprivileged.

#### Scenario: System step elevates via pkexec only

- GIVEN a plan containing a system step and a user-scope step
- WHEN `apply` executes the plan
- THEN the system step runs through `pkexec` and the user-scope step runs unprivileged, and no step uses `sudo`

### Requirement: Non-mutating offline check

`check` MUST be non-mutating and offline-safe. It MUST perform drift detection
covering: `niri validate`, `noctalia config validate`, byte-comparison of managed
files against embedded templates, declared-package presence, greetd/boot/PAM
comparison, failed-unit checks, `pacman -Dk` and `pacman -Qk`, and `.pacnew`
warnings. It MUST report each drifted file together with its backup path.
Fingerprint verification MUST be limited to enrollment presence and MUST be
warn-only.

#### Scenario: Drift is reported with a backup path and no mutation

- GIVEN a managed file whose content differs from the embedded template
- WHEN `check` runs offline
- THEN the file is reported as drifted together with its `*.bak.alex-cachyos` backup path and no file is modified

#### Scenario: Fingerprint presence is warn-only

- GIVEN a host with no enrolled fingerprints
- WHEN `check` runs
- THEN a warning is reported and the check does not fail on enrollment absence, and no biometric data is read

### Requirement: Detect-and-adopt

`adopt` MUST detect existing unmanaged files that conflict with desired state and
adopt them by creating one-time `*.bak.alex-cachyos` backups recorded in an adoption
manifest. It MUST suggest a host profile based on hostname, and it MUST adopt
existing `gentle-ai` and `gentle-pi` checkouts instead of re-cloning them.

#### Scenario: Unmanaged conflicting file is adopted with a one-time backup

- GIVEN an existing file at a path the catalog manages, not previously managed by the configurator
- WHEN `adopt` runs
- THEN a `*.bak.alex-cachyos` backup is created exactly once and recorded in the adoption manifest

#### Scenario: Existing harness checkout is adopted

- GIVEN an existing `~/Projects/gentle-pi` checkout
- WHEN `adopt` runs
- THEN the checkout is adopted rather than re-cloned

### Requirement: Rollback via inverse plan

`rollback --receipt <id>` (or `--tag catalog-vX`) MUST replay the inverse of the
recorded plan, restoring `*.bak.alex-cachyos` backups recorded in the receipt; the
per-module `--remove` surface MUST restore that module's backups. System-level
rollback MUST remain delegated to Snapper: the configurator MUST NOT reimplement
system rollback. Rollback MUST honor the non-mutating worktree guarantees defined in
`worktree-safety`.

#### Scenario: Rollback restores a managed file

- GIVEN a receipt recording a managed file write with a backup
- WHEN `rollback --receipt <id>` runs
- THEN the backup content is restored and a new receipt records the rollback

#### Scenario: System-level rollback stays external

- GIVEN a rollback request that includes system-level state
- WHEN rollback executes
- THEN the configurator restores only file and configuration state and reports that system-level rollback is delegated to Snapper

### Requirement: Checkpoint command

`checkpoint create --tag catalog-vX.Y.Z -m <message>` MUST create an annotated Git
tag capturing the catalog snapshot, per the tag semantics defined in `catalog`.

#### Scenario: Checkpoint creates an annotated tag

- GIVEN a valid catalog in the configurator repository
- WHEN `checkpoint create --tag catalog-v1.0.0 -m "initial"` runs
- THEN an annotated tag `catalog-v1.0.0` exists with the provided message

### Requirement: Dry-run is a plan, not an execution

`--dry-run` MUST produce the full plan with step-level outcomes projected, grouped
network-required steps reported upfront, and zero mutations performed.

#### Scenario: Dry-run mutates nothing

- GIVEN a host with pending drift
- WHEN `apply --dry-run` runs
- THEN the planned steps are printed, network-required steps are grouped and reported, and the host state is byte-identical afterwards
