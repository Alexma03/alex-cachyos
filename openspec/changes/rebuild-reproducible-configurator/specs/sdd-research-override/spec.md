# SDD Research Override Retirement Specification

## Purpose

The temporary Pi web-enabled `sdd-research` override exists specifically because
Pi's packaged agent lacks the exact `pi-web-access` web tools needed for
source-backed research. Retirement is governed by a Pi-specific predicate — the
closure of `gentle-ai#3846` plus `gentle-pi#471` with a packaged agent exposing the
exact web tools and non-empty grants, verified by manifest and runtime smoke checks
— never by a generic Gentle AI stable release alone.

## Requirements

### Requirement: Override installation

The catalog MUST install `~/.pi/agent/subagents/sdd-research.md` with front-matter
`name: sdd-research`, model `commandcode/deepseek/deepseek-v4-flash`, `thinking:
max`, and the tool allowlist `read, grep, find, edit, write, web_search,
source_check, fetch_content, get_search_content, mem_search, mem_get_observation,
mem_save`, with the body carrying the SDD research executor contract
(`gentle-ai.sdd-research/v1` artifact schema, `gentle-ai.sdd-preproposal/v1`
schema, and the admission grants `documentation=[fetch_content]` and
`open-web=[web_search, source_check, fetch_content, get_search_content]`).

#### Scenario: Override file matches the contract

- GIVEN a converged host
- WHEN `~/.pi/agent/subagents/sdd-research.md` is inspected
- THEN its front-matter carries the name, model, thinking level, and exact allowlist, and its body carries the two schema versions and admission grants

### Requirement: Pi-specific retirement predicate

The catalog MUST encode `retire_when: "pi-sdd-research-web-capable"` as a predicate
requiring ALL of: (1) `Gentleman-Programming/gentle-ai#3846` and
`Gentleman-Programming/gentle-pi#471` are closed or completed, and the consumed
`gentle-pi` `main` checkout contains the packaged `sdd-research` agent; (2) the
packaged agent shipped by the consumed local-main `gentle-pi` checkout lists the
exact web tools (`web_search`, `source_check`, `fetch_content`, `get_search_content`
or their Pi equivalents) and declares non-empty grants for `documentation` and
`open-web` matching the override; and (3) post-removal manifest and runtime smoke
verification passes. A generic Gentle AI stable release (including `v2.5.0`) that
ships `sdd-research` without the Pi web grants MUST NOT satisfy the predicate.

#### Scenario: Generic stable alone does not retire the override

- GIVEN a consumed generic Gentle AI stable release that ships a generic `sdd-research` phase without the Pi web grants
- WHEN the retirement predicate is evaluated
- THEN the predicate is false and the override remains installed

#### Scenario: Predicate is true only on full Pi capability

- GIVEN both upstream issues closed, a consumed local-main `gentle-pi` checkout containing the packaged agent with the exact web tools and non-empty grants, and passing manifest and smoke verification
- WHEN the retirement predicate is evaluated
- THEN the predicate is true

### Requirement: Retirement evaluation and removal

`apply --update` MUST evaluate the retirement predicate. On a true predicate, the
system MUST delete `~/.pi/agent/subagents/sdd-research.md`, run `gentle-ai sync`,
verify `managed-assets.json` integrity, and smoke-probe that the packaged
`sdd-research` agent is discoverable without the override file. `check` MUST report
override staleness against the predicate.

#### Scenario: Retirement removes the override and verifies the packaged agent

- GIVEN a true retirement predicate
- WHEN `apply --update` runs
- THEN the override file is deleted, `gentle-ai sync` succeeds, the manifest verifies, and the packaged agent is discoverable by smoke probe

#### Scenario: Check reports override staleness

- GIVEN a host where the retirement predicate is true but the override is still installed
- WHEN `check` runs
- THEN the override is reported as stale against the predicate
