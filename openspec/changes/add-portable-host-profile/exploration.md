# Exploration: Portable non-Galaxy host profile

### Current State

`rebuild-reproducible-configurator` is the active parent change and currently assumes that `galaxy` is the sole concrete host. Its catalog design intends `global → role → host` layering, but the implemented catalog model currently exposes only document kind, module booleans, template/overlay references, source pins, and checkout pins. It does not yet represent host role membership, release metadata, or an explicit policy for risky hardware/system behavior.

Host resolution itself is already fail-closed: explicit `--host` wins, hostname is the only automatic selector, and unknown hosts fail without DMI or machine identifiers. The planner already supports explicit module selection, but it receives already-built modules and has no host-risk policy boundary. The Go entrypoint currently handles only help and module listing, so live apply/check remains owned by the parent change.

The only production profile and overlay are Galaxy-specific. Existing assets contain fixed `DP-3`/`DP-1`/`eDP-1` displays, `/home/alex` paths, named input devices, Galaxy-specific EgisTec fingerprint/PAM behavior, destructive bootstrap update/removal and GRUB/Plymouth actions, and Cosmic-specific desktop mutation. Applying those defaults to another PC would be unsafe.

This change is therefore an amendment to the active parent change, not an independent alternate architecture. It must revise the parent's “exactly one Galaxy host” catalog assumption and its Galaxy-only fixture/acceptance assumptions before the parent reaches WU-5, WU-6, WU-14, or WU-20 completion.

### Affected Areas

- `openspec/changes/rebuild-reproducible-configurator/proposal.md` — currently scopes the initial catalog to Galaxy alone; the new proposal must explicitly depend on and amend this assumption.
- `openspec/changes/rebuild-reproducible-configurator/specs/catalog/spec.md` — requires exactly one concrete `galaxy` host and needs a coordinated delta.
- `openspec/changes/rebuild-reproducible-configurator/specs/cachyos-platform/spec.md` — currently maps Galaxy-sensitive module behavior one-to-one without a portable-default policy.
- `openspec/changes/rebuild-reproducible-configurator/specs/acceptance-verification/spec.md` — fixture and hardware evidence are Galaxy-centered; portable fixture evidence must remain distinct from live target convergence.
- `openspec/changes/rebuild-reproducible-configurator/tasks.md` — WU-5, WU-6, WU-14, and WU-20 are the integration points and must not be silently contradicted.
- `catalog/schema/catalog-v1.schema.json` and `internal/catalog/{types,decode,merge,validate,digest}.go` — catalog boundary for multiple hosts, role membership, and explicit risky-capability policy.
- `internal/app/host.go` — stable fail-closed host-selection interface; likely extended by catalog loading rather than hardware discovery.
- `internal/planner/plan.go` — must preserve exact selection while receiving only policy-approved module definitions and blocked preconditions.
- `internal/platform/cachyos/**` — consumes explicit host policy plus observations; risky behavior must require opt-in and fail closed.
- `templates/{niri/config.kdl,noctalia/settings.toml,hyprwhspr/config.json}` and `overlays/galaxy/**` — Galaxy assets must be separated from portable defaults rather than conditionally reused.
- `testdata/**` and future acceptance tests — need a synthetic non-production portable fixture and a separately declared live integration target.

### Approaches

1. **Module booleans only** — Add another host document, disable `bootstrap` and `fingerprint`, and reuse the remaining module/assets.
   - Pros: Smallest schema change; reuses current planner selection.
   - Cons: Unsafe because `desktop` still contains fixed devices/paths and Cosmic pruning, while risky substeps inside a module cannot be independently denied.
   - Effort: Low, but insufficient.

2. **Portable role plus explicit host risk policy** — Put hardware-independent defaults in the workstation role, keep host identity/profile data separate, and require explicit per-host opt-ins for fingerprint/PAM, fixed display/input configuration, destructive bootstrap behavior, and Cosmic pruning. Runtime observations may block an opted-in action but never enable it.
   - Pros: Fail-closed; preserves `global → role → host`; keeps catalog parsing/merge as a deep module and platform command construction local; supports later real host data without invention.
   - Cons: Requires coordinated schema/types/merge/validation, platform factory, fixture, and parent-change spec/task updates.
   - Effort: Medium.

3. **Runtime hardware autodetection as authority** — Detect devices/platform state and automatically enable matching behavior.
   - Pros: Less profile authoring.
   - Cons: Violates the confirmed opt-in decision, couples identity to hardware heuristics, and risks destructive false positives. Runtime observation is useful only as a deny/block condition.
   - Effort: Medium and unacceptable risk.

### Recommendation

Use approach 2. Define **portable defaults** as hardware-independent desired state inherited from the workstation role; define a **host profile** as the explicit catalog document selected by `--host` or hostname; define a **risky capability** as a host-owned opt-in whose absence is false; and define an **integration target** as separate runtime authorization for live verification. Avoid “generic host” and “autodetected profile,” because neither is safe authority.

Keep the interface narrow: `internal/catalog` should return one validated resolved host policy; `internal/platform/cachyos` should turn that policy plus read-only observations into satisfied/apply/blocked steps. Do not add a speculative plugin/adapter hierarchy. Multiple platform implementations do not exist, and the real variation is host policy within CachyOS+niri.

Portable defaults must exclude Galaxy overlays, fingerprint/PAM, fixed outputs and device names, literal home paths, destructive bootstrap update/removal, and Cosmic pruning. The real target hostname, output names, device identifiers, and enabled risky capabilities remain unresolved profile data supplied later; no production host file should invent them. A synthetic fixture may use clearly non-production values.

The proposal should declare an explicit dependency on `rebuild-reproducible-configurator`, amend its catalog/platform/acceptance assumptions, and define integration order before the parent marks WU-5/WU-6/WU-14/WU-20 complete. Pi/Gentle AI work, multi-distro/DE support, live system mutation, and Go cutover are non-goals.

### Risks

- Two active changes can produce contradictory catalog requirements unless dependency and integration order are explicit.
- A host-level module boolean alone cannot safely gate destructive substeps inside `desktop` or `bootstrap`.
- Reusing Galaxy assets under a different host name would create false portability.
- Fixture success must never be reported as live hardware convergence.
- The target profile cannot be production-ready until the user later supplies its real identity and hardware values.

### Ready for Proposal

Yes. Product decisions are confirmed, selected research is unselected because the evidence and constraints are repository-local, and no unresolved product choice blocks proposal drafting. The proposal must carry this confirmed pre-proposal handoff:

```yaml
schema: gentle-ai.sdd-preproposal/v1
revision: 1
change_name: add-portable-host-profile
artifact_store: openspec
exploration_ref: openspec/changes/add-portable-host-profile/exploration.md
research:
  selected: false
  status: unselected
product_decisions:
  status: confirmed
  portable_defaults: hardware-independent and fail-closed
  risky_behavior: explicit host opt-in plus runtime observation as deny-only evidence
  excluded_by_default:
    - Galaxy overlays
    - fingerprint and PAM mutation
    - fixed display, device, and home-path values
    - destructive bootstrap update and removal
    - Cosmic pruning
  target_profile_data: supplied later; never invented
dependency:
  change: rebuild-reproducible-configurator
  relationship: explicit amendment of host, catalog, platform, fixture, and acceptance assumptions
proposal_scope:
  include:
    - portable workstation role defaults
    - multiple-host catalog support
    - typed risky-capability policy
    - synthetic portable fixture and explicit live-target gate
    - parent-change integration order
  exclude:
    - Pi and Gentle AI reproduction
    - multi-distro or alternative desktop support
    - live apply or cutover
proposal_ready: true
```
