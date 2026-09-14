# Cutover Evidence Report

## 1. Scope and Boundary Declaration

This report records the verification status and cutover readiness for the `alex-cachyos` Go configurator.

- **Non-Destructive Boundary:** This change **does not delete any legacy Bash files**. Both `./apply` and the new Go binary `alex-cachyos` coexist in the repository.
- **Dual Verification:** Both Bash scripts and Go packages pass continuous syntax and verification gates.
- **Hardware Gating:** Live mutations (root-level pacman transactions, fingerprint reader enrollment, systemd service reconfiguration) are strictly gated on execution on the physical target hardware (Samsung Galaxy Book `galaxy`). Offline fixtures and dry-run flows are 100% verified in automated tests.

---

## 2. Test Verification Matrix

| Verification Gate | Command | Status | Coverage / Details |
| :--- | :--- | :--- | :--- |
| **Go Test Suite** | `go test ./...` | **PASS** | Full coverage of all Go packages (catalog, planner, receipt, platform, gitx, acceptance). |
| **Static Analysis** | `go vet ./...` | **PASS** | Zero diagnostics or lint defects across codebase. |
| **Embedded Assets** | `go run ./tools/sync-assets --check` | **PASS** | Embedded templates and overlays are in sync with source trees. |
| **Bash Syntax Guard** | `bash -n apply bin/* lib/*.sh modules/*.sh` | **PASS** | All legacy shell scripts valid syntax, no regressions. |
| **Acceptance Goldens** | `go test ./internal/acceptance -v` | **PASS** | Galaxy fixture dry-run, multi-layer inheritance, rollback safety. |

---

## 3. Specification Traceability Matrix

| Specification | Primary Package / Tests | Hardware-Gated Behaviors | Verification Evidence |
| :--- | :--- | :--- | :--- |
| **`acceptance-verification`** | `internal/acceptance` | None (fully mocked / fixture driven) | `TestCatalogFixturesMatchAcceptanceGoldens`, `TestHardwareFreeFixtureConvergenceAndRollbackRefusal` |
| **`cachyos-platform`** | `internal/platform/cachyos` | Live pacman installs, systemd service unit manipulation | Fixture-based unit tests for package set calculations, config template rendering |
| **`catalog`** | `internal/catalog` | None | Strict YAML decode, schema validation, 40-char git commit pins, layer precedence tests |
| **`convergence-commands`** | `internal/app`, `internal/cli` | Live command execution | CLI argument parsing, dry-run flags, plan generation, command dispatching tests |
| **`planner`** | `internal/planner` | None | Topological sort, cycle rejection, idempotency detection tests |
| **`receipts`** | `internal/receipt` | None | Golden schema tests, atomic `current.json` rename, fsync, non-destructive append |
| **`security-boundaries`** | `internal/catalog`, `internal/receipt` | Hardware biometric template storage | Receipts explicitly omit secret values, biometrics, and dirty diffs |
| **`staged-migration`** | Acceptance suite, CI/Validation scripts | Live cutover execution | Dual validation (`bash -n` + `go test`), Bash preserved, side-by-side execution |
| **`worktree-safety`** | `internal/gitx` | None | Dirty worktree rejection, branch protection, non-destructive worktree creation |

---

## 4. Hardware Verification Protocol (Pending Target Machine)

When running on the Samsung Galaxy Book `galaxy`, the cutover procedure will execute:

1. `alex-cachyos check --host galaxy`
   - Verify local host identification and catalog validity.
2. `alex-cachyos apply --host galaxy --dry-run`
   - Verify preflight checks, system lock acquisition, plan resolution, and dry-run receipt emission without mutating files.
3. `alex-cachyos apply --host galaxy`
   - Converge system configuration: packages, devtools (mise), Niri/Noctalia desktop, fingerprint PAM/rules.
4. Verify audit receipt emitted under `$XDG_STATE_HOME/alex-cachyos/receipts/`.
