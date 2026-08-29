# Worktree Safety Specification

## Purpose

Dirty worktrees — in the configurator repository itself and in the adopted
`gentle-ai`/`gentle-pi` clones — hold pre-existing user work that MUST never be
destroyed, cleaned, stashed, or committed by the configurator. This capability
defines the refusal semantics for checkout updates and the non-mutating guarantees
for catalog reads and rollback. This implements `apply.preserve_dirty_worktree: true`
from `openspec/config.yaml`.

## Requirements

### Requirement: No destructive git operations on dirty trees

The configurator MUST NOT run `git checkout --force`, `git reset --hard`, `git
clean`, or stash/commit operations against the active configurator worktree or any
adopted harness clone. Updates to adopted checkouts MUST use `git fetch` followed
by a plain `git checkout` of the pinned branch or commit, which refuses when local
modifications overlap the update.

#### Scenario: Plain checkout refuses on dirty overlap

- GIVEN an adopted `gentle-pi` clone with local modifications to a file that the pinned update would change
- WHEN the configurator updates the checkout via fetch plus plain checkout
- THEN git refuses the switch and the local modifications are intact

#### Scenario: No force flags are ever emitted

- GIVEN any update or rollback path
- WHEN git commands are constructed
- THEN no command includes `--force`, `reset --hard`, or `clean`

### Requirement: Dirty-worktree pre-check and abort

Before updating any checkout, the configurator MUST run `git status --porcelain`
and, when the working tree is dirty and the pending checkout would clobber local
modifications, MUST abort that step with an actionable message, leave the worktree
intact, and record `dirty: true` (plus the skip) in the receipt. The porcelain
summary recorded MUST be limited to counts, not diff content.

#### Scenario: Dirty clone aborts the checkout step

- GIVEN a dirty `gentle-pi` clone whose pending checkout would clobber modifications
- WHEN `apply` reaches the checkout step
- THEN the step aborts with an actionable message, the worktree is byte-identical, and the receipt records `dirty: true` and `skipped: true`

#### Scenario: Dirty summary records counts only

- GIVEN a dirty clone at update time
- WHEN the receipt is inspected
- THEN the porcelain summary contains counts of modifications, not diff content

### Requirement: Non-mutating catalog reads for prior versions

Reading a prior catalog version for comparison, re-application, or rollback MUST use
non-mutating object reads (for example `git show <tag>:<path>` or `git cat-file`)
and MUST NOT run `git checkout <tag>` in the active worktree. When a prior version
must be applied, the configurator MUST materialize it in an isolated, detached
worktree and run from there; the active dirty worktree MUST remain untouched.

#### Scenario: Prior catalog is read without checkout

- GIVEN a prior annotated tag `catalog-v1.0.0` and a dirty active worktree
- WHEN rollback prepares to re-apply that version
- THEN the catalog content is read via a non-mutating object read and the active worktree status is unchanged

#### Scenario: Isolated worktree materializes a prior version

- GIVEN a request to apply catalog `catalog-v1.0.0` on a host whose active worktree is dirty
- WHEN the re-application runs
- THEN it executes from a detached isolated worktree and the active worktree is untouched

### Requirement: No Pi git-package reconciliation for harness clones

The `gentle-ai` and `gentle-pi` clones MUST NOT be managed as Pi `git:` packages,
because Pi reconciliation resets and cleans the clone. The configurator MUST manage
these clones directly through git.

#### Scenario: Harness clones are never registered as git packages

- GIVEN catalog entries for the two harness checkouts
- WHEN the plan is built
- THEN no step invokes Pi git-package reconciliation against either clone

### Requirement: Pre-existing user work is never migrated

The configurator MUST NOT clean, stash, commit, or otherwise "migrate" the
pre-existing uncommitted changes in the active `alex-cachyos` worktree. `apply`,
`check`, and `rollback` MUST leave those changes intact.

#### Scenario: Apply leaves the configurator worktree dirty state untouched

- GIVEN the `alex-cachyos` repository with pre-existing uncommitted user work
- WHEN `apply`, `check`, and `rollback` each run
- THEN `git status` before and after shows the same set of uncommitted changes
