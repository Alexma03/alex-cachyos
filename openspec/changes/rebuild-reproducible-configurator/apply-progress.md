# Apply Progress — rebuild-reproducible-configurator

## Cumulative status

- Phase: `apply`; this reconciliation advances the record through partial progress on WU-5, WU-6, WU-8, and WU-10, and records integrated/interrupted/active/frozen states for WU-5b3a/WU-5b3b/WU-5b4, WU-10c1/WU-10c1a, WU-12a/WU-12b, WU-13a, WU-14, and WU-15 through WU-19.
- Total task rows remain **123**. This reconciliation adds 11 checked rows to the previous **37 completed**/**86 pending**, producing **48 completed** and **75 pending**. Ownership markers are unchanged.
- The 11 new checks are partial-unit evidence (individual RED/GREEN/TRIANGULATE rows). No work unit after WU-9 is marked complete, and no global verification is claimed after WU-9.

## Fully completed units (unchanged baseline)

- WU-0 through WU-2 remain the previously recorded completed baseline.
- **WU-3 — fully implemented, verified, and integrated:** commits `179dbd9`, `ceef961`, `b3fbd99`. All four WU-3 task rows are checked.
- **WU-4 — fully implemented, verified, and integrated:** commits `785fb24`, `e6e2c47`, `bad8167`. All five WU-4 task rows are checked. Current-schema merge is in `merge.go`; future value-kind switch/primitives are in `merge_values.go`; the digest is canonical JSON SHA-256.
- **WU-7 — fully implemented, verified, and integrated:** commits `725a8db`, `e475ff9`, `3c282ad`. All six WU-7 task rows are checked.
- **WU-9 — fully implemented, verified, and integrated:** commits `8917566` (checkout observation), `4e02a66` (safety/object reads), `e036a73` (dirty-overlap/ff-only advancement), `e0ea53c` (annotated tags), `926aef9` (owned detached worktrees), and `d4aaef9` (report-only updates). All five WU-9 task rows are checked.

## Partial units: integrated and verified rows (not complete)

| Unit | Checked rows this reconciliation | Commits |
|------|-----------------------------------|---------|
| WU-5 | pins RED + GREEN (2 of 7) | `dd9e2e7`, `a4cace1`, `770eba9`, `56a98ce` |
| WU-6 | planner RED, runner RED, GREEN core, network TRIANGULATE (4 of 6) | `a91fdae`, `45da1e5`, `233d808`, `9cc1e5d` |
| WU-8 | adoption RED, GREEN adopt + planner inverse support, hostname TRIANGULATE (3 of 6) | `618cee2`, `0202c60` |
| WU-10 | lock RED, exec/elevation RED (2 of 6) | `2f967e9`, `f6a987f` |

Each unit above remains **partial**: the remaining rows are unchecked and the unit is not marked complete. The WU-6 composite fixture row and every final-verification row remain unchecked.

## Integrated, active, interrupted, and frozen states

- **WU-5b3a — integrated and verified.** Commit `56a98ce`.
- **WU-5b3b — integrated and verified.** Commit `dbc0f96`; no additional checkbox row is complete.
- **WU-5b4 — digest active.** Work is in progress; no additional checkbox row is complete.
- **WU-10c1 — interrupted.** 413 uncommitted lines of executor work; superseded by the WU-10c1a recovery.
- **WU-10c1a — recovery active.** Fresh recovery in progress.
- **WU-12a — integrated.** Assets commit `eb19289`.
- **WU-12b — interrupted.** 498 uncommitted lines; fresh recovery not yet started.
- **WU-13a — integrated and verified.** Assets commit `aaa065f`; no checkbox row is marked complete.
- **WU-14 — large-asset architecture exploration active.** Exploration driven by the >400 duplicate-asset constraint; no implementation rows checked.
- **WU-15 through WU-19 — frozen.** Not started; no rows checked.

## Verification evidence

The full verification recorded after WU-4 and repeated for WU-9 exited `0` for each component:

```bash
go test ./... && go vet ./... && go run ./tools/sync-assets --check
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh
python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
```

Read-only statuses of both Gentle AI and Gentle Pi were byte-identical before and after the WU-9 verification.

No global verification is claimed after WU-9: the partial units (WU-5, WU-6, WU-8, WU-10) carry per-row RED/GREEN/TRIANGULATE evidence only, and full unit verification remains outstanding.

## Preserved baseline and external drift warning

- WU-0 remains the historical baseline: 47 worktree entries (`10` deleted, `29` modified, `8` untracked), zero stashes, branch `main`, and HEAD `8397ddc7aad00af1a5686418a3250db58b93be43`; the dependency checkouts were clean at their then-current `main` commits.
- Independent verification later observed concurrent external drift in the Gentle Pi checkout: branch `feat/476-codegraph-worker-verify` with nonzero porcelain state at observation time. It was not cleaned, rewritten, or attributed to this work; it must be re-baselined before the later Gentle Pi integration unit.

## Next step

Operational priorities, in order:

1. Complete WU-5b4 digest.
2. Recover WU-10c1a and WU-12b.
3. Resolve WU-14 assets.
4. Run full verification and reconciliation, including the remaining WU-5/WU-6/WU-8/WU-10 rows.

WU-15 through WU-19 remain frozen. Verify/archive and parent-owned lifecycle actions remain deferred.
