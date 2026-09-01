# Tasks: Add Portable Host Profile

No task invents production host values or creates branches, commits, pushes, or PRs.

## Review Workload Forecast

| Field | Value |
|---|---|
| Complexity and cohesion | Three ordered behaviors |
| Domain/interface boundaries | Catalog → platform → acceptance |
| Verification and risk burden | Default-deny, asset isolation, goldens, parent amendment |
| Chained PRs recommended | Yes |
| Suggested split | WU-1 → WU-2 → WU-3 |
| Delivery strategy | natural chained split selected by the user |
| Chain strategy | stacked-to-main if selected |

Decision needed before apply: No (resolved: WU-1 → WU-2 → WU-3 natural split)
Chained PRs recommended: Yes
Chain strategy: stacked-to-main

### Suggested Work Units

| Unit | Goal | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|
| WU-1 | Resolve host policy | `go test ./internal/catalog -count=1` | N/A: pure catalog | Schema, `internal/catalog/**` |
| WU-2 | Gate platform/assets | `go test ./internal/platform/cachyos -count=1` | Fake portable plan | Gates and role/host assets |
| WU-3 | Separate evidence | `go test ./internal/cli ./internal/app ./internal/acceptance -count=1` | Synthetic acceptance fixture | CLI/app gate and fixtures |

## WU-1 — Catalog authority (hardware-free first slice)

- [x] 1.1 RED table tests: multiple hosts, ordered/unknown/duplicate roles, closed host-only risks, omission=false, stable digest, and unknown-host failure before ports.
- [x] 1.2 GREEN update `catalog/schema/catalog-v1.schema.json`, catalog decode/merge/validate/digest/types, and add `internal/catalog/repository.go` returning one resolved policy.
- [x] 1.3 TRIANGULATE host override, empty role, observation-cannot-enable, and no-synthesized-production-values cases.
- [x] 1.4 REFACTOR canonical risks and immutable cloning without planner policy or plugins.
- [x] 1.5 Record cycle evidence, focused result, runtime `N/A`, rollback paths, and bounded diff inventory.

## WU-2 — Platform policy and asset isolation

- [x] 2.1 RED table tests for each risk: omitted, allowed+ready, allowed+blocked, allowed+unknown; disabled risks request nothing and create no step.
- [x] 2.2 RED asset tests reject Galaxy references from the synthetic host and preserve Galaxy resolution.
- [x] 2.3 GREEN add factory-local gates, workstation bases, `templates/hosts/galaxy/**`, and updated embedding declarations.
- [x] 2.4 TRIANGULATE fake plans preserve Galaxy behavior while portable bootstrap/desktop/fingerprint omit risky steps despite observations.
- [x] 2.5 REFACTOR authorization beside command construction; record cycle, focused/runtime/rollback, and diff evidence.

## WU-3 — Authorization, acceptance, and parent handoff

- [x] 3.1 RED CLI/app tests: absent target emits fixture evidence; mismatch fails before hardware ports; matching target permits live checks.
- [x] 3.2 GREEN enforce `--integration-target` and canonical fixture/live evidence without receipt-schema changes.
- [x] 3.3 TRIANGULATE Galaxy/portable goldens: merge trace, pins, order, offline check/dry-run, convergence, second `noChange:true`, and zero leakage.
- [x] 3.4 REFACTOR evidence rendering; record cycle, focused/runtime/rollback, and diff evidence.
- [x] 3.5 Map parent WU-5=catalog, WU-6=fixtures, WU-14=gates/assets, WU-20=traceability/live evidence; do not edit parent artifacts.
- [x] 3.6 Run `go test ./... && go vet ./... && go run ./tools/sync-assets --check` plus transition guard; record results and hardware-free execution.

## Parent Integration Contract

Parent WU-5/6/14/20 MUST remain incomplete until this change verifies and native status reaches `nextRecommended=archive`; the verify report MUST mark each mapped row pass/fail before parent reconciliation.
