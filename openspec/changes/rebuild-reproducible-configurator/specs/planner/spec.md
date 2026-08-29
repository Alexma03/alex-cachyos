# Planner Specification

## Purpose

The shared planner converts the merged catalog plus CLI selection flags into a
deterministic, dependency-ordered execution plan, replacing `MODULE_ORDER` and
`ao_should_run_module`. Planner behavior MUST be provable with fake-runner fixtures
so convergence logic is validated without `galaxy` hardware or an unsafe
generic-VM apply. Command semantics are defined in `convergence-commands`.

## Requirements

### Requirement: Dependency-ordered module graph

The planner MUST order steps through an explicit `dependsOn` graph that enforces the
invariants: `bootstrap` runs first, `verify` runs last, and `apps` runs before
`desktop`; the full order is `bootstrap → fingerprint/devtools/apps/vicinae →
desktop → verify`. The planner MUST reject cyclic or unsatisfiable dependencies at
plan time, before any mutation.

#### Scenario: Plan respects ordering invariants

- GIVEN the merged catalog for host `galaxy` with all modules enabled
- WHEN the planner produces a plan
- THEN `bootstrap` precedes all other modules, `verify` follows all other modules, and `apps` precedes `desktop`

#### Scenario: Cyclic dependency is rejected

- GIVEN a catalog where module A declares `dependsOn: B` and module B declares `dependsOn: A`
- WHEN the planner builds the plan
- THEN planning fails with a cycle error and no step executes

### Requirement: Module selection precedence

The planner MUST apply selection in strict precedence order: `--only` wins over
`--with`/`--without`, and `--with`/`--without` win over profile defaults.

#### Scenario: Only wins over profile defaults and with

- GIVEN a profile that enables modules A, B, and C, and a run with `--only A` and `--with B`
- WHEN the planner selects modules
- THEN only module A is selected

#### Scenario: Without removes a profile-default module

- GIVEN a profile that enables modules A and B, and a run with `--without B`
- WHEN the planner selects modules
- THEN module A is selected and module B is excluded

### Requirement: Deterministic planning

For the same catalog content, host, and selection flags, the planner MUST produce an
identical plan on repeated invocations, so fixtures can assert exact plan structure.

#### Scenario: Repeated planning yields an identical plan

- GIVEN an unchanged catalog, host, and flags
- WHEN the planner is invoked twice
- THEN both invocations produce structurally identical plans

### Requirement: Fixture-based validation without hardware

Planner and plan execution MUST be exercisable through a fake runner under
`go test ./...` without requiring the `galaxy` host. Hardware integration MUST be
gated on an explicitly declared integration target; a generic VM MUST NOT be assumed
to safely apply the `galaxy` host profile.

#### Scenario: Fixture proves the dry-run plan for galaxy

- GIVEN a fixture environment with a fake runner and the `galaxy` host catalog
- WHEN `alex-cachyos apply --host galaxy --dry-run` is exercised in tests
- THEN the fixture asserts the expected dependency order, `global → role → host` precedence resolution, and exact-pin resolution

#### Scenario: Unchanged catalog yields a no-op plan

- GIVEN a fixture where a first apply has converged and the catalog is unchanged
- WHEN apply is planned again
- THEN the plan contains no mutating steps and the run is marked `noChange`

### Requirement: Network-required step grouping

The planner MUST identify network-required steps (pacman, AUR builds, remote icon
fetches) and group them so a dry run reports them upfront; file, template, and
config steps MUST be plannable offline.

#### Scenario: Network steps are reported upfront in a dry run

- GIVEN a plan containing package installation and file rendering steps
- WHEN `--dry-run` runs while offline
- THEN the dry run completes without mutation and lists the network-required steps as a group
