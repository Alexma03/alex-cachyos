# Apply Progress — rebuild-reproducible-configurator

## Closure status

Implementation is **paused and cleanly closed at the user's request**. The integrated branch is preserved for later continuation; no push, PR, release, archive, or integration into `main` was performed.

- Integration branch: `work/rebuild-configurator-integration`
- Integrated HEAD: `7bc851119fa7601354ae6c56e89ea02e02c91569`
- Integrated range: 52 local commits after baseline `8397ddc7aad00af1a5686418a3250db58b93be43`
- Integrated scope: 122 files, 22,105 insertions
- OpenSpec task ledger: **52 checked / 71 unchecked / 123 total**
- Numeric line cap: **removed by explicit user decision**; future units are split only by behavioral cohesion and review risk.

## Verification at closure

The integration worktree is clean. The following commands passed from the integration branch:

```bash
go test ./...
go vet ./...
go run ./tools/sync-assets --check
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
```

The preserved Bash path remains valid and has not been removed or made the default fallback target.

## Integrated behavior

- Go CLI scaffold, XDG paths, deterministic embedded assets, and source manifest.
- Catalog schema/decode/validation, presence-aware merge, source-specific pins, canonical normalization/digest, and host resolution without machine identifiers.
- Deterministic planner, network grouping, fake/real runners, argv-only execution, and elevation decorator.
- Immutable receipt schema/store, atomic current index, and safe biometric identifiers.
- Non-destructive Git checkout/update/tag/worktree behavior.
- One-time adoption backups, profile suggestion, and rollback preconditions.
- Fail-closed JSON comparison, convergent executor, lock-safe apply orchestration, dry-run projection, and receipt publication.
- Pure offline drift-classification engine.
- Zero-copy template/overlay embedding and copied-vs-embedded sync semantics.
- CachyOS bootstrap request kernel and observation-driven convergence planner.
- Webapp parsing/rendering and primary-icon → favicon fallback planning.

## Preserved unintegrated work

The user selected **local WIP commits, then worktree cleanup**. These branches are intentionally not cherry-picked into integration:

| Area | WIP commit | State |
|---|---|---|
| Receipt/adoption race-safe readers | `178b39f` | Focused tests green; security review incomplete after tool failure |
| Fingerprint convergence | `a9206ce` | Cancelled mid-correction; current WIP does not compile |
| Vicinae convergence | `32df398` | Focused tests green; independent review not completed |

Older interrupted or superseded drafts were also archived as local WIP commits on their original branches before cleanup. They are historical recovery points, not integration candidates.

## Remaining work

- Complete fingerprint, Vicinae, devtools, and apps package/service factories.
- Implement desktop/verify factories and explicit hardware gate.
- Connect the pure checker to CachyOS observations.
- Complete CLI surfaces for check/adopt/rollback/checkpoint/receipt.
- Implement the typed privileged self-helper.
- Author the real global/role/Galaxy catalog and full seven-module fixture.
- Complete acceptance, docs, task reconciliation, sync, and archive.
- WU-15 through WU-19 (Pi/Gentle AI reproduction) remain frozen and unstarted until the base configurator is complete and the user declares the live Pi configuration definitive.

## Repository safety

- The main `alex-cachyos` worktree remains untouched and dirty as originally preserved.
- No stash, clean, hard reset, push, PR, release, or destructive main-worktree operation was performed.
- All obsolete rebuild worktrees were removed only after becoming clean; local branches and WIP commits preserve recoverable history.
- The sole remaining rebuild worktree is the clean integration worktree.

## Post-closure continuation — WU-8 security completion

WU-8 tasks 8.3 and 8.6 are complete in the isolated `codex/rebuild-wu8-security` worktree. No commit, staging, cherry-pick, push, PR, or main-worktree mutation was performed.

### RED → GREEN → TRIANGULATE → REFACTOR evidence

- **RED:** `go test ./internal/safefile ./internal/adopt ./internal/receipt -count=1` failed to compile because the descriptor-relative reader, adoption ownership/lookup fields, web-search boundary, and receipt `Current` API did not exist.
- **GREEN:** focused tests passed after adding Linux `openat2`-based bounded reads, descriptor-bound receipt/adoption reads, UID/GID + exact-mode validation, target identity checks, and metadata-only unmanaged web-search handling.
- **TRIANGULATE:** deterministic regular-file and symlink swaps between adoption inspection and backup open are rejected; current index/receipt path, mode, symlink, size, digest, and run-ID cases fail closed; corrupt records and changed/symlink backups fail closed; unmanaged `web-search.json` proves zero reads and zero backups while configurator-created content is compared only by SHA-256.
- **REFACTOR:** shared `internal/safefile` owns strict relative path validation, trusted directory descriptors, bounded reads, and before/after metadata identity checks. Backup mode/owner changes use the open descriptor rather than reopening the backup pathname.

### Work-unit verification and rollback

- Focused: `go test ./internal/safefile ./internal/adopt ./internal/receipt -count=1 && go vet ./internal/safefile ./internal/adopt ./internal/receipt` — exit 0.
- Full: `go test ./... && go vet ./... && bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]' && go run ./tools/sync-assets --check` — exit 0.
- Unit-owned paths: `go.mod`, `go.sum`, `internal/safefile/**`, `internal/adopt/adopt.go`, `internal/adopt/security_test.go`, `internal/adopt/web_search.go`, `internal/receipt/store.go`, `internal/receipt/store_security_test.go`, `openspec/changes/rebuild-reproducible-configurator/tasks.md`, and this file.
- Rollback is path-bounded to the unit-owned inventory. All tests use temporary directories; no host file, real Pi state, dependency checkout, index, or main worktree is modified.

## WU-10 continuation — production execution and privileged helper

Tasks **10.4, 10.5, and 10.6** are complete in the isolated `wu10-helper`
worktree. No commit, staging, push, PR, or main-worktree mutation was performed.

### RED → GREEN → TRIANGULATE → REFACTOR evidence

- **RED:** `go test ./internal/runner ./internal/executor` failed to compile on
  the missing typed self-helper and command-mutator APIs. `go test
  ./cmd/alex-cachyos` then failed to compile on the missing hidden helper runtime.
- **GREEN:** added content-addressed user staging, a bounded typed JSON helper
  request, the fixed same-binary `pkexec` invocation, root-side exact
  destination/mode policy, source owner/regular-file/hash revalidation,
  descriptor-relative candidate creation and atomic rename, and the production
  planner-step-to-command bridge.
- **TRIANGULATE:** rejection tests cover unknown operation, non-allowlisted
  destination, mode/hash mismatch, symlink source, trailing/oversized input,
  whole-batch prevalidation, and a deterministic source-path swap after open.
  Explicit spies prove dry-run reaches neither the command runner nor network
  authorization and check reaches only its observation port.
- **REFACTOR:** production wiring is kept behind `NewCommandApplier` and
  `NewProductionApplier`; the platform-specific exact destination policy stays
  in `internal/platform/cachyos`, while the generic root transaction stays in
  `internal/runner`. The pre-existing runner subprocess fixture timeout was
  raised from one to five seconds after race instrumentation proved one second
  was too short; the behavior-specific timeout test remains unchanged.

### Verification evidence

- Focused: `go test -count=1 -run
  'Test(SelfHelper|CommandMutator|CommandApplier|CheckUsesOnly)'
  ./internal/runner ./internal/executor ./internal/app ./cmd/alex-cachyos
  ./internal/platform/cachyos` — exit 0.
- Race-focused: `go test -race -count=1 ./internal/runner
  ./internal/executor ./internal/app ./cmd/alex-cachyos` — exit 0 after the
  fixture-timeout correction.
- Full: `go test ./... && go vet ./... && bash -n apply
  bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import
  json; from pathlib import Path; [json.load(p.open()) for p in
  Path("profiles").glob("*.json")]' && go run ./tools/sync-assets --check` —
  exit 0.
- Runtime harness: `go run ./cmd/alex-cachyos --list` — exit 0 with the seven
  expected modules. Live `pkexec` was intentionally not invoked; the same-binary
  wrapper and root transaction are covered through the fake elevation port and
  isolated temporary directories.

### Work-unit boundary and rollback

- Unit-owned paths: `cmd/alex-cachyos/main.go`,
  `cmd/alex-cachyos/main_test.go`, `internal/app/apply.go`,
  `internal/app/apply_test.go`, `internal/app/check_test.go`,
  `internal/executor/command.go`, `internal/executor/command_test.go`,
  `internal/platform/cachyos/privileged.go`,
  `internal/platform/cachyos/privileged_test.go`, `internal/runner/exec_test.go`,
  `internal/runner/self_helper.go`, `internal/runner/self_helper_test.go`, and
  this WU-10 ledger update.
- Bounded work-unit stat, including the seven untracked unit-owned files omitted
  by plain `git diff --stat`: **14 paths, 998 insertions, 9 deletions** (tracked
  stat: 7 paths, 184 insertions, 9 deletions; untracked: 7 paths, 814 lines).
- Rollback boundary: remove the new helper/command-adapter files and tests, then
  revert the narrow constructor, CLI helper dispatch, runner fixture timeout,
  and WU-10 ledger hunks. Staged source files and destination candidates are
  outside repository state and are cleaned/refused by the helper transaction;
  no live system file was touched by verification.

## WU-13 — Platform devtools + apps + Vicinae (2026-09-01)

Status: **implemented and verified in the isolated `wu13-vicinae` worktree; not staged or committed**.

### RED → GREEN → TRIANGULATE → REFACTOR evidence

| Task | RED evidence | GREEN / triangulation evidence |
|---|---|---|
| Devtools | `go test ./internal/platform/cachyos` failed to compile because `DevtoolsObservation`, `DevtoolsFileObservation`, and `BuildDevtoolsRequestPlan` did not exist. | The factory loads embedded mise/npm/pnpm assets, renders `@HOME@`, converges one marker block per zsh/bash/fish file, emits typed Pacman and mise requests, and matches golden digest `010d5519a63cc75e3d35fefde625d798b5d269c3a8e4fbc662604c5b94251085`. |
| Apps | The same focused RED failed because `AppsObservation`, `AppsFileObservation`, and apps plan factories did not exist. | Exact embedded Pacman/AUR names, catalog `AURLocalPin` commit/checksum validation, services/groups, webapp launcher/files, and typed favicon fallback pass focused tests. `paru` is an unelevated user-scope request; system Pacman/systemctl/usermod requests remain system-scoped for the WU-10 elevation decorator/helper. |
| Vicinae | The same focused RED failed because `VicinaeObservation` and the Vicinae plan factories did not exist. | Read-only salvage evidence from `32df398` was adapted without cherry-picking: exact `vicinae-bin`, catalog-pinned local `paru` build/install, user service enable/start, embedded environment/shortcut files, strict COSMIC launcher rewrite, removal/inverse behavior, and typed observations pass. |
| Triangulation | A temporary golden sentinel intentionally failed with the emitted deterministic digest before it was fixed. | Devtools, apps, and Vicinae each prove a fully satisfied second plan. Planner invariants prove every apps step precedes a desktop module that declares `DependsOn: [apps]`. Marker rewrite and COSMIC launcher rewrite are idempotent. |

Focused verification:

```text
go test ./internal/platform/cachyos -count=1
ok alex-cachyos/internal/platform/cachyos

go vet ./internal/platform/cachyos
exit 0
```

Full verification and transition guard:

```text
go test ./...                                      exit 0
go vet ./...                                       exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
                                                    exit 0
go run ./tools/sync-assets --check                 exit 0
```

### WU-13 corrective apply — Vicinae catalog pin enforcement

The remaining Vicinae defect was the same reproducibility class already corrected for Apps: `vicinae.package.install` emitted `paru -S --needed --noconfirm vicinae-bin`, so the catalog `sourceCommit` and `patchSHA256` could not control the installed bytes.

Focused RED:

```text
go test ./internal/platform/cachyos -run TestVicinaeFactoryUsesExactPackageAndUserServiceRequests -count=1
build failed: VicinaeObservation had no catalog pin authority for the regression
exit 1
```

GREEN replaces the mutable package-name install with the catalog-owned sequence: clone/fetch the canonical AUR remote, detach at the exact `sourceCommit`, materialize the commit patch deterministically, verify `patchSHA256` using `sha256sum --check --strict`, then run `paru -B --install` against the verified local source directory. The final install step depends on checksum verification. Every operation remains argv-only, user-scoped, validator-approved, and secret-safe; service and file behavior is unchanged.

Focused evidence:

```text
go test ./internal/platform/cachyos -run TestVicinaeFactoryUsesExactPackageAndUserServiceRequests -count=1
ok alex-cachyos/internal/platform/cachyos

go test ./internal/platform/cachyos -run TestVicinae -count=1
ok alex-cachyos/internal/platform/cachyos

go test ./internal/platform/cachyos -count=1
ok alex-cachyos/internal/platform/cachyos
```

Integration wiring remains outside this isolated WU-13 correction and is reserved for the combined integration target.

Post-correction canonical verification:

```text
go test ./...                                      exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 profile JSON validation                    exit 0
go run ./tools/sync-assets --check                 exit 0
go vet ./...                                       exit 0
git diff --check                                   exit 0
```

Baseline remains `a6aee2adc3a575038e1511ca8d68d96aeecc716e`; no staging, commit, push, PR, or main-worktree mutation occurred.

Unit-owned paths:

- `internal/platform/cachyos/devtools.go`
- `internal/platform/cachyos/devtools_test.go`
- `internal/platform/cachyos/apps.go`
- `internal/platform/cachyos/apps_test.go`
- `internal/platform/cachyos/vicinae.go`
- `internal/platform/cachyos/vicinae_test.go`
- `openspec/changes/rebuild-reproducible-configurator/tasks.md`
- `openspec/changes/rebuild-reproducible-configurator/apply-progress.md`

Bounded unit diff stat, counting the six intended untracked Go files together with the two tracked OpenSpec updates:

```text
8 files changed, 3007 insertions(+), 5 deletions(-)
```

### WU-13 corrective apply — pin-enforced AUR/local installation

The fresh apply gate rejected the first GREEN because the catalog pins were only recorded as planner metadata while installation still used `paru -S <name>`. That command resolves mutable AUR heads and therefore did not let the requested source commit and checksum control the installed bytes.

Corrective RED:

```text
go test ./internal/platform/cachyos -run TestAppsAURInstallationsAreBoundToExactSourceAndChecksumPins -count=1
--- FAIL: TestAppsAURInstallationsAreBoundToExactSourceAndChecksumPins
missing request "apps.aur.ai-usagebar-bin.checkout"; only the aggregate mutable apps.packages.aur.install request existed
exit 1
```

Corrective GREEN binds every missing AUR/local package to five argv-only user-scope operations: clone/fetch its canonical AUR Git remote, detach at the catalog `sourceCommit`, materialize that commit's binary patch deterministically, verify the catalog `patchSHA256` with `sha256sum --check --strict`, then run `paru -B --install` against that verified local directory. The install step depends on the checksum-verification step; changing either pin changes the corresponding request identity. No shell, `sudo`, embedded `pkexec`, or WU-10 helper bypass was introduced. Pacman/systemctl/usermod requests remain system-scoped for the WU-10 elevation decorator.

Exact race evidence:

```text
go test -race ./internal/platform/cachyos -count=1
ok  alex-cachyos/internal/platform/cachyos  7.676s
exit 0
```

Runtime-harness evidence:

```text
paru --help | grep -E -- 'paru \{-B --build\}|-i --install' | head -2
    paru {-B --build}       [dir(s)]
    -i --install          Install package as well as building
exit 0
```

A live AUR installation harness is **N/A** for this isolated work unit: it would fetch/build third-party packages and install them into the developer's real CachyOS package database, violating the hardware-independent fake-runner requirement and the explicit no-host-mutation boundary. The focused test is the applicable deterministic harness: it validates every generated request, exact commit/checksum propagation, dependency binding, local-directory install argv, pin-sensitive identities, and user/system scope without executing host mutations.

Corrective verification:

```text
go test ./internal/platform/cachyos -count=1       exit 0 (0.417s)
go test -race ./internal/platform/cachyos -count=1 exit 0 (7.676s)
go test ./...                                      exit 0
go vet ./...                                       exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 profile JSON validation                    exit 0
go run ./tools/sync-assets --check                 exit 0
```

## WU-12 reconciliation — bootstrap proof and pinned Chrome source (2026-09-01)

WU-12.2 was reconciled against the current typed bootstrap implementation. The existing structural tests already prove the full-system repository/transaction policy and pre-transaction versions, exact wanted/removal names, explicit ownership, independent atomic GRUB and Plymouth chains, package-authoritative ananicy-cpp/UFW services, marker-block zsh convergence, and the exact CachyOS LTS deferral allowlist. The remaining concrete defect was `bootstrap.chrome.install`: it still planned mutable `paru -S google-chrome` resolution.

RED:

```text
go test ./internal/platform/cachyos -run 'TestBootstrap(RequestsUseEmbeddedListsAndExactDeltas|ModuleUsesResolvedPolicyChromePin)' -count=1
build failed: BootstrapInputs lacked HomeRoot/ChromePin and BootstrapObservation lacked HomeRoot/Catalog
exit 1
```

GREEN binds missing Chrome installation to the resolved catalog `aurLocal.google-chrome` pin. The module now emits typed user-scope operations to clone/fetch the canonical AUR remote, detach at `sourceCommit`, materialize the commit patch deterministically, verify `patchSHA256` with `sha256sum --check --strict`, and install only the verified local directory with `paru -B --install --needed --noconfirm`. The install step depends on checksum verification. Runtime observations cannot override resolved policy pins, and disabled/installed Chrome paths do not require a pin or cross the network boundary.

Focused evidence:

```text
go test ./internal/platform/cachyos -run 'TestBootstrap(RequestsUseEmbeddedListsAndExactDeltas|ModuleUsesResolvedPolicyChromePin)' -count=1
ok  alex-cachyos/internal/platform/cachyos  0.006s

go test ./internal/platform/cachyos -count=1
ok  alex-cachyos/internal/platform/cachyos  0.429s
```

Canonical verification:

```text
go test ./...                                      exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 profile JSON validation                    exit 0
go run ./tools/sync-assets --check                 exit 0
go vet ./...                                       exit 0
```

Runtime harness: **N/A**. The changed boundary is a pure request planner exercised through typed fake-plan assertions. A live Chrome AUR build/install would mutate the developer's package database and violate the explicit no-live-mutation boundary.

Fingerprint reconciliation deliberately did not advance WU-12.3–12.6. The current `fingerprint.go` exposes only one policy-gated high-level step; it does not yet prove exact installed commit+pkgrel convergence, a closed source/checksum/build/artifact binding, `pacman -U`, or byte-exact PAM overlay application. The repository packaging files contain a static commit and patch checksum, and `packaging/` is already sync-declared, but no real resolved Galaxy catalog pin or production observation exists. Marking those tasks complete would invent the missing authority and hardware-dependent evidence.

Rollback boundary: revert the four bootstrap implementation/test files and these two cumulative OpenSpec updates. No live package, service, boot, or fingerprint state was changed.

### WU-11 partial apply — typed command surfaces and offline check boundary

Baseline: `056a576c1931562e85408547e1f0cbb33729f7c1` on isolated branch
`codex/rebuild-command-surfaces`. This slice used ordinary repository tooling
only and did not touch `.atl` or any live host state.

Completed evidence:

- The existing offline checker covers the full WU-11 inventory and remains
  observation-only. A new typed `CheckRequestFactory`/`NewCheckCommand` boundary
  derives inventory from resolved host policy and parsed selection, then invokes
  only `CheckObserver`; seeded post-apply drift reports its recorded backup path.
- `internal/cli` parses and validates `apply`, `check`, `adopt`, `rollback`,
  `checkpoint create`, `receipt show`, and `status`, including the preserved
  compatibility flags and deterministic `--json` output selection.
- `internal/app.CommandService` resolves known host and catalog authority before
  command handlers. The unknown-host regression proves zero repository resolve,
  hostname, and operation calls for an unknown explicit host.
- `CommandHandlers` gives every surface a distinct typed handler and fails closed
  when a handler is absent. Selection and removal slices are defensively copied,
  and invalid receipt results are rejected rather than returned by alias.
- The binary propagates selection, dry-run, host, rollback, adoption, and
  checkpoint intent through the typed request; human and JSON output share typed
  results, drift exits non-zero, lock contention remains exit 75, and arbitrary
  runtime errors cannot expose raw output or secret-like values.
- Production-independent `receipt show` and `status` read the atomically selected
  current immutable receipt through XDG state and do not require a catalog.
  No production catalog values were added.
- `docs/rollback.md` documents inverse preconditions, immutable new receipts,
  Snapper ownership, and dirty-worktree safety without claiming the still-missing
  production rollback adapters.

RED evidence:

```text
go test ./internal/cli ./internal/app -count=1
internal/cli: Command/CommandApply/CommandCheck and command option fields undefined
internal/app: CommandName/CommandRequest/CommandResult/NewCommandService undefined
exit 1

go test ./cmd/alex-cachyos -count=1
cmd/alex-cachyos/main_test.go: runWithRuntime undefined
exit 1

go test ./internal/app -run TestCommandHandlers -count=1
internal/app/commands_test.go: CommandHandlers undefined
exit 1

go test ./internal/app -run TestCheckCommandBuildsHostInventory -count=1
internal/app/commands_test.go: CheckRequestFactoryFunc/NewCheckCommand undefined
exit 1

go test ./internal/app -run TestCommandServiceRejectsInvalidReceiptResults -count=1
FAIL: invalid receipt result returned without error
exit 1

go test ./internal/app -run TestCommandServiceDoesNotRequireCatalog -count=1
FAIL: receipt returned command runtime unavailable
exit 1
```

Focused GREEN:

```text
go test ./internal/app -run 'Test(Check|Command)' -v -count=1
PASS

go test ./internal/cli ./cmd/alex-cachyos -v -count=1
PASS

go test ./internal/app -run 'TestCommandServiceRejectsInvalidReceiptResults|TestCommandServiceDoesNotRequireCatalog' -count=1
PASS
```

Canonical verification:

```text
go test ./...                                      exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 profile JSON validation                    exit 0
go run ./tools/sync-assets --check                 exit 0
go vet ./...                                       exit 0
git diff --check                                   exit 0
```

Tasks proven complete in this slice: WU-11.1 and WU-11.5. WU-11.2,
WU-11.3, WU-11.4, and WU-11.6 remain unchecked. The command grammar, typed
routing, output, and current-receipt reads are implemented, but full production
command execution still depends on:

1. production catalog documents and composition (currently intentionally
   absent; fixture values cannot become machine authority);
2. the WU-14 verify-module inventory/observer needed by production `check`;
3. receipt-by-ID loading, tagged-catalog object-read/isolation, and rollback
   inverse execution/new-receipt publication;
4. checkpoint committed-catalog/assets validation and production Git identity
   composition; and
5. complete apply/adopt plan construction from the final module inventory.

No stubs claim success: unavailable production handlers return the typed,
sanitized `command runtime unavailable` failure.

Unit-owned paths:

- `cmd/alex-cachyos/main.go`
- `cmd/alex-cachyos/main_test.go`
- `internal/cli/command.go`
- `internal/cli/command_test.go`
- `internal/cli/output.go`
- `internal/app/commands.go`
- `internal/app/commands_test.go`
- `docs/rollback.md`
- `openspec/changes/rebuild-reproducible-configurator/tasks.md`
- `openspec/changes/rebuild-reproducible-configurator/apply-progress.md`

Bounded partial-unit diff before staging: 10 files, 1,162 insertions, 12
deletions. `.atl` has no status entries; the worktree-local `.codegraph/` index
is excluded by the user's global Git ignore and is not part of the unit.

## WU-14.1–14.3 — Typed desktop and exact-target verification subset (2026-09-01)

Status: **maximal hardware-independent subset implemented and verified in the isolated `wu14-desktop-verify` worktree; no live host was selected or mutated**.

### RED → GREEN → TRIANGULATE → REFACTOR evidence

- **RED:** `go test ./internal/platform/cachyos -run 'Test(Desktop|OSDesktop)' -count=1` failed to compile because `BuildDesktopRequestPlan`, `NewDesktopRuntime`, `NewOSDesktopFilePort`, and the desktop live-verifier APIs did not exist.
- **GREEN:** the desktop factory reads the embedded authoritative package inventory and emits distinct typed package, role-file, Quickshell-polkit, `.dmrc`, niri-validation, Noctalia-validation, and package-validation operations. Production-capable Pacman observation, executor mutation dispatch, descriptor-relative file observation, one-time adoption backup, and same-directory atomic publication are covered without wrapping the legacy shell module.
- **TRIANGULATE:** in-memory ports prove dry-run performs zero package/file mutations, first apply converges the typed plan, and the second apply performs zero mutations. Symlink targets fail closed. Unknown or mismatched integration targets invoke zero live verifier commands; an exact catalog-known match is the only route to allowlisted receipt-backed evidence. Galaxy and portable fixture goldens now expose the expanded desktop operation order without portable host-specific leakage.
- **REFACTOR:** role-owned workstation assets remain under `templates/roles/workstation`; host-owned fixed display/input/literal-home assets remain behind their independent capability gates. No production hostname, hardware identifier, display, input device, or home path was invented.

### Verification

```text
go test ./internal/platform/cachyos -run 'Test(Desktop|OSDesktop|RunnerDesktop)' -count=1   exit 0
go test ./internal/acceptance -count=1                                                     exit 0
go test ./...                                                                                exit 0
go vet ./...                                                                                 exit 0
go run ./tools/sync-assets --check                                                           exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh                           exit 0
python3 profile JSON validation                                                             exit 0
git diff --check                                                                             exit 0
```

No WU-14 checkbox is advanced by this subset. The first row still requires Cosmic prune/PAM/plugins/portals completeness; the second still requires the full WU-11 inventory and explicit final-module planner proof; the hardware row still requires its complete CLI string/exit-code contract; and the acceptance row still requires the full seven-module Galaxy plus overlay-sync evidence. Live package/service/desktop validation remains pending until a real catalog host and explicit matching integration target are supplied.

## WU-11 completion — injected command composition, rollback, receipts, and checkpoints (2026-09-01)

Baseline: integrated `5db9f20` on isolated branch `codex/rebuild-command-completion`. This target-independent slice used ordinary repository tooling only. It introduced no production hostname, catalog, hardware value, live mutation, or Gentle AI state.

Completed behavior:

- Immutable receipt lookup now resolves historical evidence by validated run ID through the descriptor-open state root. The scanner validates owner/mode, rejects symlinks and corrupt/ambiguous stores, and never infers a receipt filename from caller input.
- Typed application constructors now compose apply, check, adopt, rollback, checkpoint, receipt, and status from injected repositories/factories/ports. The default binary remains fail-closed for catalog-dependent operations until a production catalog is supplied; current/by-ID receipt reads remain production-independent.
- Receipt rollback acquires the shared mutation lock, observes and validates every managed-file precondition before any inverse executes, performs created/adopted/package-owned typed inverses in stable order, and publishes a distinct immutable rollback receipt. Existing source receipts are never edited. System transactions are reported as `snapper-delegated`; no competing system rollback engine was added.
- Tagged rollback reads only caller-declared files through validated Git object reads, computes a stable snapshot digest, forwards `--remove` modules to the injected reapply planner, and records the reapplied catalog tag in a new receipt. `planner.BuildRemovalPlan` maps selected steps to declared inverses in reverse application order and refuses missing/unknown authority.
- Checkpoint composition acquires the mutation lock, validates injected catalog and managed-asset paths are present and unchanged at HEAD, and only then creates the existing annotated `catalog-vX.Y.Z` tag. The path-limited Git validation deliberately ignores unrelated dirty files and never commits, checks out, stashes, resets, or forces.
- Human/JSON adapters now render typed adoption, rollback, and checkpoint results and map receipt lookup, rollback conflict, checkpoint validation, catalog-tag, usage, drift, and lock-contention failures to deterministic sanitized strings/exit codes.

RED evidence was reproduced against an exported clean `HEAD` tree with the new focused tests only:

```text
go test ./internal/app ./internal/receipt ./internal/planner ./internal/gitx ./internal/cli ./cmd/alex-cachyos -count=1
FAIL: NewRollbackCommand/RollbackCommandConfig/MutationLockFactory undefined
FAIL: Store.Read/ErrReceiptNotFound/ErrReceiptAmbiguous undefined
FAIL: BuildRemovalPlan/ErrMissingInverse undefined
FAIL: Client.ValidateCommittedPaths/ErrCheckpointPathNotCommitted undefined
FAIL: typed rollback/checkpoint result fields undefined
FAIL: historical receipt runtime returned command runtime unavailable
RED_EXIT=1
```

Focused GREEN:

```text
go test ./internal/receipt ./internal/app ./internal/planner ./internal/gitx ./internal/cli ./cmd/alex-cachyos -count=1
ok: all six focused package groups
exit 0
```

TRIANGULATE evidence:

- `TestCheckCommandBuildsHostInventoryAndRunsOfflineChecker` seeds managed-file drift, retains the backup path, and produces non-zero exit intent.
- `TestCommandResultPreservesAppliedCatalogIdentity` proves catalog tag, release, and digest survive the typed command boundary.
- `TestRunMapsConcurrentMutatorContentionToExit75` proves a concurrent mutating command maps to exit 75 and the allowlisted message.
- Actual temporary Git repositories prove tagged object reads leave dirty worktree/index/HEAD bytes unchanged and checkpoint validation accepts unrelated dirt while rejecting declared-path drift/untracked data.

Canonical verification:

```text
go test ./...                                      exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
                                                    exit 0
python3 profile JSON validation                    exit 0
go run ./tools/sync-assets --check                 exit 0
go vet ./...                                       exit 0
git diff --check                                   exit 0
```

Tasks proven complete in this slice: WU-11.2, WU-11.3, WU-11.4, and WU-11.6. Parent ledger: **69/123 checked, 54 unchecked**. Production execution still correctly requires the later real catalog/host composition; synthetic fixtures remain tests rather than machine authority.

Rollback boundary: revert the WU-11-owned product/test files and these two cumulative OpenSpec updates. No live package, file, service, Git tag, or host state was changed by verification.
## WU-12.3–12.6 continuation — target-independent fingerprint pipeline (2026-09-01)

Status: **WU-12.3 and WU-12.5 are implemented and verified from resolved fixture
authority; WU-12.4 and WU-12.6 remain open.** The cumulative parent ledger is
**71 checked / 52 unchecked / 123 total** in the integrated worktree.

### RED → GREEN → TRIANGULATE → REFACTOR evidence

- **RED:** `go test ./internal/platform/cachyos -run 'TestFingerprint' -count=1`
  failed to compile because `BuildFingerprintPlan`, the resolved pin and
  observation types, and the typed fingerprint operations did not exist.
- **GREEN:** the planner validates the embedded PKGBUILD SHA-256, exact upstream
  commit, pkgrel, patch SHA-256, provides/conflicts declarations, deterministic
  build environment, and exact artifact SHA-256 before the sole system install
  request: typed `/usr/bin/pacman -U --needed --noconfirm <absolute-local-artifact>`.
  All requests pass `runner.ValidateCommandRequest`; no shell, sudo, mutable
  AUR install, or live mutation is used.
- **TRIANGULATE:** missing/unsafe pins fail closed; a matching installed
  sourceCommit+pkgrel plus byte/mode-exact PAM overlays produces zero command
  requests and satisfied dispositions; default-denied policy never consumes an
  unsafe pin. Replacement of an installed package requires an exact local
  rollback artifact and SHA-256. PAM created/adopted ownership binds only to
  remove-file or the exact one-time backup restore inverse.
- **REFACTOR:** `PlatformEvidence.Fingerprint` is a target-independent resolved
  boundary. Runtime observations cannot become desired authority. The
  production Galaxy catalog adapter and actual artifact pin remain deliberately
  absent rather than being invented.

The Galaxy acceptance fixture now supplies explicit synthetic resolved pins and
exact overlay observations; its golden records the expanded fingerprint step
order only. This is fixture evidence, not a live-host or full seven-module
acceptance claim. The portable fixture golden is unchanged and default-deny
still emits no fingerprint steps.

### Verification

```text
go test ./internal/platform/cachyos -run 'TestFingerprint' -count=1   exit 0
go test ./internal/platform/cachyos -count=1                          exit 0
go test ./internal/acceptance -count=1                                exit 0
go test ./...                                                          exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh    exit 0
python3 profile JSON validation                                       exit 0
go run ./tools/sync-assets --check                                    exit 0
go vet ./...                                                           exit 0
git diff --check                                                       exit 0
```

The first canonical `go test ./...` run exposed the acceptance fixture's
missing resolved fingerprint evidence. That single diagnosed failure was fixed
by adding explicit synthetic pins and overlay observations; the next focused
acceptance run and canonical chain passed.

Runtime harness: **N/A**. This slice is a pure typed planner exercised through
fixture evidence. Running makepkg or `pacman -U` would mutate or depend on the
developer host and violate the explicit no-live-mutation boundary.

WU-12.4 remains unchecked because the production catalog has no authoritative
artifact/pkgrel/source-date pin adapter and the parent module dependency ranks
are not yet fully encoded. WU-12.6 remains unchecked because it is the closure
row for the complete WU-12, including WU-12.4. Packaging and overlays were
already sync-declared and `sync-assets --check` remains green.
## WU-15.1–15.5 — Target-independent Pi core subset (2026-09-01)

Status: **generic fixture contract implemented and verified; zero WU-15 rows
advanced and no production or live-integration claim made**. Baseline was
`5db9f20`; the work ran only in the isolated `wu15-pi-core` worktree and never
opened or mutated a real `~/.pi` tree.

Implemented evidence:

- `internal/pi/packages.go` validates the closed nine-package npm inventory with
  exact semver pins plus the literal `../../Projects/gentle-pi` package. The
  local reference must resolve from `.pi/agent/settings.json` to the named,
  read-only selected checkout and is never copied. A narrow typed installer port
  exposes exact-catalog requests only—there is no update-all or arbitrary argv
  operation. Read-only observation probes both documented npm layouts, reports
  ambiguity, and keeps desired pins separate from resolved versions.
- `internal/pi/render.go` renders the exact minimal settings shape as stable
  one-line JSON with one trailing newline. Only `@HOME@` and `@USER@` resolve;
  empty, multiline, or unknown-token values fail before publication. The valid
  runtime-owned changelog marker remains in rendered bytes but is excluded from
  the desired-state digest and drift decision.
- `internal/pi/routes.go` validates the enum-like set of exactly 23 route names
  and renders both `{model, effort}` and `{model, thinking}` views from one typed
  source. Lean/task behavior and both false flags are explicit. Drift identifies
  the changed file, route, field, and expected value; missing explicit false
  fields and noncanonical encodings also drift.
- `internal/pi/persona.go` renders generic typed persona and background values
  canonically. All render outputs carry SHA-256 digests bound to their intended
  bytes (or, for settings, to desired fields excluding runtime metadata).
- Fixtures contain only conspicuously synthetic versions, remotes, route values,
  persona, and background values. Tests use `t.TempDir()` homes and no production
  host identity, user data, credentials, hardware value, or live Pi state.

RED evidence (bounded):

```text
go test ./internal/pi -run 'Test(BuildPackagePlan|ObservePackages)' -count=1
build failed because the package-plan/probe API did not exist

go test ./internal/pi -run 'Test(Render|CheckRoute)' -count=1
build failed because the render/route/persona API did not exist

go test ./internal/pi -run TestRenderSettingsRejectsForgedPackagePlan -count=1
failed because a forged package spec passed the initial shallow plan validator

go test ./internal/pi -run TestInstallPackagesUsesOnlyTypedExactCatalogRequests -count=1
build failed: undefined: InstallPackages
```

The forged-plan regression was fixed by revalidating the exact package set,
spec/version relationship, local checkout identity, commit shape, ordering, and
install requests at every consumer boundary. The explicit-false regression was
fixed with presence-aware validation so an omitted `debug: false` cannot compare
equal merely through Go zero values.

Focused and canonical verification:

```text
go test ./internal/pi -count=1                                                        exit 0
go test ./...                                                                         exit 0
go vet ./...                                                                          exit 0
go run ./tools/sync-assets --check                                                    exit 0
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh                    exit 0
python3 profile JSON validation                                                       exit 0
git diff --check                                                                      exit 0
```

Rows remain unchecked because the repository does not contain authoritative
production versions for all nine npm packages, the production `{model, level}`
assignments for the 23 routes, or production persona/background values. A
fixture golden cannot substitute for that catalog authority, so WU-15.1–15.5
are only partially evidenced and WU-15.6 integration is intentionally untouched.

Unit-owned paths:

- `internal/pi/packages.go`
- `internal/pi/packages_test.go`
- `internal/pi/persona.go`
- `internal/pi/persona_test.go`
- `internal/pi/render.go`
- `internal/pi/render_test.go`
- `internal/pi/routes.go`
- `internal/pi/routes_test.go`
- `internal/pi/testdata/package-catalog.yaml`
- `internal/pi/testdata/render-authority.json`
- `openspec/changes/rebuild-reproducible-configurator/tasks.md`
- `openspec/changes/rebuild-reproducible-configurator/apply-progress.md`

## 2026-09-14 — Pi / Gentle AI scope removal (User architectural decision)

Per explicit user direction, the entire Pi, Gentle AI, subagent, and model route
reproduction scope was removed from `alex-cachyos`. The installer scope is now
exclusively focused on CachyOS system and desktop provisioning (Niri + Noctalia +
packages + devtools + hardware/fingerprint).

Actions taken:
- Deleted `internal/pi/` entirely (packages, persona, render, routes, testdata).
- Removed Pi package pin validators and references from `internal/catalog/` and test fixtures.
- Removed Pi runtime, Gentle AI invocations, managed assets, and review mode from `internal/receipt/schema.go` and `testdata/receipts/golden-v1.json`.
- Removed 6 Pi-specific specs from `openspec/changes/rebuild-reproducible-configurator/specs/`.
- Dropped WU-15 through WU-19 in `tasks.md`.
- All verification passed: `go test ./... -count=1`, `go vet ./...`, `go run ./tools/sync-assets --check`, and Bash transition guard.
