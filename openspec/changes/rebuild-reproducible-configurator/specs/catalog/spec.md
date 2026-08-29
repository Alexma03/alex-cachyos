# Catalog Specification

## Purpose

The catalog is the versioned, typed, validated desired-state source of truth that
replaces `profiles/*.json` with `python3`/`eval` parsing and ad-hoc package-list
parsing. This capability defines schema validation, `global → role → host` merge
precedence, host resolution, exact-version pinning with the desired-versus-resolved
distinction, embedded-asset rendering, and `catalog-v*` checkpoint tag semantics.
The `planner`, `convergence-commands`, `receipts`, `pi-packages`, and
`pi-source-checkouts` capabilities build on it.

## Requirements

### Requirement: Versioned schema with pre-mutation validation

The system MUST define a versioned catalog schema identified by `catalogVersion` and
MUST validate every catalog load against a JSON Schema plus typed Go validation
before any mutation is planned or executed. Validation MUST reject unsupported
`catalogVersion` values, unknown module names, invalid package lists, references to
missing overlay/template assets, and invalid checkout pins.

#### Scenario: Invalid catalog is rejected before any mutation

- GIVEN a catalog with an unsupported `catalogVersion` or content that fails schema validation
- WHEN any command loads the catalog
- THEN the command exits non-zero with a validation error naming the failure
- AND no file, package, or service on the host is modified

#### Scenario: Unknown module name is rejected

- GIVEN a host profile that enables a module name absent from the module registry
- WHEN the catalog is validated
- THEN validation fails with an error naming the unknown module

### Requirement: Global-to-role-to-host merge precedence

The system MUST merge catalog layers so that host values override role values, which
override global values, with deterministic deep-merge semantics per field. The role
layer MUST be permitted to be empty (reserved for future use), and the initial
catalog MUST contain exactly one concrete host profile, `galaxy`, with shared
defaults held in the global layer.

#### Scenario: Host layer overrides global default

- GIVEN a global layer setting a value and the `galaxy` host layer setting a different value for the same field
- WHEN the catalog is merged for host `galaxy`
- THEN the host value is used

#### Scenario: Empty role layer is accepted

- GIVEN a catalog whose role layer is empty
- WHEN the catalog is merged for any host
- THEN merging succeeds and deterministically produces the global-plus-host result

### Requirement: Host resolution without sensitive identifiers

The system MUST resolve the active host profile from the machine hostname, with an
explicit `--host` argument taking precedence. The system MUST NOT use DMI data,
`/etc/machine-info`, or other hardware or machine identifiers as catalog keys. When
no host profile matches and no `--host` is provided, the system MUST fail without
mutation with an actionable error listing the known host profiles.

#### Scenario: Explicit host override wins

- GIVEN a machine whose hostname does not match any host profile
- WHEN a command runs with `--host galaxy`
- THEN the `galaxy` host profile is selected

#### Scenario: Unmatched hostname fails closed

- GIVEN a machine whose hostname matches no host profile and no `--host` argument is provided
- WHEN a mutating command runs
- THEN the command exits non-zero with an error listing known host profiles
- AND no mutation occurs

### Requirement: Exact-version pinning only

Desired-state package entries in the catalog MUST pin exact versions; range
specifiers such as `^`, `~`, or `>=` MUST be rejected by catalog validation. The
fingerprint PKGBUILD pinning (upstream commit and patch sha256) MUST be preserved in
the catalog's packaging assets.

#### Scenario: Range specifier is rejected

- GIVEN a catalog package entry using a range specifier instead of an exact version
- WHEN the catalog is validated
- THEN validation fails with an error identifying the offending entry

#### Scenario: Exact pin plans reproducibly

- GIVEN a catalog pinning a Pi package at an exact version
- WHEN the catalog is loaded and planned on two machines
- THEN both plans reference the same exact version

### Requirement: Desired pins versus resolved versions

Catalog exact pins define the desired state. Runtime-resolved versions observed via
package listings or installed package state MUST NOT redefine the desired state and
MUST be recorded only as separately named resolved-version fields (see
`receipts`). `check` MUST report drift between desired exact pins and resolved
installed versions.

#### Scenario: Resolved drift is reported without redefining desired state

- GIVEN a catalog exact pin of a package at version X and an installed resolved version Y
- WHEN `check` runs
- THEN drift between X and Y is reported
- AND the catalog desired pin remains X

#### Scenario: Receipt separates the two fields

- GIVEN a completed run
- WHEN its receipt is inspected
- THEN desired exact pins and resolved installed versions appear as distinct, separately named fields

### Requirement: Embedded assets and typed template rendering

The Go binary MUST embed templates, overlays, and packaging assets via `go:embed`
and MUST NOT require repository-relative asset paths at runtime. Template rendering
MUST use typed `@HOME@` and `@USER@` token substitution, and validation MUST be
performed before any file write.

#### Scenario: Binary renders assets outside the repository

- GIVEN the compiled Go binary executed from a directory outside the repository checkout
- WHEN a template step is planned and executed
- THEN rendering succeeds from embedded assets without repository-relative reads

#### Scenario: Unknown token fails validation before write

- GIVEN a template containing a token outside the supported substitution set
- WHEN the template is rendered
- THEN rendering fails with a validation error
- AND no target file is written

### Requirement: Annotated catalog checkpoint tags

Accepted catalog versions MUST be captured as annotated `catalog-v*` Git tags that
carry tagger metadata and a message and are visible to `git describe`. Moving an
existing checkpoint tag MUST require explicit force semantics (a `+` refspec or
`--force`). Receipts MUST record the catalog tag applied by each run. The
`checkpoint` command surface is defined in `convergence-commands`.

#### Scenario: Checkpoint tag is annotated and describe-visible

- GIVEN a catalog snapshot accepted as version `1.0.0`
- WHEN the checkpoint is created
- THEN an annotated tag `catalog-v1.0.0` exists and is reported by `git describe`

#### Scenario: Tag move refuses without force

- GIVEN an existing annotated tag `catalog-v1.0.0`
- WHEN an attempt is made to move it without a `+` refspec or `--force`
- THEN the move is rejected
