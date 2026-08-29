# Apply Progress — rebuild-reproducible-configurator

## Current state

- Completed implementation units: WU-0, WU-1.
- Next unit: WU-2.
- Native checkbox progress: 12/123 total; 11/119 implementation-owned; 1/4 parent-owned.
- Delivery strategy: `ask-on-risk`; chain strategy: `stacked-to-main`.
- Local work-unit commits/worktrees are authorized; push and PR construction are not.
- Dependency checkouts remain observation-only.

## Baseline — WU-0

- Workspace branch/HEAD at capture: `main` / `8397ddc7aad00af1a5686418a3250db58b93be43`.
- Dirty-worktree baseline: 47 porcelain entries (`D:10`, `M:29`, `??:8`), SHA-256 `75c8520996e51ff244d0d1c46165f3f1052b45a0fb732c1fd6d5190eff5fbed1`.
- Stashes: 0.
- Gentle AI checkout: clean at `782e8dfe6b8ac26607e3239eb1e004ca7db1df0b`.
- Gentle Pi checkout: clean at `a3d87c196268774c8989169e45634e9b46066881`.
- Transition guard: exit 0.
- Start-state guard (`go.mod` and `internal/` absent): exit 0 before WU-1.

## WU-1 — Go scaffold, CLI surface, and XDG paths

### TDD evidence

- RED: CLI package and state-path tests initially failed before their packages existed.
- GREEN: minimal CLI parsing, entrypoint, XDG path resolution, and tests were added.
- TRIANGULATE: module-list, exit-code, override, ownership, permissions, and no-`/tmp` cases were added.
- REFACTOR: exit codes were extracted to `internal/cli/exit.go`; tests remained green.

### Verification

- `go test ./... && go vet ./...`: exit 0.
- Bash/profile transition guard: exit 0.
- Product paths: `go.mod`, `cmd/alex-cachyos/main.go`, `internal/cli/**`, `internal/statepath/**`.
- OpenSpec paths: `openspec/config.yaml`, `tasks.md`, this progress file.
- Product authored size: 323 lines including blanks (290 non-blank), within the 400-line unit budget.
- WU-2 paths are absent and all WU-2 checkboxes remain unchecked in this integration snapshot.

## Safety and delivery

- No pre-existing dirty path was copied into the integration branch.
- No dependency checkout or real user configuration was modified.
- No stash, clean, reset, force operation, elevation, package installation, push, or PR occurred.
- The SDD proposal/spec/design snapshot is a local integration-base commit and is not itself a delivery PR.
