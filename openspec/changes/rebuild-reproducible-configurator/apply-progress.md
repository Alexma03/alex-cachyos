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
