# Proposal: Add Portable Host Profile

## Intent

`rebuild-reproducible-configurator` assumes `galaxy` is the only concrete host, while its assets and risky system actions are Galaxy-specific. This amendment adds a fail-closed portable host model so another CachyOS+niri workstation can be represented without reusing unsafe Galaxy behavior or inventing target hardware.

## Scope

### In Scope
- Hardware-independent defaults in the `workstation` role and multiple explicit host profiles.
- Typed host-owned opt-ins for fingerprint/PAM mutation, fixed displays/devices/home paths, destructive bootstrap actions, and Cosmic pruning; absent opt-ins are `false`. A portable host may enable fixed-value capabilities only with its own explicit, non-invented host values; the synthetic fixture enables none.
- A synthetic non-production portable fixture and a separate explicit live integration-target gate.
- Coordinated integration before parent WU-5, WU-6, WU-14, and WU-20 close.

### Out of Scope
- Pi/Gentle AI reproduction, other distributions/desktops, live apply, Go cutover, or invented production host values.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `catalog`: allow multiple hosts, role membership, and typed risky-capability policy.
- `cachyos-platform`: require host opt-in before risky steps; runtime observations may block but never enable them.
- `acceptance-verification`: add portable fixture evidence while keeping live convergence separately authorized.

## Approach

Keep host selection hostname/`--host` based and fail-closed. `internal/catalog` resolves one validated host policy; `internal/platform/cachyos` combines it with read-only observations to produce satisfied, apply, or blocked steps. Portable defaults exclude all Galaxy overlays and risky capabilities. No plugin hierarchy is introduced.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `catalog/**`, `internal/catalog/**` | Modified | Multi-host layering and risk policy |
| `internal/platform/cachyos/**`, `internal/app/**`, `internal/cli/**` | Modified | Policy-gated construction and explicit live-target authorization |
| `templates/**`, `tools/sync-assets/**` | Modified | Physical workstation/Galaxy asset separation |
| `testdata/**`, acceptance tests | Modified | Synthetic fixture and live-target separation |
| Parent change artifacts | Amended later | Reconcile WU-5/6/14/20 after this change is specified |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Parent requirements conflict | Medium | Integrate this amendment before affected parent work units close |
| Galaxy behavior leaks into portable defaults | Medium | Default-deny validation and fixture assertions |
| Fixture mistaken for live convergence | Low | Separate evidence states and explicit target gate |

## Rollback Plan

Revert this amendment's catalog policy, portable fixture, and delta specs before parent integration; preserve Galaxy behavior and the Bash path unchanged.

## Dependencies

- **Depends on and amends:** `rebuild-reproducible-configurator` host, catalog, platform, fixture, and acceptance assumptions.

## Success Criteria

- [ ] A synthetic portable host resolves deterministically without Galaxy assets or risky steps.
- [ ] Missing opt-ins never enable risky behavior; observations only deny or block.
- [ ] Portable hosts may use fixed values only through explicit opt-ins and their own non-invented host assets.
- [ ] Unknown hosts fail before mutation, and fixture success is never reported as live convergence.
