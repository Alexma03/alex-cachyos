# Apply Progress — rebuild-reproducible-configurator

## Phase scope and result

- Phase: `apply`; this authorized slice finalizes **WU-2 only** (`WU-2a` sync-assets tool and `WU-2b` embedded copies). WU-3 was not started.
- Artifact backend: `openspec`. Strict TDD is inactive (`strict_tdd: false`), although the task plan retains RED/GREEN/TRIANGULATE evidence requirements.
- WU-2 product files were implemented and independently verified in isolated worktrees, then copied byte-for-byte into the authoritative worktree. This session made no product-code correction; it updated only this progress artifact after the required checks passed.
- This WU-2 executor did not issue writes to dependency checkouts, user configuration, the index, commits, branches, delivery artifacts, or unrelated dirty paths. Independent post-apply verification nevertheless observed concurrent external drift in the Gentle Pi checkout (branch `feat/476-codegraph-worker-verify`, nonzero porcelain state at observation time); it was not cleaned, rewritten, or attributed to WU-2 and must be re-baselined before the later Gentle Pi integration unit.

## Structured status consumed

The fresh authoritative status supplied by the parent was consumed:

```json
{"taskProgress":{"total":123,"completed":17,"pending":106,"allComplete":false},"applyState":"ready","dependencies":{"proposal":"all_done","specs":"all_done","design":"all_done","tasks":"all_done","apply":"ready","verify":"blocked","archive":"blocked"},"blockedReasons":[],"actionContext":{"mode":"repo-local","workspaceRoot":"/home/alex/Projects/alex-cachyos","allowedEditRoots":["/home/alex/Projects/alex-cachyos"]},"nextRecommended":"apply"}
```

Action-context warnings: none. The authoritative workspace and the only allowed edit root were `/home/alex/Projects/alex-cachyos`. The workload gate was resolved as `ask-on-risk` with `stacked-to-main`; the parent/user delivery path authorizes this assigned WU-2 slice. The parent lifecycle still owns review, receipts, delivery gates, and branch/PR actions.

## Preserved WU-0 and WU-1 record

- WU-0 remains the historical baseline: 47 worktree entries (`10` deleted, `29` modified, `8` untracked), zero stashes, branch `main`, `HEAD` `8397ddc7aad00af1a5686418a3250db58b93be43`; dependency checkouts were clean at their then-current `main` commits. The current external Gentle Pi drift noted above supersedes only the live dependency-status observation, not the baseline evidence or WU-2 ownership boundary.
- WU-1 remains complete: rows 75–82 are checked; the CLI/state-path RED, GREEN, TRIANGULATE, and REFACTOR evidence is preserved; its final Go tests, vet, Bash/profile guard, and bounded scaffold handoff were recorded previously. No WU-1 product correction was needed in this slice.

## WU-2 evidence

The required combined verification was run once in the authoritative worktree and exited `0`:

```bash
go generate ./internal/assets/... && go test ./... && go vet ./... && go run ./tools/sync-assets --check
```

The separate Bash/profile transition guard was run once and exited `0`:

```bash
bash -n apply bin/alex-cachyos-webapp-launch lib/*.sh modules/*.sh && python3 -c 'import json; from pathlib import Path; [json.load(p.open()) for p in Path("profiles").glob("*.json")]'
```

The WU-2 implementation covers byte-preserving declared-asset synchronization, canonical path/size/SHA-256 manifest generation, drift detection without rewriting in `--check`, symlink/path safety, `go:embed data`, and the `go:generate` wiring. The scoped WU-2 handoff paths are:

- `tools/sync-assets/main.go`
- `tools/sync-assets/main_test.go`
- `internal/assets/assets.go`
- `internal/assets/assets_test.go`
- `internal/assets/data/catalog/.gitkeep`
- `internal/assets/data/source-manifest.json`
- `openspec/config.yaml`
- `openspec/changes/rebuild-reproducible-configurator/tasks.md`
- `openspec/changes/rebuild-reproducible-configurator/apply-progress.md`

The read-only scoped `git diff --stat` emitted no output because these WU-2 paths are untracked in the preserved dirty worktree; no staging or index operation was performed.

`openspec/config.yaml` retains the Bash syntax gate, profile JSON gate, and Go test gate. The asset check appears exactly once in `rules.verify.test_command`, exactly once in `testing.runner`, and once as the dedicated `testing.commands.asset_check` entry; it was not duplicated within either verify/testing chain.

## Completed tasks and persisted checkboxes

- All five WU-2 implementation rows (the RED, GREEN, TRIANGULATE, config-update, and final-verification rows) are visibly persisted as `- [x]` in `tasks.md`; the final evidence above supports those checks. No checkbox transition was needed during this finalization because the copied task artifact already contained the checked rows.
- Ownership audit: 123 checkbox rows total, 119 implementation-owned, 4 parent-owned, and no malformed ownership markers. Persisted counts are **17/123 total**, **16/119 implementation**, and **1/4 parent**.
- The user-selected review split is recorded: **WU-2a tool slice = 400 lines** and **WU-2b embed slice = 89 lines**. No `size:exception` was supplied or used. Current review boundary is WU-2; delivery remains `ask-on-risk`, `stacked-to-main`, with branch/PR construction deferred.

## Remaining work

The next unchecked work starts at WU-3; later unchecked rows are intentionally not duplicated here because `tasks.md` remains authoritative:

- [ ] RED `internal/catalog/validate_test.go`: failing tests for rejection of unsupported `catalogVersion`, unknown top-level fields, unknown module names (registry = the seven Bash module names), references to missing overlay/template assets, and invalid checkout-pin shape; each error names the failure; a purity assertion shows validation performs no filesystem writes or OS port calls. Evidence: failing test output. <!-- sdd-owner: implementation -->
- [ ] GREEN: add `catalog/schema/catalog-v1.schema.json`, `internal/catalog/types.go` (presence-aware decode fields), `internal/catalog/decode.go` (unknown-field rejection, YAML → JSON-compatible data → schema → typed validation), `internal/catalog/validate.go`. Evidence: tests pass. <!-- sdd-owner: implementation -->
- [ ] TRIANGULATE `testdata/catalog/galaxy-minimal.yaml`: a valid minimal catalog passes end-to-end; each invalid fixture under `testdata/catalog/invalid-*.yaml` produces a message naming the offending entry. <!-- sdd-owner: implementation -->
- [ ] Verify and prepare the work-unit diff: `go test ./... && go vet ./... && go run ./tools/sync-assets --check` exit 0 (schema dir now embedded via declared copy); transition guard exits 0; baseline unchanged; record the unit-owned path list and `git diff --stat` as the bounded diff handoff. Staging/committing is deferred: it happens only after a separate explicit user authorization, with explicit unit-owned paths and never `git add -A`. <!-- sdd-owner: implementation -->

## Phase disposition

- No design deviation was introduced. The existing Bash tree remains the migration authority, and WU-3 plus all later implementation work remain pending.
- Verify/archive and all parent-owned lifecycle actions remain deferred. This executor performed no review/refutation/correction/validation actor work, receipt creation or approval, delivery-gate validation, staging, commit, branch, push, PR, install, network, elevation, stash, reset, clean, checkout, or destructive operation.
- After this WU-2 implementation completion, the executor returns `parent-lifecycle`; it does not begin WU-3 or recommend a repeated apply loop.
