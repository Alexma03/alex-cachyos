# Apply Progress — rebuild-reproducible-configurator

## Cumulative status

- Phase: `apply`; this reconciliation records WU-0 through WU-4, WU-7, and completed WU-9 implementation evidence.
- Total task rows remain **123**: WU-9 adds five checked rows to the previous **32 completed**/**91 pending**, producing **37 completed** and **86 pending**. Ownership markers are unchanged.
- WU-0 through WU-2 remain the previously recorded completed baseline. This reconciliation changes only the OpenSpec progress artifacts; it does not change product code.

## Completed implementation units

- **WU-3 — fully implemented, verified, and integrated:** commits `179dbd9`, `ceef961`, and `b3fbd99`. All four WU-3 task rows are checked.
- **WU-4 — fully implemented, verified, and integrated:** commits `785fb24`, `e6e2c47`, and `bad8167`. All five WU-4 task rows are checked. Current-schema merge is in `merge.go`; future value-kind switch/primitives are in `merge_values.go`; the digest is canonical JSON SHA-256.
- **WU-7 — fully implemented, verified, and integrated:** commits `725a8db`, `e475ff9`, and `3c282ad`. All six WU-7 task rows are checked.
- **WU-9 — fully implemented, verified, and integrated:** commits `8917566` (checkout observation), `4e02a66` (safety/object reads), `e036a73` (dirty-overlap/ff-only advancement), `e0ea53c` (annotated tags), `926aef9` (owned detached worktrees), and `d4aaef9` (report-only updates). All five WU-9 task rows are checked.

## Pending implementation

- WU-5 and WU-8 remain pending; neither unit is marked complete by this reconciliation.

## Verification evidence

The full verification recorded after WU-4 and repeated for WU-9 exited `0` for each component:

```bash
go test ./... && go vet ./... && go run ./tools/sync-assets --check
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
```

Read-only statuses of both Gentle AI and Gentle Pi were byte-identical before and after the WU-9 verification.

## Preserved baseline and external drift warning

- WU-0 remains the historical baseline: 47 worktree entries (`10` deleted, `29` modified, `8` untracked), zero stashes, branch `main`, and HEAD `8397ddc7aad00af1a5686418a3250db58b93be43`; the dependency checkouts were clean at their then-current `main` commits.
- Independent verification later observed concurrent external drift in the Gentle Pi checkout: branch `feat/476-codegraph-worker-verify` with nonzero porcelain state at observation time. It was not cleaned, rewritten, or attributed to this work; it must be re-baselined before the later Gentle Pi integration unit.

## Next step

The next catalog dependency is **WU-5** (source-specific pins, host resolution, and initial catalog authoring). **WU-9 is complete**. WU-5 and WU-8 remain pending and are not marked complete. Verify/archive and parent-owned lifecycle actions remain deferred.
