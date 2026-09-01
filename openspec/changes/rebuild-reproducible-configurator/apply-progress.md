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
