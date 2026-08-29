# Pi Source Checkouts Specification

## Purpose

The development channel is the desired state: target machines consume the `main`
branches of the local `gentle-ai` and `gentle-pi` checkouts, not a stable or RC
release, which are evidence and baselines only. This capability covers cloning or
adopting the checkouts, non-destructive updates, the exact-commit receipt record,
and the integrity-preserving local-main package-runtime handoff that closes the
current gap where the gentle-pi `main` postinstall still provisions a package-local
Gentle AI `v2.4.0` binary. Worktree refusal semantics are normative in
`worktree-safety`; receipts in `receipts`.

## Requirements

### Requirement: Development-channel authority

The desired harness authority MUST be the `main` branches of the local
`~/Projects/gentle-ai` and `~/Projects/gentle-pi` checkouts. Stable and RC releases
(including Gentle AI `v2.4.0` and `v2.5.0-rc.1`, and Gentle Pi `2.2.0`) MUST be
treated as evidence and baselines for installer behavior only, never as the desired
state.

#### Scenario: Plan targets the main checkouts

- GIVEN the merged catalog
- WHEN the harness-checkout steps are planned
- THEN the plan targets the local `main` checkouts and no step pins a stable or RC release as the desired runtime

### Requirement: Clone-if-missing, adopt-if-present

For each harness checkout, the configurator MUST clone it when missing and MUST
adopt the existing checkout when present, updating it via `git fetch` followed by a
plain `git checkout` of the pinned commit or `main` tip that refuses on dirty
overlap, per `worktree-safety`.

#### Scenario: Missing clone is created

- GIVEN a fresh machine with no `~/Projects/gentle-pi`
- WHEN the checkout step runs
- THEN the repository is cloned and checked out at the catalog pin or `main` tip

#### Scenario: Existing clone is adopted and updated non-destructively

- GIVEN an existing `~/Projects/gentle-ai` checkout on `main` with a clean tree
- WHEN the checkout step runs
- THEN the checkout is adopted, fetched, and checked out at the pinned tip without any force operation

### Requirement: Exact resolved commit recorded

Receipts MUST record, for each harness checkout: `git rev-parse HEAD`, a porcelain
summary limited to counts, whether the clone was adopted or freshly cloned, and the
package-local binary version and manifest hash.

#### Scenario: Receipt records the resolved commit

- GIVEN a completed run that updated both checkouts
- WHEN the receipt is parsed
- THEN it contains the exact `HEAD` commit for each checkout and the adopted-versus-cloned flag

### Requirement: Integrity-preserving local-main runtime handoff

The configurator MUST provide a tested build-and-install path that makes the
package-local Gentle AI runtime actually consume the local `gentle-ai` `main`
source, preserving the same signature, SHA-256, and manifest verification the
gentle-pi installer enforces. The path MUST be verified by checking the
package-local binary version and `managed-assets.json` integrity after install.
A plain checkout of `main` MUST NOT be reported as sufficient by itself, because
the current `gentle-pi` postinstall provisions the package-local Gentle AI
`v2.4.0` binary and the runtime rejects PATH, global, sibling, and symlink
fallbacks.

#### Scenario: Local-main build is installed and verified

- GIVEN adopted `gentle-ai` and `gentle-pi` `main` checkouts
- WHEN the local-main build-and-install path runs
- THEN the package-local Gentle AI binary reflects the `main` build rather than the stale `v2.4.0` baseline, verified via the binary version and `managed-assets.json`

#### Scenario: Checkout alone is not claimed as sufficient

- GIVEN a run where only the checkouts were updated and the local-main install path was not executed or failed
- WHEN the run reports runtime state
- THEN it does not claim the package-local runtime consumes `main`

#### Scenario: Skip-install escape hatch fails closed

- GIVEN a run performed with `GENTLE_PI_SKIP_GENTLE_AI_INSTALL=1`
- WHEN native review operations are attempted
- THEN they fail closed with `package-local-binary-missing` and the configurator does not report the run as converged

### Requirement: Post-install verification

After the postinstall or local-main install path, the configurator MUST verify the
package-local binary version and `managed-assets.json` integrity, and MUST run
`pnpm install` for `gentle-pi` with the postinstall semantics intact (or via the
local-main path).

#### Scenario: Post-install integrity is verified

- GIVEN a completed harness install
- WHEN verification runs
- THEN the package-local binary version and the `managed-assets.json` manifest both check out and the outcome is recorded in the receipt
