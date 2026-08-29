# Staged Migration Specification

## Purpose

The Bash implementation remains the authoritative, functional configurator while
the Go CLI is built and validated alongside it. Cutover is a later, explicit,
evidence-gated decision; this change's first slice MUST NOT delete or break the
Bash path. This capability preserves reviewability and a rollback fallback.

## Requirements

### Requirement: Bash implementation preserved during migration

While the Go CLI is under development and validation, the existing Bash `./apply`
with `lib/`, `modules/`, `templates/`, `overlays/`, and `packaging/` MUST remain
present and functional, and MUST continue to be the authority for production
convergence until Go validation passes.

#### Scenario: Bash path still works during the transition

- GIVEN the repository with both the preserved Bash tree and the new Go module
- WHEN `bash -n` runs over the Bash entrypoint, libraries, and modules
- THEN syntax validation passes and the Bash entrypoint remains executable

### Requirement: Go CLI developed alongside with embedded copies

The Go CLI MUST be developed in a new module (`cmd/alex-cachyos` plus `internal/*`)
without deleting Bash files. Embedded assets MUST be copies under the Go module
(`go:embed`); the Go binary reads the embedded copies while Bash continues to read
the original tree, so neither path mutates the other's inputs.

#### Scenario: Go binary uses embedded assets while Bash uses originals

- GIVEN the repository containing both the original `templates/` tree and embedded Go copies
- WHEN the Go binary renders a template and the Bash path renders the same template
- THEN each reads its own asset source and neither writes into the other's tree

### Requirement: Cutover gated on validation evidence

A cutover proposal (making Go the default path) MUST NOT be made until the planner
fixture suite passes, `check` and `--dry-run` produce expected results on an
explicit integration target, and receipt and manifest verification pass. Until
then, the Bash path MUST remain the documented default.

#### Scenario: Cutover is blocked without fixture evidence

- GIVEN a Go CLI whose planner fixture suite has not passed
- WHEN migration status is assessed
- THEN the Bash path remains the documented default and no cutover is proposed

### Requirement: Transition testing configuration

During the transition, `openspec/config.yaml` MUST retain the Bash syntax and JSON
validation commands, and MUST be updated to include the `go test ./...` runner once
Go tests exist, so both paths stay continuously validated.

#### Scenario: Config carries both runners during transition

- GIVEN the repository mid-migration with Go tests landed
- WHEN `openspec/config.yaml` is inspected
- THEN both the Bash syntax commands and `go test ./...` are present as validation commands

### Requirement: Bash rollback fallback

The staged migration MUST preserve the Bash-based rollback mechanisms
(`*.bak.alex-cachyos` file restore and Snapper for system state) as the fallback
while the Go path is being validated.

#### Scenario: Bash fallback rollback remains available

- GIVEN a host converged during the migration period
- WHEN a rollback is needed before Go validation completes
- THEN the Bash restore path and Snapper remain usable and documented
