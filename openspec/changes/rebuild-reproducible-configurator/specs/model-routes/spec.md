# Model Routes Specification

## Purpose

The 23 model routes are duplicated across two files with different key spellings
(`subagents.json` uses `model`+`effort`; `gentle-ai/models.json` uses
`model`+`thinking`). The catalog models the routes once and renders both files from
that single source, with drift management so the two cannot silently diverge. This
capability also fixes the non-default lean/task behavior explicitly.

## Requirements

### Requirement: Single-source 23 model routes with dual rendering

The catalog MUST define exactly 23 model routes once, each as `{name, model,
level}`, covering the route names `gentle-ai-{explore,worker,verify}`,
`sdd-{init,onboard,explore,research,proposal,spec,design,tasks,apply,verify,status,sync,archive}`,
`review-{risk,reliability,resilience,readability}`, and
`jd-{judge-a,judge-b,fix-agent}`. The system MUST render
`~/.pi/agent/subagents.json` (with `effort` keys) and
`~/.pi/gentle-ai/models.json` (with `thinking` keys) from that single source.

#### Scenario: Both files render from one source

- GIVEN the catalog's 23 model routes
- WHEN the Pi configuration steps run
- THEN `subagents.json` contains the 23 profiles with `model` and `effort`, and `models.json` contains the same 23 names with `model` and `thinking`, with matching values

#### Scenario: Route count and names are exact

- GIVEN the rendered output of both files
- WHEN the route names are extracted
- THEN both files contain exactly the 23 expected names and nothing else

### Requirement: Cross-file drift detection

`check` MUST byte-compare both rendered files against the catalog-rendered
expectations and MUST fail or report drift when either file diverges from the
single source, so the two files cannot drift independently.

#### Scenario: Drift between the two files is detected

- GIVEN a host where `models.json` has been hand-edited to a different level for one route
- WHEN `check` runs
- THEN the drifted route is reported with both the file and the expected value

### Requirement: Explicit lean and task behavior

The desired state MUST explicitly set `session_resources: lean`, `default_mode:
task`, `enable_continue: false`, and `debug: false` in
`~/.pi/agent/subagents.json`, rather than relying on Pi defaults. `check` MUST
assert these values.

#### Scenario: Lean task values are explicit and verified

- GIVEN a converged host
- WHEN `check` runs
- THEN `subagents.json` reports `session_resources: lean`, `default_mode: task`, `enable_continue: false`, and `debug: false`, each asserted against the catalog
