# Apply Progress — add-portable-host-profile

## Cumulative status

- Completed: WU-1 tasks 1.1–1.5, WU-2 tasks 2.1–2.5, and WU-3 tasks 3.1–3.6 (16/16).
- Pending: verification, native attempt settlement by the parent, and archive routing.
- Delivery boundary: natural WU-1 → WU-2 → WU-3 chain; stacked-to-main only if PRs are later created.
- Production host values: deferred; the implementation provides catalog authority and synthetic names only.

## WU-1 — Catalog authority

### RED → GREEN → TRIANGULATE → REFACTOR

| Stage | Evidence |
|---|---|
| RED | `go test ./internal/catalog -count=1` failed with undefined `NewRepository`, risk capability, and policy symbols before production changes. |
| GREEN | The focused command passed after schema, decode, merge, validate, digest, types, and repository implementation. |
| TRIANGULATE | Table tests cover multiple explicit hosts, declared role order, host override, empty role lists, unknown/duplicate roles, every closed risk, omission=false, deny-only observations, stable digest, unknown-host pre-port failure, and no synthesized production path/value. |
| REFACTOR | Canonical capability order and `Allows` remain in catalog; repository inputs/results deep-clone mutable values. No planner policy, plugin hierarchy, hardware discovery, or production portable catalog was added. |

### Verification

- Focused: `go test ./internal/catalog -count=1` — PASS.
- Static analysis: `go vet ./internal/catalog` and `go vet ./...` — PASS.
- Exact repository chain: `go test ./... && bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]' && go run ./tools/sync-assets --check` — PASS.
- Initial full-chain attempt: FAILED because the copied schema changed before `data/source-manifest.json` was regenerated; failure was settled at evidence revision `sha256:82c1cc6c11203ce6285e11f566952baa1df31ec91166e47a2f936039a0ca3154`. `go run ./tools/sync-assets` regenerated the manifest, and the distinct final chain passed.
- Runtime harness: N/A — WU-1 is pure in-memory catalog resolution. Tests prove an unknown host returns before the injected asset port; no hardware, observation, lock, or mutation port ran.

### Rollback and bounded diff inventory

- Rollback: revert `catalog/schema/catalog-v1.schema.json`, its generated `internal/assets/data/**` copies/manifest, and `internal/catalog/{types,decode,merge,validate,digest,repository}.go` with their tests.
- Product diff: eight tracked files modified and `internal/catalog/repository.go` plus `internal/catalog/repository_test.go` added; no planner, platform, CLI, profile, template, overlay, or production-host asset changed.
- Artifact diff: this cumulative progress file was added and tasks 1.1–1.5 were checked. Proposal, specs, design, and parent-change artifacts were not edited.
- Repository actions: no staging, commit, push, PR, or branch mutation.

## WU-2 — Platform policy and asset isolation

### RED → GREEN → TRIANGULATE → REFACTOR

| Stage | Evidence |
|---|---|
| RED | `go test ./internal/platform/cachyos ./internal/assets -count=1` initially failed on undefined policy APIs and absent host/role asset paths. The focused correction regression `go test ./internal/runner -run TestValidateCommandRequestAcceptsPacmanPackageNamedAfterShell -count=1` then reproduced the exact invalid rejection: direct `/usr/bin/pacman -D --asexplicit zsh` failed at `argv[2]`. |
| GREEN | Factory-local policy/evidence gates now construct authorized bootstrap, desktop, and fingerprint descriptors; Galaxy-specific values are segregated under `templates/hosts/galaxy/<riskCapability>/**`; generic workstation bases live under `templates/roles/workstation/**`; embed/sync declarations and the generated source manifest were updated. |
| TRIANGULATE | The eight-risk table covers omitted, allowed+ready, allowed+blocked, and allowed+unknown states. Fake plans prove a synthetic portable host omits every risky step and Galaxy asset despite ready observations, while the Galaxy policy retains its authorized bootstrap, desktop, fingerprint, and owned-asset behavior. Corrective tests also prove fixed-display, fixed-input, and literal-home assets cannot cross capability boundaries and an undeclared role cannot authorize workstation assets. |
| REFACTOR | Authorization remains beside each CachyOS factory and command construction. Role assets are selected only from `ResolvedHostPolicy.Desired.Templates` under an exactly declared role; host templates require the selected host and exact risk-capability directory. The runner exception remains restricted to shell-named data operands of direct `pacman`; shell executables, shell-form flags, `sudo`, scope/elevation, and all other validation remain denied. Manifest expectations follow canonical declaration order. |

### Verification

- Minimal regression: `go test ./internal/runner -run TestValidateCommandRequestAcceptsPacmanPackageNamedAfterShell -count=1` — RED before the correction, then PASS.
- Focused: `go test ./internal/runner ./internal/platform/cachyos ./internal/assets ./tools/sync-assets -count=1` — PASS.
- Runtime harness: `go test ./internal/platform/cachyos -run 'TestPortableFakePlanExcludesRiskyStepsAndGalaxyAssetsDespiteObservations|TestGalaxyPolicyPreservesRiskyBootstrapAndOwnedAssets' -count=1` — PASS; synthetic portable and Galaxy plans were exercised without live hardware.
- Exact repository chain: `go test ./... && bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]' && go run ./tools/sync-assets --check` — PASS.
- Static analysis: `go vet ./...` — PASS.
- Earlier bounded failures remain preserved in the native ledger at `sha256:63ca6eaeeff58319f68557061f6bb65e3b2bc98ce6291fdbaf118e80dc8de01d` and `sha256:a34ec8707c682aab0cfe8f73695326c8a3ae1c136943c9e25652c183aa3313f6`; the parent owns settlement of this corrected attempt.

### Gatekeeper correction

- RED: `go test ./internal/platform/cachyos ./internal/assets -run 'TestDesktopHostAssetsAreCapabilityIsolated|TestDesktopRejectsWorkstationAssetsWithoutDeclaredRole|TestGalaxyCapabilityFragmentsDoNotCrossRiskBoundaries' -count=1` failed because fixed-display and literal-home steps selected the same Noctalia file, fixed-input assets lacked a capability owner, workstation assets were emitted without the workstation role, and the segregated fragments did not exist.
- GREEN: the same command passed after capability-addressed Galaxy fragments, exact role-owned template selection, and host+risk validation were implemented.
- Focused rerun: `go test ./internal/runner ./internal/platform/cachyos ./internal/assets ./tools/sync-assets -count=1` — PASS.
- Canonical rerun: the exact repository chain above, `go vet ./...`, and `git diff --check` — PASS.
- Scope: only WU-2 platform ownership, assets, tests, generated manifest, and cumulative evidence changed; the pacman validator correction was preserved, and WU-3 remains untouched.

### Rollback and bounded diff inventory

- Rollback: revert `internal/platform/cachyos/{policy,bootstrap,bootstrap_requests,desktop,fingerprint}.go`, their focused tests, the narrow `internal/runner` pacman-data validation seam and regression, `templates/{hosts/galaxy,roles/workstation}/**`, `templates/embed.go`, `templates/desktop/packages.pacman`, the README layout entry, `tools/sync-assets/main*.go`, and regenerated asset data/manifest. WU-1 catalog authority remains independently usable.
- Product behavior: risk authorization stays inside CachyOS factories; planner types and receipt schema are unchanged. No production portable host name, display, device, home path, or hardware observation was added.
- Asset behavior: Galaxy behavior is composed from generic workstation bases plus separately governed `fixedDisplays`, `fixedInputDevices`, and `literalHomePaths` fragments. No single capability fragment contains another capability's values; portable workstation bases contain no Galaxy overlay, fixed display/device identity, or literal `/home/alex` path.
- Artifact diff: tasks 2.1–2.5 are checked and this section is merged after the complete WU-1 evidence. WU-3 and parent-change artifacts remain untouched.
- Repository actions: no staging, commit, push, PR, or branch mutation.

## WU-3 — Authorization, acceptance, and parent handoff

### RED → GREEN → TRIANGULATE → REFACTOR

| Stage | Evidence |
|---|---|
| RED | `go test ./internal/cli ./internal/app -count=1` failed on the absent `IntegrationTarget`, evidence types, renderer, and authorization gate. `go test ./internal/acceptance -count=1` then failed on the absent fixture report API. A focused secret test reproduced raw verifier-error leakage before it was redacted. |
| GREEN | CLI parsing/help now carries `--integration-target HOST` independently of `--host`. `ResolveVerification` resolves known hosts before the live verifier, returns fixture evidence when the target is absent, rejects mismatches and unknown hosts before the live port, requires an immutable receipt ID for live evidence, and renders only canonical allowlisted fields. The receipt v1 schema is unchanged. |
| TRIANGULATE | Galaxy and unmistakably synthetic portable catalog fixtures resolve through the real repository and CachyOS three-module factory path. Goldens record merge trace, exact test-only pin, catalog/plan digests, topological step order, risk opt-ins, offline check, dry-run non-mutation, first/second convergence (`secondNoChange:true`), typed rollback, and deferred production/live state. Portable evidence has workstation-role assets and zero Galaxy overlay/path or risky-step leakage despite ready observations. |
| REFACTOR | Deterministic evidence rendering is isolated in `internal/acceptance`; target authorization is isolated in `internal/app`. Live verifier failures are reduced to a typed safe error rather than propagating command output or secrets. The fixture report explicitly declares its amendment-only three-module scope so it cannot be mistaken for the parent seven-module acceptance result. |

### Verification

- Focused: `go test ./internal/cli ./internal/app ./internal/acceptance -count=1` — PASS.
- Hardware-free runtime harness: `go test ./internal/acceptance -run 'TestCatalogFixturesMatchAcceptanceGoldens|TestHardwareFreeFixtureConvergenceAndRollbackRefusal' -v -count=1` — PASS. It used synthetic in-memory/file fixtures only; no live machine, hardware, network mutation, dry-run lock acquisition, or production host value was used.
- Canonical repository chain: `go test ./... && bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]' && go run ./tools/sync-assets --check` — PASS.
- Static/diff checks: `go vet ./...` and `git diff --check` — PASS.
- CodeGraph: `codegraph sync && codegraph status` — PASS; the worktree index is up to date.

### Parent integration contract and transition guard

| Parent unit | Amendment evidence | Parent-owned remaining gate |
|---|---|---|
| WU-5 | Multi-host resolution, trace, stable digest, and exact fixture pins | Add reviewed production catalogs/pins without copying synthetic values. |
| WU-6 | Galaxy and portable synthetic golden shape | Run the complete seven-module fixture suite. |
| WU-14 | Host-owned risk gates plus role/host asset isolation | Integrate the complete desktop/verify factory surface. |
| WU-20 | Canonical fixture/live evidence boundary | Record final output SHA and immutable live receipt ID on an explicitly selected target. |

- Transition guard: parent WU-5/6/14/20 remain incomplete and parent artifacts were not edited. This amendment must pass `sdd-verify`; only a later native `nextRecommended=archive` permits parent reconciliation.
- Live-target evidence remains intentionally deferred. No synthetic report is promoted to a live receipt, and no production host/hardware values were invented.

### Rollback and bounded diff inventory

- WU-3 rollback: revert `internal/cli/{command.go,flags_test.go}`, `internal/app/integration_target*.go`, `internal/acceptance/**`, and `docs/portable-host-profile.md`; WU-1/WU-2 remain independently usable.
- WU-3 product files: one CLI parser file, one app authorization/evidence file, one acceptance renderer, synthetic catalog/golden fixtures, focused tests, and one workflow/handoff document. Receipt schema, parent change artifacts, and live-machine paths are unchanged.
- Repository actions: no staging, commit, push, PR, branch mutation, or live mutation.

### Gatekeeper correction

- RED success-path validation: `go test ./internal/app -run 'TestResolveVerificationRejectsSensitiveSuccessfulResult' -count=1` failed before production changes because successful verifier receipt/check fields had no validator and the typed invalid-evidence sentinel did not exist.
- GREEN success-path validation: receipt identities now accept only bounded canonical `receipt-*`, `run-*`, or lowercase `sha256:*` forms and reject sensitive markers; check names and statuses use closed allowlists, duplicate names and oversized result sets fail closed, and all rejection errors omit supplied values. Focused app tests pass.
- RED application evidence: `go test ./internal/acceptance -run 'TestHardwareFreeFixtureConvergenceAndRollbackRefusal|TestCatalogFixturesMatchAcceptanceGoldens' -count=1` failed on the absent real exercise API. After the real path was added, the stale goldens failed with the derived 16-step Galaxy and 4-step portable mutation counts, proving the previous constant evidence was displaced.
- GREEN application evidence: `ExerciseFixturePlan` runs each resolved plan through the real executor and `app.Applier` using in-memory observer/mutator ports and isolated temporary state. Dry-run succeeds while the application lock is already held, publishes an audit receipt, records zero application/network mutation and unchanged state; first apply mutates the real plan, and second apply records zero mutations with `noChange:true`. `BuildFixtureReport` rejects any execution value not produced by this path.
- Corrective focused chain: app success/error validation, acceptance goldens/application evidence, undriven-report rejection, and `go test ./internal/cli ./internal/app ./internal/acceptance -count=1` — PASS.
- Corrective canonical chain: full repository tests, shell syntax, profile JSON parsing, `go run ./tools/sync-assets --check`, `go vet ./...`, and `git diff --check` — PASS.
- Scope remains WU-3 only: WU-1/WU-2 behavior, receipt schema, parent artifacts, production values, and live-machine state are unchanged.
