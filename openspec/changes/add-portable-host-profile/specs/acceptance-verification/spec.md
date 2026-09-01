# Delta for Acceptance Verification

## ADDED Requirements

### Requirement: Amendment integration order

`add-portable-host-profile` MUST amend `rebuild-reproducible-configurator` before parent WU-5, WU-6, WU-14, or WU-20 is completed. Its standard `verify-report.md` MUST carry a passing `Parent Integration Contract` row for each work unit, and each affected parent work unit MUST retain an unchecked prerequisite naming that row until native status recommends archive. Parent catalog, platform, fixture, and acceptance artifacts MUST then reflect these requirements.

#### Scenario: Parent work cannot close first

- GIVEN this amendment is not integrated
- WHEN an affected parent work unit is evaluated for completion
- THEN its prerequisite remains unchecked and completion is blocked with the amendment dependency and contract row named

## MODIFIED Requirements

### Requirement: Fixture-first convergence proof

The fake-runner suite MUST prove deterministic precedence, pins, ordering, check/dry-run, and no-change re-apply for both Galaxy and a clearly non-production synthetic portable host. Synthetic values MUST NOT become production host data.

(Previously: The full fixture asserted only the `galaxy` plan.)

#### Scenario: Full fixture suite passes

- GIVEN Galaxy and synthetic portable fixtures
- WHEN `go test ./...` runs
- THEN both assert their plans and the portable fixture contains no Galaxy assets or risky steps

### Requirement: Hardware verification gated on an explicit target

Real niri/Noctalia validators, Pacman integrity checks, package installation, and other hardware mutations MUST run only with separate explicit integration-target authorization for the selected host. Status output MUST identify evidence as `fixture` or `live`; live evidence MUST be recorded in the cutover report with its immutable receipt ID. Fixture success or host-profile selection alone MUST NOT authorize hardware execution or be reported as live convergence.

(Previously: Hardware verification named `galaxy` or another declared target without separating profile selection from authorization.)

#### Scenario: Fixture success is not hardware convergence

- GIVEN either fixture passes without explicit live-target authorization
- WHEN status is reported
- THEN it reports fixture evidence only and performs no hardware verification
