# Receipts Specification

## Purpose

Every configurator run emits exactly one immutable JSON receipt under the
XDG-compliant state directory, with an atomic current index. Receipts are the audit
record of what was applied, desired, resolved, and skipped; they never contain
credential values, biometric data, or worktree diff content. Cross-cutting secret
and biometric bans are normative in `security-boundaries`.

## Requirements

### Requirement: One immutable receipt per run

Every `apply`, `check`, `adopt`, and `rollback` run MUST write exactly one JSON
receipt at `$XDG_STATE_HOME/alex-cachyos/receipts/<timestamp>-<shortHash>.json`
(defaulting to `$HOME/.local/state/alex-cachyos/receipts/` when `$XDG_STATE_HOME`
is unset or empty). Receipts MUST be immutable: subsequent runs MUST NOT modify or
delete earlier receipts.

#### Scenario: A receipt is written for each run

- GIVEN two consecutive configurator runs
- WHEN both complete
- THEN two distinct receipt files exist, each matching the naming pattern

#### Scenario: Later runs never rewrite earlier receipts

- GIVEN an existing receipt file from a prior run
- WHEN a new run completes
- THEN the prior receipt file is byte-identical to before and a new receipt file exists

### Requirement: Atomic current index

The system MUST maintain a `current` index (a symlink or pointer file) that resolves
to the most recent receipt, and MUST update it atomically so a reader never observes
a partial or dangling intermediate state.

#### Scenario: Index update is atomic

- GIVEN a `current` index pointing at receipt A and a completed run producing receipt B
- WHEN the index is updated
- THEN a concurrent reader observes either A or B, never a partial pointer

### Requirement: Receipt content schema

Receipts MUST record: executed and skipped steps with outcomes, managed-file hashes,
desired exact pins and resolved installed versions as distinct fields, credential
names only, resolved checkout commits, the applied catalog tag, the host, and a
`noChange` flag. Receipts MUST NOT contain credential values, biometric data, or
the diff content of dirty worktrees.

#### Scenario: Desired and resolved versions are distinct fields

- GIVEN a run that installed Pi packages
- WHEN its receipt is parsed
- THEN desired exact pins and resolved installed versions appear as separate, individually assertable fields

#### Scenario: Receipts contain credential names only

- GIVEN a run that configured web-search credential references
- WHEN the receipt is scanned
- THEN only credential names appear and no credential value is present

### Requirement: XDG state compliance

Configurator-owned state MUST follow the XDG Base Directory Specification 0.8:
receipts under `$XDG_STATE_HOME`, the run lock under `$XDG_RUNTIME_DIR`, and caches
under `$XDG_CACHE_HOME`, with spec-defined defaults when variables are unset or
empty. The configurator MUST NOT migrate, relocate, or expect XDG compliance from
Pi's own `~/.pi` state.

#### Scenario: Defaults apply when XDG variables are unset

- GIVEN a host where `$XDG_STATE_HOME` is unset or empty
- WHEN a run emits a receipt
- THEN the receipt is written under `$HOME/.local/state/alex-cachyos/receipts/`

#### Scenario: Override is honored without touching Pi state

- GIVEN `$XDG_STATE_HOME` set to an absolute path
- WHEN a run emits a receipt
- THEN the receipt is written under that path
- AND no file under `~/.pi` is moved or migrated by the configurator
