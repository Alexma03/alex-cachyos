# Acceptance Verification Specification

## Purpose

Defines the observable, fixture-first acceptance behavior for the change as a
whole, mapping to the proposal's success criteria. Hardware-dependent verification
is gated on an explicitly declared integration target; a generic VM is never assumed
safe for applying the `galaxy` host profile.

## Requirements

### Requirement: Fixture-first convergence proof

The planner, precedence resolution, exact-pin resolution, no-op re-apply
(`noChange: true`), and dependency ordering MUST be provable via `go test ./...`
with a fake runner, without requiring `galaxy` hardware. The same checks MUST be
asserted by the fixture for `check`/`--dry-run` behavior.

#### Scenario: Full fixture suite passes

- GIVEN the Go module with the planner and fake runner
- WHEN `go test ./...` runs
- THEN the fixture asserts the `galaxy` dry-run plan, ordering invariants, precedence, exact-pin resolution, and the no-change second apply

### Requirement: Hardware verification gated on an explicit target

Hardware-dependent verification (real `niri validate`, `noctalia config validate`,
`pacman -Dk`/`-Qk`, real package installs) MUST run only on the designated `galaxy`
host or an explicitly declared integration target. Passing the fixture suite MUST
NOT be reported as hardware convergence.

#### Scenario: Fixture success is not claimed as hardware convergence

- GIVEN a passing fixture suite on a development machine that is not the declared integration target
- WHEN convergence status is reported
- THEN it distinguishes fixture validation from hardware convergence and does not claim the latter

### Requirement: Post-convergence drift-free check

On the integration target after convergence, `alex-cachyos check` MUST report zero
drift, with `niri validate` and `noctalia config validate` passing and `pacman
-Dk`/`-Qk` clean.

#### Scenario: Check reports zero drift after convergence

- GIVEN the integration target converged by `apply`
- WHEN `check` runs
- THEN it reports zero drift and the platform validators pass

### Requirement: Pi environment acceptance

After convergence: `pi list` MUST show all catalog exact pins at their correct
versions with the local `gentle-pi` path package resolved; receipts MUST
distinguish desired exact pins from resolved versions; both harness checkouts MUST
be at their `main` pins or tips with exact commits recorded; the package-local
Gentle AI binary MUST reflect `main` (verified via `managed-assets.json` and binary
version); the 23 routes MUST render identically in both files; lean/task values
MUST be set; `AGENTS.md`/`APPEND_SYSTEM.md` ownership MUST hold; review mode MUST
report effective on with deciding global; web-search config MUST contain only
reference-form credentials; and the temporary override MUST be present with its
retirement predicate encoded.

#### Scenario: Pi package list matches the catalog

- GIVEN a converged machine
- WHEN `pi list` output is compared to the catalog
- THEN every catalog exact pin appears at its exact version and the local path package is resolved

#### Scenario: Local-main runtime actually runs main

- GIVEN a converged machine
- WHEN the package-local Gentle AI binary version and `managed-assets.json` are inspected
- THEN they reflect the local `main` build rather than the stale package-local baseline version

#### Scenario: Review mode reports the user's choice

- GIVEN a converged machine
- WHEN `gentle-ai review mode status --cwd <repo>` runs
- THEN it reports `effective: on` and `deciding: global`

### Requirement: Worktree and receipt acceptance

Acceptance MUST include: the active `alex-cachyos` worktree's pre-existing dirty
state is unchanged by `apply`/`check`/`rollback`; a dirty harness clone aborts its
checkout step with `dirty: true` and `skipped: true` recorded; receipts exist one
per run under the XDG state directory with an atomic current index; and an annotated
`catalog-v*` tag exists and is recorded in receipts.

#### Scenario: Dirty state survives all three commands

- GIVEN the `alex-cachyos` repository with pre-existing uncommitted work
- WHEN `apply`, `check`, and `rollback` run in sequence
- THEN `git status` reports the same dirty state before and after

#### Scenario: Receipts and tag acceptance

- GIVEN a converged integration target
- WHEN receipts and tags are inspected
- THEN each run has exactly one immutable receipt, the current index resolves to the latest, and the applied catalog tag is recorded
