# Review Mode (RDD) Specification

## Purpose

Receipt-driven development is globally enabled as this user's explicit desired
state, while the package default is opt-in off. Reproduction MUST explicitly enable
the global switch and verify the effective state, and MUST treat
`disabled/unmanaged` reports as truthful non-authorization.

## Requirements

### Requirement: Explicit global enable

The reproduction flow MUST run `gentle-ai review mode enable --scope global` — the
only command that turns RDD on — rather than relying on the package's opt-in
default. The configurator MUST NOT silently leave the switch off.

#### Scenario: Enable runs explicitly during reproduction

- GIVEN a fresh machine after the harness install
- WHEN the reproduction flow reaches the RDD step
- THEN it invokes the global-scope enable command and records the outcome

### Requirement: Verified effective state

After enabling, the flow MUST verify with `gentle-ai review mode status --cwd
<repo>`, which is non-mutating, and MUST assert that the effective mode is on with
the deciding source `global`. `check` MUST assert the same effective state on
subsequent runs.

#### Scenario: Status verifies the enabled state

- GIVEN a machine where the global enable command has run
- WHEN `gentle-ai review mode status --cwd <repo>` is invoked by the flow
- THEN it reports `effective: on` with `deciding: global`

#### Scenario: Check asserts the effective state

- GIVEN a converged host
- WHEN `check` runs
- THEN it asserts the review mode status reports effective on and deciding global, and reports drift if not

### Requirement: Truthful non-authorization while disabled

While RDD is disabled, the configurator MUST NOT fabricate review approval and MUST
report `disabled/unmanaged` truthfully; ordinary repository policy decides
delivery. The configurator MUST NOT start, retry, or re-enable review on its own
when the switch is disabled.

#### Scenario: Disabled state is reported truthfully

- GIVEN a machine where the review-mode switch is disabled
- WHEN review-mode status is consulted
- THEN the state is reported as disabled/unmanaged without any fabricated approval
