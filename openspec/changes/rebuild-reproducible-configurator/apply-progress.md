# Apply Progress — rebuild-reproducible-configurator

## Cumulative status

- Phase: `apply`; this reconciliation records WU-0 through WU-4, WU-7, and the partial WU-9 implementation evidence.
- Total task rows remain **123**: the prior **17 completed** rows plus **15 newly checked** rows produce **32 completed** and **91 pending**. Ownership markers are unchanged.
- WU-0 through WU-2 remain the previously recorded completed baseline. This reconciliation changes only the OpenSpec progress artifacts; it does not change product code.

## Completed implementation units

- **WU-3 — fully implemented, verified, and integrated:** commits `179dbd9`, `ceef961`, and `b3fbd99`. All four WU-3 task rows are checked.
- **WU-4 — fully implemented, verified, and integrated:** commits `785fb24`, `e6e2c47`, and `bad8167`. All five WU-4 task rows are checked. Current-schema merge is in `merge.go`; future value-kind switch/primitives are in `merge_values.go`; the digest is canonical JSON SHA-256.
- **WU-7 — fully implemented, verified, and integrated:** commits `725a8db`, `e475ff9`, and `3c282ad`. All six WU-7 task rows are checked.

## Partial implementation

- **WU-9 — partial only:** `8917566` records checkout observation and `4e02a66` records safety/object reads. All WU-9 task rows remain unchecked because the combined rows still lack dirty-overlap/ff-only advancement, isolated worktrees, tags, and update reporting.

## Verification evidence

The full verification recorded after WU-4 exited `0` for each component:

```bash
go test ./... && go vet ./... && go run ./tools/sync-assets --check
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
```

## Preserved baseline and external drift warning

- WU-0 remains the historical baseline: 47 worktree entries (`10` deleted, `29` modified, `8` untracked), zero stashes, branch `main`, and HEAD `8397ddc7aad00af1a5686418a3250db58b93be43`; the dependency checkouts were clean at their then-current `main` commits.
- Independent verification later observed concurrent external drift in the Gentle Pi checkout: branch `feat/476-codegraph-worker-verify` with nonzero porcelain state at observation time. It was not cleaned, rewritten, or attributed to this work; it must be re-baselined before the later Gentle Pi integration unit.

## Next step

The next catalog dependency is **WU-5** (source-specific pins, host resolution, and initial catalog authoring). **WU-9 remains partial** until its unchecked requirements are completed. Verify/archive and parent-owned lifecycle actions remain deferred.
