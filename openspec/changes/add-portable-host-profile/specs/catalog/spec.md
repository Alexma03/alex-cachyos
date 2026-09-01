# Delta for Catalog

## ADDED Requirements

### Requirement: Default-deny risky capability policy

Each risky capability MUST be an explicit host-owned boolean opt-in. Omission MUST equal `false`. Runtime observations MAY block an opted-in capability but MUST NOT enable one. Portable defaults MUST exclude Galaxy overlays, fingerprint/PAM mutation, fixed display/device/home values, destructive bootstrap actions, and Cosmic pruning.

#### Scenario: Missing opt-in remains disabled

- GIVEN a host omits a risky capability
- WHEN its catalog is resolved and planned
- THEN that capability is disabled and produces no mutation

#### Scenario: Observation is deny-only

- GIVEN a risky capability is disabled but matching hardware is observed
- WHEN planning runs
- THEN the observation does not enable the capability

## MODIFIED Requirements

### Requirement: Global-to-role-to-host merge precedence

The system MUST deterministically merge `global → named roles in declared order → host`. Multiple explicit hosts MUST be allowed. The `workstation` role MUST contain hardware-independent defaults, while host identity and risky opt-ins remain host-owned. Empty roles MUST remain valid. Production host values MUST NOT be invented.

(Previously: The initial catalog allowed exactly one concrete `galaxy` host and an empty reserved role.)

#### Scenario: Host layer overrides inherited defaults

- GIVEN a host names `workstation` and overrides an inherited value
- WHEN that host is merged
- THEN its value wins over role and global values

#### Scenario: Empty role layer is accepted

- GIVEN a declared role has no values
- WHEN a host names that role
- THEN merging succeeds deterministically

### Requirement: Host resolution without sensitive identifiers

The system MUST select only an explicit catalog host: `--host` takes precedence, otherwise hostname is used. DMI, machine information, and hardware identifiers MUST NOT select a host. Unknown hosts MUST fail before planning, locking, or mutation and list known hosts.

(Previously: Resolution was defined against the sole `galaxy` profile.)

#### Scenario: Explicit host override wins

- GIVEN a hostname that differs from a known portable host
- WHEN a command supplies that host with `--host`
- THEN the named profile is selected

#### Scenario: Unmatched hostname fails closed

- GIVEN no explicit host and no hostname match
- WHEN any command resolves desired state
- THEN it fails before mutation and lists known hosts
