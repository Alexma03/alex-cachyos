# Design: Add Portable Host Profile

## Technical Approach

Extend the existing catalog deep module, not the generic planner. A repository resolves `global -> roles in declared order -> host` into one immutable `ResolvedHostPolicy`. CachyOS factories combine that policy with typed read-only evidence and construct only authorized steps.

```text
--host/hostname -> ResolveHost -> Repository.Resolve -> ResolvedHostPolicy
                                                        |
                                   ObservationRequestFor(policy)
                                                        |
                                      BuildModules(policy, evidence) -> planner
```

Unknown hosts, missing roles, invalid policy, or live-target mismatch fail before observation, locking, or mutation.

## Architecture Decisions

| Choice | Rejected | Rationale |
|---|---|---|
| Host-only ordered `roles` and closed `riskPolicy` in catalog v1 | Module booleans; autodetection | Catalogs are unpublished; typed omission preserves default-deny and evidence cannot become authority. |
| Gate inside CachyOS factories | Policy fields on `planner.Step`; plugins | Risk stays beside command construction; one platform does not justify a hierarchy. |
| Physical role/host asset families | Conditional or reused Galaxy templates | Path ownership makes leakage mechanically testable. |
| Separate `--integration-target HOST` | `--host` as authorization | Desired-state selection and permission for live hardware work are distinct. |

## Interfaces / Contracts

Schema and Go document types gain host-only `roles: []string` and `riskPolicy`. The closed booleans are `fingerprintPam`, `fixedDisplays`, `fixedInputDevices`, `literalHomePaths`, `bootstrapSystemUpdate`, `bootstrapPackageRemoval`, `bootstrapBootMutation`, and `cosmicPrune`. Omission is false; Global/Role policy fields, duplicate/unknown roles, and unknown capabilities fail validation. A portable host may opt into fixed values only through its own host assets; the synthetic fixture opts into none.

```go
type EvidenceKind string // fixture | live
type ResolvedHostPolicy struct { Name string; Roles []string; Desired Catalog; Risks RiskPolicy }
type VerificationEvidence struct { Kind EvidenceKind; Host, IntegrationTarget string; Checks []CheckEvidence }
func (r *Repository) Resolve(name string) (ResolvedHostPolicy, error)
func (p ResolvedHostPolicy) Allows(RiskCapability) bool
```

`ObservationRequestFor` requests evidence only for allowed capabilities. Evidence is `ready|blocked|unknown`, never enabled. Disabled means no request/step; allowed plus non-ready becomes `planner.DispositionBlocked`.

`internal/cli/command.go` parses/documents `--integration-target`; the gate accepts it only when non-empty and equal to the resolved host. Command output emits canonical `VerificationEvidence`. Fixture output persists as deterministic acceptance goldens. Parent WU-20 records live output/hash plus the existing immutable receipt ID in `docs/cutover-evidence.md`; no new receipt schema is introduced.

## File Changes

| Path | Action |
|---|---|
| `catalog/schema/catalog-v1.schema.json`, `internal/catalog/{types,decode,merge,validate,digest}.go` | Add policy/resolution fields |
| `internal/catalog/repository.go` | Add host inventory and resolver |
| `internal/platform/cachyos/{policy,bootstrap,fingerprint,desktop}.go` | Add local policy/evidence gates |
| `internal/cli/command.go`, `internal/app/integration_target.go` | Parse and enforce live authorization |
| `templates/niri/config.kdl` -> `templates/hosts/galaxy/niri/config.kdl` | Move Galaxy values |
| `templates/noctalia/settings.toml` -> `templates/hosts/galaxy/noctalia/settings.toml` | Move Galaxy values |
| `templates/hyprwhspr/config.json` -> `templates/hosts/galaxy/hyprwhspr/config.json` | Move Galaxy values |
| `templates/roles/workstation/{niri,noctalia,hyprwhspr}/**` | Add hardware-independent bases |
| `templates/embed.go`, `tools/sync-assets/main.go` | Embed/declare both families; remove old declarations |
| `overlays/galaxy/**` | Keep unchanged and Galaxy-only |
| `testdata/{catalog,planner,acceptance}/portable-synthetic/**` | Add non-production fixture |
| `docs/cutover-evidence.md` | Parent integration records live output/hash and receipt ID |

No production portable host is invented.

## Testing Strategy

Table-driven unit tests cover schema, ordered precedence, exact pins, omission=false, stable digest, and capability x evidence. For **each** Galaxy and synthetic-portable fixture, goldens assert resolved global/role/host trace and host overrides, exact desired pins, dependency ordering, offline `check`, dry-run network grouping with zero commands/writes, first fake convergence, and second apply with zero mutations plus `noChange:true`.

Live-gate tests prove absent authorization emits `kind:fixture`; a mismatched target fails; both invoke no hardware port and write no live evidence. Matching authorization emits `kind:live`; acceptance verifies cutover-report output/hash and receipt-ID linkage.

## Threat Matrix

| Boundary | Adversarial cases | Applicability | Response/tests |
|---|---|---|---|
| Documentation-like paths | Executable-looking docs | N/A: no executable classification | None |
| Git repository selection | Relative/absolute `git -C` | N/A: no VCS/cwd change | None |
| Commit state | Staged, `commit -a`, empty index | N/A: no commit automation | None |
| Push state | Tracking, first push, refspec | N/A: no push automation | None |
| PR commands | Head, environment, composition | N/A: no PR automation | None |

## Migration / Rollout

Implement resolver, gates/assets, fixtures, then live evidence. Amendment verification writes the standard `verify-report.md` with a `Parent Integration Contract` table for WU-5/6/14/20. Each parent WU later receives an unchecked prerequisite naming that report; it is checked only when native status returns `nextRecommended=archive` and the relevant contract row passes. Thus the parent's task ledger blocks closure without editing it in this phase.

Rollback reverts this amendment before parent integration; Bash and Galaxy remain usable and no persisted catalog migration exists.

## Open Questions

None.
