# Proposal — rebuild-reproducible-configurator

> Rebuild `alex-cachyos` as a reproducible Go CLI that converges CachyOS+niri+Noctalia **and** the complete non-default Pi/Gentle AI development environment from a single declarative, version-pinned catalog — staged alongside the existing Bash implementation.

- Change: `rebuild-reproducible-configurator`
- Status: proposal (decision review — corrected gatekeeper rerun)
- Artifact store: `openspec`
- Inputs: `explore.md` (corrected gatekeeper rerun, complete) · `research.md` rev 1 (S1–S18, done) · `preproposal.md` rev 2 (product decisions confirmed, `proposal_ready: true`) · `openspec/config.yaml`
- Author: proposal executor (no product-code implementation; confirmed handoff honored — no product decisions reopened)

## Quick path

1. Read **§1 Intent** → **§2 Goals** → **§3 Scope and non-goals** (2 min) for problem, outcome, and boundary.
2. Review **§4 Desired-state catalog** through **§12 Temporary sdd-research override** for the separated product decisions that require approval (catalog, planner, platform, Pi reproduction, checkouts, packages, AGENTS.md split, model routes/RDD, and the Pi-specific retirement predicate).
3. Confirm **§13 Security cross-cutting**, **§16 Risks**, **§17 Rollback** (non-mutating), and **§18 Success criteria** (fixture-based, no galaxy-on-generic-VM).
4. On approval, next phase is `spec.md` (RFC 2119, Given/When/Then) → `design.md` → `tasks.md` — the Go CLI is built and validated **alongside** the preserved Bash implementation (§3.3, §14).

## 1. Intent and problem statement

**Current state is not reproducible.** `alex-cachyos` today is a Bash CLI (`./apply`) with JSON profiles parsed via `python3`+`eval`, ordered shell modules (`bootstrap → fingerprint → devtools → apps → vicinae → desktop → verify`), file-based templates/overlays, and manual package-list handling. It converges one CachyOS host (`galaxy`) correctly but cannot reproduce the user's Pi/Gentle AI environment on a fresh machine without manual steps.

**The Pi/Gentle AI environment is non-default and machine-specific.** Settings live in `~/.pi/agent/settings.json` (minimal), `~/.pi/agent/subagents.json` (owns `lean`/`task`/`model_profiles` with 23 routes), `~/.pi/gentle-ai/models.json` (same 23 routes under `thinking`), global `AGENTS.md`, temporary `subagents/sdd-research.md` (Pi web-enabled, see §12), background/persona JSON, local `~/Projects/gentle-ai` and `~/Projects/gentle-pi` `main`-branch checkouts, and a pinned npm package set (including `../../Projects/gentle-pi` as a local path package). The package manager's defaults would lose the user's choices (e.g. RDD globally enabled, `lean`/`task`, effort/thinking levels). Credentials and biometric state must never be cataloged.

**Why now and what delivery channel.** Every new or rebuilt CachyOS machine pays manual reconstruction cost, risks drift between `subagents.json`/`models.json`, risks dirty-worktree loss on checkout updates, and silently diverges if `APPEND_SYSTEM.md` ownership is mishandled or RDD state is defaulted off. The selected Go rebuild makes the whole machine — OS desktop and agent harness — reproducible, auditable, and rollback-safe. The desired authority is the **development channel**: target machines consume the `main` branches of the local `gentle-ai` and `gentle-pi` checkouts (§8). Stable/RC releases are evidence and baselines only, not the desired state (§8, §12, §15). The current `gentle-pi` `main` postinstall still pins a package-local Gentle AI `v2.4.0` binary and rejects PATH/global fallback (S6/S7) — this is a real integration gap; cloning `main` does **not** automatically make the package-local runtime consume `main` and requires a tested local-main build/install path (§8, §16).

## 2. Goals (product outcomes)

- One `alex-cachyos` Go binary (embedded assets via `go:embed`) converges a fresh CachyOS+niri+Noctalia machine **and** the full Pi/Gentle AI environment from a pinned catalog, tracking `main` for the harness checkouts.
- `apply` is idempotent, offline-safe for file/config steps, and records an immutable per-run receipt under `$XDG_STATE_HOME/alex-cachyos/`.
- Host identity resolves via hostname + `--host` override (no DMI/sensitive identifiers as catalog keys).
- Dirty worktrees in both the configurator repo and the adopted `gentle-ai`/`gentle-pi` clones are never destroyed.
- Secrets/biometrics are never stored; web-research credentials are secret-free references, auth remains interactive.
- Migration is staged and reviewable: the existing Bash `./apply` remains authoritative until the Go CLI is validated (§3.3, §14, §18).

## 3. Scope and non-goals

### 3.1 In scope (first slice)

| Area | Included |
|------|----------|
| **Catalog & inheritance** | Typed, validated desired-state catalog; precedence `global → role → host`; `galaxy` as sole concrete host profile initially; host detection replaces hardcoded `AO_PROFILE=galaxy` |
| **Planner & execution** | Shared planner with dependency-ordered steps (bootstrap first, verify last, apps before desktop); `apply`, `check` (alias `--only verify`), `adopt`, `rollback`, `checkpoint` (annotated Git tags), `receipt` behavior — validated via planner/fake-runner fixture before hardware apply (§18) |
| **CachyOS platform** | CachyOS Linux + niri (Wayland) + Noctalia shell/greeter only; `pkexec` elevation, flock lock, `*.bak.alex-cachyos` adoption, explicit pacman explicit-marking, atomic GRUB, legacy Cosmic prune, Plymouth strip, all existing Bash module content mapped into typed catalog/planner |
| **Pi/Gentle AI reproduction (development channel)** | Exact-pinned npm packages (§4, §9); local `gentle-pi` path package; adopted `gentle-ai`/`gentle-pi` `main` checkouts with a tested, integrity-preserving local-main runtime path (§8); 23 model routes (single source → two file shapes); `lean`/`task` flags; `AGENTS.md` user-owned vs `APPEND_SYSTEM.md` package-managed (§10); temporary Pi web-enabled `sdd-research` override with Pi-specific retirement predicate (§12); package-managed agents/chains/support/manifests as install outputs verified against manifest; RDD global enable+verify; secret-free web-research template; interactive OAuth retained |
| **Safety & receipts** | One immutable JSON receipt per run + `current` index under `$XDG_STATE_HOME/alex-cachyos/`; annotated `catalog-v*` checkpoint tags; rollback via inverse plan + Snapper for system-level; idempotent pacman/AUR/build checks; offline grouping |

### 3.2 Non-goals (explicitly out)

- Multi-distro or multi-DE support (no Hyprland/GNOME/KDE/Cosmic; Noctalia+niri is the boundary).
- Managing secrets, biometric templates (`/var/lib/fprint`), keyring, Tailscale/NordVPN session state, or Pi `auth.json` / cache / sessions / `.pi/npm` node_modules.
- Vendoring package-managed Gentle AI/Gentle Pi assets as primary install path.
- XDG-migration of Pi's own `~/.pi` state (Pi intentionally uses `~/.pi/agent/{npm,git}/`; only the configurator's own receipts/state follow XDG).
- Automatic migration of the existing dirty worktree's uncommitted user work (must be preserved, never cleaned/stashed/committed by the tool).
- Treating any stable or RC release (including `gentle-ai v2.5.0-rc.1`) as the desired harness authority — they are evidence/baselines only (§8, §12).
- Retiring the Pi web-enabled `sdd-research` override on generic Gentle AI `sdd-research` stable alone — retirement requires the Pi-specific predicate in §12.

### 3.3 Staged migration (reviewability)

The current dirty Bash implementation (`./apply` + `lib/` + `modules/` + `templates/` + `overlays/` + `packaging/`) is **preserved intact** while the Go CLI is built and validated. The Go binary (`cmd/alex-cachyos`) is developed alongside, with `go:embed` assets, planner/fake-runner tests (§18), and `bash -n` retained during transition. No big-bang replacement: cutover occurs only after planner fixture, `check`/`--dry-run`, and receipt/manifest verification pass on the explicit integration target, with Bash remaining available for rollback via `*.bak.alex-cachyos` and Snapper (§17).

## 4. Desired-state catalog and profile inheritance (separated)

**Decision:** Typed, validated catalog replaces `profiles/*.json` + `python3`/`eval` + grep/awk package-list parsing [research S1–S2, explore §2]. Embedded via `go:embed` so runtime needs no repo-relative paths.

| Aspect | Decision |
|--------|----------|
| **Schema** | Versioned catalog (e.g. `catalogVersion: 1`) with JSON Schema + Go struct validation; rejects unknown modules, validates package lists, overlay/template references, and checkout pins |
| **Precedence** | `global → role → host` merge; host resolved from `hostname` with `--host` explicit override; DMI/`/etc/machine-info` not used as catalog keys (privacy, per pre-proposal) |
| **Initial population** | Single concrete host `galaxy`; global holds shared defaults; role layer empty but reserved |
| **Pinning — exact versions (reproducible)** | Catalog pins **exact** versions only (e.g. `npm:pi-web-access@0.27.0`, `npm:pi-commandcode-provider@1.2.3`, `npm:pi-mcp-adapter@2.6.0`). No caret (`^`) or range examples in the desired state. PKGBUILD `libfprint-egismoc-sdcp-git` retains pinned `_commit` + patch sha256. `gentle-pi` `2.2.0` and Gentle AI `2.4.0` are **build evidence/baselines** for the installer (§15); the desired harness authority is the `main` checkouts (§8). Catalog-exact pins are skipped by `pi update --all` (S2); **transient resolved versions** observed via `pi list`/`~/.pi/agent/npm` are recorded in receipts but do not define the desired state — the distinction is enforced in spec and receipt schema |
| **Templates/overlays** | Typed rendering with `@HOME@`/`@USER@` token handling (replaces `sed` substitution); validation before write |
| **Checkpoint tags** | Annotated tags `catalog-v*` per accepted catalog version (S15 lightweight vs annotated); `git describe`-visible; moving a tag requires `+`/`--force` (S16) |

## 5. Planner / apply / check / adopt / rollback / checkpoint / receipt behavior (separated)

| Capability | Behavior |
|-----------|----------|
| **Planner** | Dependency graph (bootstrap → fingerprint/devtools/apps/vicinae → desktop → verify); `--only m1,m2` wins, then `--with`/`--without`, then profile defaults (ported from `ao_should_run_module`); ordered steps with explicit `dependsOn`; exercised via fake-runner fixture (§18) without requiring `galaxy` hardware |
| **`apply`** | Idempotent convergence: pacman `-Q` pre-checks, `--needed`, fingerprint skip-rebuild when pinned commit+pkgrel already installed; `flock` single-instance lock at `$XDG_RUNTIME_DIR/alex-cachyos-*.lock` (exit 75); `pkexec` elevation only for system steps; `--dry-run` groups network-required steps upfront; Bash `apply` remains available during staged migration (§3.3) |
| **`check`** | Non-mutating, offline-safe drift detection: `niri validate`, `noctalia config validate`, byte-compare to embedded templates, declared-package presence, greetd/boot/PAM compare, failed units, `pacman -Dk/-Qk`, `.pacnew` warning; reports drifted files with backup path; never reads biometric data (presence check only) |
| **`adopt`** | Detect-and-adopt existing unmanaged files via `*.bak.alex-cachyos` one-time backup manifest; hostname-based profile suggestion; adopt existing `gentle-ai`/`gentle-pi` clones instead of re-cloning |
| **`rollback`** | Per-module `--remove` restoring `*.bak.alex-cachyos` backups; receipts allow inverse-plan execution; system-level rollback remains Snapper-backed (desktop module note — not reimplemented); **never mutates the active dirty worktree** — see §17 |
| **`checkpoint`** | `checkpoint create --tag catalog-vX.Y.Z -m "..."` creates annotated Git tag on catalog snapshot; receipts record `catalogTag` applied by each machine |
| **`receipt`** | One immutable JSON file per run: `~/.local/state/alex-cachyos/receipts/<timestamp>-<shortHash>.json` + atomic `current` index (symlink or `current.json` pointer); records steps, file hashes, versions, credential **names** only, resolved checkout commits, catalog tag, host; never secret values; XDG-compliant (S13 `XDG_STATE_HOME`); distinguishes `desiredExactPins` vs `resolvedInstalledVersions` (§4) |

CLI surface preserved: `--profile` → `--host`, `--only`, `--with`/`--without`, `--remove`, `--dry-run`, `--check`, `--list`, `-h/--help`.

## 6. CachyOS + niri + Noctalia initial scope (separated)

- **Platform:** CachyOS Linux, niri compositor (Wayland), Noctalia shell + `noctalia-greeter` + `greetd` + `AccountsService`/`.dmrc`. No X11 tooling.
- **Elevation:** `pkexec` with fingerprint polkit agent (`pam.d` `sufficient` for `sudo`/`polkit-1`); never `sudo`.
- **Modules mapped 1:1 from Bash:** `bootstrap` (sysup, want/remove lists, GRUB atomic, plymouth, ananicy-cpp, UFW, Chrome via paru, zsh marker blocks, LTS deferral), `fingerprint` (PKGBUILD), `devtools` (mise + pnpm/npm configs), `apps` (pacman+AUR, docker/tailscale/nordvpn, webapps `name|url|icon_url` with favicon fallback), `vicinae` (`vicinae-bin` + user service), `desktop` (niri/Noctalia/hyprwhspr/quickshell-polkit/greetd/Cosmic prune/AccountsService/polkit `qs -c polkit`/Noctalia plugins), `verify` (convergence checks inventory).
- **Out of scope:** any other compositor/DE, X11 helpers (`xdotool`/`xrandr`), non-CachyOS package managers.

## 7. Pi/Gentle AI configuration — user-owned vs package-managed (separated)

| Ownership | Files / behavior |
|-----------|-----------------|
| **User-owned (cataloged)** | `~/.pi/agent/settings.json` minimal shape (model/provider/thinking/changelog/packages/theme — no `session_resources`/`model_profiles`), `~/.pi/agent/subagents.json` (`lean`/`task`/`enable_continue`/`debug` + 23 `model_profiles` with `effort`), `~/.pi/gentle-ai/models.json` (23 routes with `thinking`), `~/.pi/gentle-ai/persona.json`, `~/.pi/gentle-ai/background-subagents.json`, `~/.pi/agent/AGENTS.md` (global environment guidance, see §10), `~/.pi/web-search.json` **template** (secret-free refs only), temporary `~/.pi/agent/subagents/sdd-research.md` (Pi web-enabled, §12) |
| **Package-managed (generated, never vendored)** | `~/.pi/agent/gentle-ai/managed-assets.json` (SHA-256 manifest), `~/.pi/agent/gentle-ai/support/*`, `~/.pi/agent/chains/*`, `~/.pi/agent/agents/*`, package-local Gentle AI binary under `~/.pi/agent/npm` or gentle-pi bundle; installed/refreshed by the selected local-`main` gentle-pi checkout's `postinstall` (`scripts/install-gentle-ai.mjs`, currently `INSTALLER_VERSION=2.4.0` as baseline) + `gentle-ai sync` **or** the tested local-main build/install path that preserves integrity while consuming `main` (§8); verified against manifest after install (S5–S7, S10, S12, S15–S16) |
| **Explicitly not cataloged** | `auth.json`, `mcp.json`/`mcp-cache.json`/`models-store.json`, `sessions/**`, `pi-pretty/**`, `web-search-cache/**`, `~/.pi/npm/**` node_modules, `.engram/chunks/**`, `.atl/`, `.codegraph/`, `/var/lib/fprint`, keyring, `.npmrc` auth lines |

The configurator **regenerates** package-managed assets from the selected checkout; it **never** copies them verbatim from a captured machine (S6/S12/S16).

**Preserved split:** `AGENTS.md` is user-owned; `APPEND_SYSTEM.md` is package-managed — see §10.

## 8. Local main-branch source checkouts with dirty-worktree safety and commit receipts (separated)

**Development channel is the desired state.** Target machines consume the `main` branches of the local `gentle-ai` and `gentle-pi` checkouts (the `~/Projects/gentle-ai` and `~/Projects/gentle-pi` clones), not a stable or RC release. Stable/RC tags (`gentle-ai v2.4.0`, `v2.5.0-rc.1`, `gentle-pi 2.2.0`) are evidence/baselines for the current installer behavior, not the desired authority.

- **Sources:** `~/Projects/gentle-ai` (`github.com/gentleman-programming/gentle-ai/v2`, `origin/main`) and `~/Projects/gentle-pi` (`gentle-pi` `main`, postinstall currently installs package-local Gentle AI `2.4.0` as baseline). The catalog tracks a pin or `main` tip for each checkout; receipts record the exact resolved commit applied.
- **Reproduction:** clone if missing, otherwise **adopt** existing checkout; `git fetch` + plain `git checkout <pinned main commit or branch tip>` — which **refuses** on dirty overlap (S14) — never `--force`/`--force checkout` which would discard work (S14). If dirty and checkout would clobber, abort with actionable message, leave worktree intact, and record `dirty:true` in receipt.
- **No Pi git-package reconcile for these clones.** Pi's `git:` package reconciliation does `reset --hard` + `clean` then `npm install` (S2 C4) — destructive to dirty worktrees and violates `apply.preserve_dirty_worktree: true`. Manage these clones directly via git, not via `pi install git:...`.
- **Integration gap — package-local runtime vs `main`:** The current `gentle-pi` `main` postinstall still provisions the package-local Gentle AI `v2.4.0` binary and the runtime **rejects** PATH/global/sibling/symlink fallbacks (S6/S7). Cloning `main` therefore does **not** automatically make the package-local runtime consume `main`. This proposal treats that as a real gap requiring a **tested, integrity-preserving local-main build/install path** (e.g. a source-build or installer flag that builds/installs the local `gentle-ai` `main` into the package-local location with the same signature/manifest verification the installer enforces, plus `gentle-ai sync` and `managed-assets.json` verification). The design must specify, implement, and test that path; it must not claim the gap is closed by checkout alone. `GENTLE_PI_SKIP_GENTLE_AI_INSTALL=1` is honored for dev/offline semantics, but then native review fails closed with `package-local-binary-missing` per S6/S7 — not a valid converge.
- **Post-checkout:** `pnpm install` for `gentle-pi` (postinstall intact or via the local-main path above); verify package-local binary version/manifest after install.
- **Receipts record:** `git rev-parse HEAD`, `git status --porcelain` summary (counts, not diff content), whether existing clone was adopted vs freshly cloned, and the package-local binary version/manifest hash.

## 9. Pinned Pi package installation (separated)

- **Source of truth:** Explicit pinned npm specs with **exact** versions in the catalog (e.g. `npm:pi-commandcode-provider@1.2.3`, `npm:pi-mcp-adapter@2.6.0`, `npm:pi-web-access@0.27.0`). No `^`/`~`/`>=`/range examples in the desired state. Local path `../../Projects/gentle-pi` resolved against settings file location (S2 C1); `npmCommand` pinned if used.
- **Exact vs resolved:** The catalog's exact pins are the desired state; the transient **resolved** versions observed in `~/.pi/agent/npm` / `pi list` are recorded in receipts as `resolvedInstalledVersions` for audit but do not redefine the desired pin. Drift between desired exact and resolved is reported by `check`.
- **Scope:** user-global (`~/.pi/agent/settings.json`) by default; `-l` project scope only when a project explicitly needs it (S2 C3).
- **Behavior:** Exact-pinned npm specs are skipped by `pi update --extensions`/`--all` (S2) — gives reproducible sets; Pi auto-installs missing project packages after trust (S2); `pi update --all` is not the primary convergence path — the configurator drives installs from catalog.
- **Receipts:** recorded desired exact pins and resolved versions (from `~/.pi/agent/npm`); package identity per S2 C5.

## 10. Global AGENTS.md ownership and package-managed APPEND_SYSTEM.md (separated)

**Decision (per pre-proposal):** version user-owned global environment instructions as `AGENTS.md` → `~/.pi/agent/AGENTS.md`; leave `APPEND_SYSTEM.md` fully package-managed by Gentle Pi. Preserved unchanged.

- `AGENTS.md` (Pi docs S17): global instructions loaded at startup from `~/.pi/agent/AGENTS.md` plus cwd-parent walk. This is the user-owned overlay for pkexec/fingerprint/CachyOS+niri+Noctalia context.
- `APPEND_SYSTEM.md` (S17, S6): **append** (not replace) semantics for `SYSTEM.md`; Gentle Pi writes runtime contract text (`gentle-ai:agent-routing` implementation-routing block, RDD switch docs) into it. The configurator must not treat the combined file as an opaque blob to copy — it leaves it package-managed and lets the installed checkout regenerate it (via postinstall or the local-main path in §8 plus `gentle-ai sync`). User environment guidance lives in `AGENTS.md`, not by appending to a vendored `APPEND_SYSTEM.md`.
- `SYSTEM.md` replacement not used.

## 11. Model routes, lean/task behavior, RDD enabled state, secret-free web research, interactive auth (separated)

### 11.1 23 model routes (single source → two file shapes)

- Same 23 profile names → provider/model → level appear in two files with different key spellings: `subagents.json: {model, effort}` vs `gentle-ai/models.json: {model, thinking}` (explore §3.4, pre-proposal confirmed).
- Catalog models this **once** (e.g. `modelProfiles: [{name, model, level}]`) and **renders both files** from that source; `check` diff-manages them so they cannot drift. Covers `gentle-ai-{explore,worker,verify}`, `sdd-{init,onboard,explore,research,proposal,spec,design,tasks,apply,verify,status,sync,archive}`, `review-{risk,reliability,resilience,readability}`, `jd-{judge-a,judge-b,fix-agent}`.

### 11.2 Lean / task / debug

- Non-default Pi settings `session_resources: lean`, `default_mode: task`, `enable_continue: false`, `debug: false` live in `~/.pi/agent/subagents.json` (explore §3.5, S1) and are explicitly set — not relying on Pi defaults.

### 11.3 RDD enabled state

- Package default is opt-in **off** since `gentle-ai v2.4.0` (S10), but this user's desired state is globally **enabled** (explore §3.6, `gentle-ai review mode status` → `on`, decided by `global`). Reproduction **explicitly runs** `gentle-ai review mode enable --scope global` (S11: only command that turns it on) and verifies with `gentle-ai review mode status --cwd <repo>` (reports `global`/`clone`/`deciding`/`effective` without mutation, S11). While disabled, ordinary repository policy decides delivery and receipts report `disabled/unmanaged` truthfully (S11 C24).

### 11.4 Secret-free web research

- `~/.pi/web-search.json` emitted from a secret-free template using **env-var** (`$NAME`/`${NAME}`) or **trusted-command** (`!command`) reference forms (S4 C17); interpolation applies to provider credentials only. Literal keys never versioned. Env equivalents (`OPENAI_API_KEY`, `EXA_API_KEY`, etc.) retain precedence (S4 C18). Receipts record credential **names** only.

### 11.5 Interactive auth (per-machine, never cataloged)

- Codex auth reuse (`/login`), Kimi (`/login kimi-coding`), browser-cookie opt-in (`authFetch`/`browserCookies`, S4 C20), and any future provider OAuth remain interactive runtime steps after `apply`. The configurator detects "authenticated but not cataloged" without reading `auth.json` (e.g. via `gentle-ai`/`pi` status probes, not file reads).

## 12. Temporary Pi web-enabled sdd-research override and Pi-specific retirement predicate (separated)

**Why this override exists.** Pi's packaged agent currently lacks the exact `pi-web-access` grants needed for source-backed research. This temporary override exists **specifically** because that Pi capability is missing — not merely because Gentle AI's generic `sdd-research` phase exists. A generic Gentle AI stable that ships `sdd-research` alone is insufficient to retire it.

- **Install now:** `~/.pi/agent/subagents/sdd-research.md` with front-matter `name: sdd-research`, `model: commandcode/deepseek/deepseek-v4-flash`, `thinking: max`, allowlist `read, grep, find, edit, write, web_search, source_check, fetch_content, get_search_content, mem_search, mem_get_observation, mem_save`, body = SDD research executor contract (`gentle-ai.sdd-research/v1`, `gentle-ai.sdd-preproposal/v1`, admission `documentation=[fetch_content]` / `open-web=[web_search,source_check,fetch_content,get_search_content]`). This matches the override observed in explore §3.8 and the official Gentle AI spec in S8. The allowlist's web tools (`web_search`, `source_check`, `fetch_content`, `get_search_content`) are the Pi-specific capability the override provides.

- **Retirement predicate — Pi-specific, not generic stable:** Remove the override automatically **only when all of the following hold**, verified at `apply --update` time:

  1. **Official Pi capability admission exists** — the accepted upstream work that introduces Pi web grants is complete: `Gentleman-Programming/gentle-ai#3846` (Pi capability admission) **and** `Gentleman-Programming/gentle-pi#471` (Gentle Pi packaged `sdd-research` agent) are closed/completed and the consumed `gentle-pi` `main` checkout contains the packaged agent.
  2. **Consumed packaged agent exposes exact web tools and non-empty grants** — the `sdd-research` agent shipped by the **consumed local-main** `gentle-pi` checkout (not an arbitrary stable) lists the exact web tools (`web_search`, `source_check`, `fetch_content`, `get_search_content` or their Pi equivalents) and declares **non-empty grants** for `documentation` and `open-web` matching the override. Verified via the agent's front-matter/file and `~/.pi/agent/gentle-ai/managed-assets.json` manifest plus a runtime smoke check (e.g. `pi`/agent manifest reports the grant, or a dry-run `sdd-research` invocation shows admission with those tools).
  3. **Manifest and smoke verification** — after removing the override, `gentle-ai sync` succeeds and `managed-assets.json` integrity is verified; a smoke probe confirms the packaged `sdd-research` agent is discoverable without the override file.

  Encode as `retire_when: "pi-sdd-research-web-capable"` in catalog (predicate on the two issues plus manifest/smoke, not on `gentle-ai >= v2.5`). A generic Gentle AI `v2.5` stable that ships `sdd-research` without the Pi web grants **does not satisfy** the predicate.

- **Behavior:** `check` reports override staleness vs the predicate; `apply --update` evaluates the predicate; on retirement, delete `subagents/sdd-research.md`, run `gentle-ai sync`, verify `managed-assets.json` integrity (S8/S15) and smoke-probe the packaged agent.

## 13. Security, secrets, biometrics, rollback, offline, idempotency, non-goals — cross-cutting

| Concern | Requirement |
|---------|-------------|
| **Secrets** | Never read `~/.pi/agent/auth.json`, `mcp.json`, `web-search.json` live values, keyring, or cache; never embed credential values in catalog, templates, receipts, or tags; use `$ENV`/`!command` refs (S4) |
| **Biometrics** | Never collect, store, or log fingerprint templates; `verify`/`check` only runs `fprintd-list` presence (warn-only) per existing module; receipts/catalogs contain no `/var/lib/fprint` content |
| **Rollback** | `*.bak.alex-cachyos` backups + receipt inverse plan for file/config converges; Snapper for system-level; annotated tag to re-apply prior `catalog-v*` **without mutating the active dirty worktree** (§17) |
| **Offline** | `--dry-run`, `check`, file/template rendering work offline; pacman/AUR/build steps require network and are grouped/reported upfront |
| **Idempotency** | Every step guards with target-state check (`pacman -Q`, version compare, file hash compare, service active check); re-running `apply` with no catalog change is no-op (receipt still emitted marking `noChange:true`) |
| **Staged migration** | Bash `apply` preserved during Go build; Go CLI validated via fixture before hardware cutover (§3.3, §18) |
| **Non-goals restated** | No secret/biometric capture; no XDG migration of Pi state; no dirty-worktree destruction; no retirement before Pi-specific predicate; no multi-DE |

## 14. Affected areas

- **Staged Go module (new, alongside Bash):** `go.mod`, `cmd/alex-cachyos`, `internal/catalog`, `internal/planner`, `internal/apply`, `internal/check`, `internal/receipt`, `internal/gitx`, `internal/pi`. **Bash implementation is not deleted in the first slice** — `apply`, `lib/`, `modules/`, `templates/`, `overlays/galaxy/`, `packaging/libfprint-egismoc-sdcp-git/` remain authoritative until Go verification passes (§3.3, §18). Embedded assets (`templates/**`, `overlays/galaxy/**`, `packaging/**`) are copied into `assets/` or embedded via `go:embed` during the transition; the Go binary reads embedded copies, Bash continues to read the originals.
- **Catalog:** `catalog/` (or `config/catalog.yaml` + `config/hosts/galaxy.yaml`) with schema + `catalog-v*` annotated tags.
- **State:** `$XDG_STATE_HOME/alex-cachyos/receipts/` + `current` index; lock in `$XDG_RUNTIME_DIR`; optional cache in `$XDG_CACHE_HOME`.
- **Pi state touched:** `~/.pi/agent/settings.json`, `~/.pi/agent/subagents.json`, `~/.pi/gentle-ai/models.json`, `~/.pi/agent/AGENTS.md`, `~/.pi/agent/subagents/sdd-research.md` (temporary, Pi web-enabled), `~/.pi/web-search.json` (template), `~/Projects/gentle-{ai,pi}` clones, `~/.pi/agent/npm` via `pi install`, plus the local-main build/install path outputs (§8).
- **Untouched:** `~/.pi/agent/auth.json`, caches, sessions, `~/.pi/npm/node_modules`, biometric store; plus the active dirty `alex-cachyos` worktree (§17).
- **Docs:** `docs/` updated to English; `openspec/config.yaml` updated with `go test ./...` runner when tests land (retaining `bash -n` during staged migration).

## 15. Dependencies

- CachyOS/Arch tooling: `pacman`, `paru`, `systemd`, `grub-mkconfig`, `mkinitcpio`, `snapper` (for system rollback).
- Pi (`earendil-works/pi`) — `pi install`/`pi update`/`pi list`/`pi config` (S1–S2); Gentle Pi `2.2.0` / Gentle AI `2.4.0` stable and `v2.5.0-rc.1` prerelease are **evidence/baselines** for current installer behavior (§8); `v2.5.0-rc.1` alone is not a retirement signal for the Pi web-enabled override (§12). The desired channel is `main` (§8).
- Git (fetch/checkout/tag semantics S14–S16); XDG Base Directory Spec 0.8 (S13).
- `pkexec` + `polkit` + `fprintd`; `mise`, `pnpm`, `niri`, `noctalia`, `vicinae`, `hyprwhspr`, `quickshell-polkit`.
- Upstream issues for the Pi web-enabled predicate: `Gentleman-Programming/gentle-ai#3846` and `Gentleman-Programming/gentle-pi#471` (§12).

## 16. Risks and mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Dirty-worktree loss (configurator or gentle clones) | High — repo is heavily dirty today (explore §1.1) | High — user work lost | Plain `git checkout` (refuses on overlap), never `--force`; pre-check `git status --porcelain`; abort with message; `apply.preserve_dirty_worktree: true` enforced; receipts log dirty state; rollback never mutates active worktree (§17) |
| Local-main runtime gap (clone `main` but package-local binary stays `2.4.0`) | High — current `main` postinstall behavior (S6/S7) | High — `main` never actually runs | Treat as first-class gap in §8: design, implement, and test an integrity-preserving local-main build/install path with manifest + smoke verification; verify `managed-assets.json` and package-local binary version after install; never claim checkout alone is sufficient |
| APPEND_SYSTEM.md clobber / contract freeze | Medium | High — RDD/routing breaks | Leave file package-managed; user content in `AGENTS.md`; never copy blob; verify after `gentle-ai sync` |
| Pi `~/.pi/npm` vs `~/.pi/agent/npm` path divergence (contradiction S1/S2 vs local observation) | Medium | Medium — install to wrong dir | Catalog records intended dir; `check` probes both; verify against installed Pi version at runtime (research Contradictions) |
| Secret leakage via `web-search.json` or receipts | Medium | Critical | Template uses `$ENV`/`!command` only; receipts store names not values; pre-commit check rejects literal keys; never read `auth.json` |
| RDD silently stays off | Medium | Medium — lose user's enabled choice | Explicit `enable --scope global` + `status` verify; `check` asserts `effective:on` / `deciding:global` |
| Retirement too early (generic stable mistaken for Pi web capability) | Medium | Medium — lose override, research loses web tools | Predicate requires Pi-specific issues #3846 + #471, exact web tools, non-empty grants, manifest + smoke (S8/S15); generic `v2.5` stable alone insufficient (§12) |
| `subagents.json` ↔ `models.json` drift (effort vs thinking) | High | Medium — routing divergence | Single source, dual render, diff-managed; `check` fails on drift |
| Offline / partial network during AUR/webapp icon fetch | Medium | Low | Group network steps; degrade gracefully (favicon fallback); `--dry-run` reports upfront |
| Biometric data accidentally persisted | Low | Critical | Ban `/var/lib/fprint` from catalog/receipts; presence-only check; review gate for receipt schema |
| Big-bang replacement breaks reviewability | Medium | High — unreviewable diff, lost Bash fallback | Staged migration (§3.3, §14): Bash preserved, Go validated via fixture before cutover |

## 17. Rollback plan (design-safe, preserves dirty worktrees)

**Invariant:** No rollback step mutates the active dirty `alex-cachyos` worktree. All catalog reads for prior versions are non-mutating; isolated worktrees are used when a prior version must be applied. Pre-existing user work is never cleaned, stashed, or committed.

1. **File/config rollback:** `alex-cachyos rollback --receipt <id>` or `rollback --tag catalog-vX` replays the inverse plan restoring `*.bak.alex-cachyos` backups recorded in the receipt; `check` confirms convergence. The prior catalog/catalog-tag content is read via `git show <tag>:<path>` (or `git cat-file`) — **never** `git checkout <tag>` in the active worktree.
2. **Catalog version rollback:** To re-apply a prior snapshot, the tool either (a) reads the tagged catalog snapshot with `git show catalog-v<prev>:catalog/...` and applies it from memory, or (b) materializes it in an **isolated worktree** (`git worktree add --detach /tmp/alex-cachyos-rollback-<tag> catalog-v<prev>`) and runs `apply` from there. The active dirty worktree at `~/Projects/alex-cachyos` is untouched. A new receipt records the rollback and the tag it re-applied.
3. **System-level rollback:** `snapper` snapshot (desktop module) — `alex-cachyos` does not reimplement system rollback, only file/config inverse.
4. **Package rollback:** `pacman -U` prior package from cache or reinstall exact-pinned version; `pi install` pins revert via catalog exact pins (§9).
5. **Dirty-worktree guarantee:** Rollback never runs `git checkout <tag>`, `git reset --hard`, or `git clean` on any dirty tree (configurator or adopted clones); if a step would discard local modifications, it aborts with an actionable message.

## 18. Success criteria (measurable — fixture-based, no hardware assumption)

| # | Criterion | How verified |
|---|-----------|-------------|
| 1 | **Planner convergence is validated without requiring `galaxy` hardware:** `go test ./...` planner/fake-runner fixture proves `alex-cachyos apply --host galaxy --dry-run` (and the planner's dependency order bootstrap→…→verify, `global→role→host` precedence, exact-pin resolution) converges to the expected plan and that a second apply with no catalog change is no-op (`noChange:true` receipt). Explicit hardware integration is tested only on the designated `galaxy` host or an explicitly declared integration target — a generic VM is **not** assumed to safely apply the `galaxy` host profile | `go test ./...` fixture + receipt inspection; hardware test gated on explicit target |
| 2 | `alex-cachyos check` after converge reports 0 drift; `niri validate` + `noctalia config validate` pass; `pacman -Dk/-Qk` clean (on the integration target; fixture asserts the same checks via fake runner) | `check` output + command exits + fixture |
| 3 | Pi packages: `pi list` shows all catalog **exact** pins with correct versions; `../../Projects/gentle-pi` resolved as local path package; receipt distinguishes `desiredExactPins` vs `resolvedInstalledVersions` | `pi list` vs catalog + receipt |
| 4 | Adopted clones: `~/Projects/gentle-ai` and `~/Projects/gentle-pi` at their `main` pins/tips with exact commits recorded in receipt (`rev-parse HEAD`); `pnpm install` plus the tested **local-main build/install path** places a package-local Gentle AI binary that actually reflects `main` (not stale `2.4.0`), verified via `managed-assets.json` and binary version/smoke | `git rev-parse HEAD` vs receipt + manifest + `gentle-ai --version` smoke |
| 5 | Dirty-worktree safety: `apply` with dirty `gentle-pi` clone aborts checkout step, preserves modifications, receipt marks `dirty:true` + `skipped:true` | Test with dirty clone fixture |
| 6 | 23 model routes: `subagents.json` and `models.json` byte-equal to catalog-rendered expectations; single source drives both | `check` diff + hash compare + fixture |
| 7 | `lean`/`task`/`enable_continue`/`debug` correctly set in `subagents.json` | JSON assertion |
| 8 | `~/.pi/agent/AGENTS.md` contains user environment guidance; `APPEND_SYSTEM.md` is package-managed (not overwritten with user blob) and `gentle-ai sync` preserves correct split | File compare + `managed-assets.json` verify |
| 9 | RDD: `gentle-ai review mode status --cwd <repo>` reports `effective:on`, `deciding:global` | Command output |
| 10 | `~/.pi/web-search.json` contains only `$ENV`/`!command` refs for credentials, no literal keys; receipts contain credential names only | JSON scan + receipt scan |
| 11 | Pi web-enabled `sdd-research` override present at `~/.pi/agent/subagents/sdd-research.md` with web tools allowlist; `retire_when: pi-sdd-research-web-capable` predicate encoded on `#3846` + `#471`; predicate is **false** on generic Gentle AI `v2.5` stable alone and **true** only when the consumed `main` packaged agent exposes exact web tools + non-empty grants and manifest+smoke pass | File + predicate unit test + manifest/smoke fixture |
| 12 | Receipts: one immutable JSON per run under `$XDG_STATE_HOME/alex-cachyos/receipts/` + `current` index; annotated tag `catalog-v*` exists and receipt records it; receipt distinguishes exact desired pins from resolved versions | Filesystem + `git tag --list` + receipt schema check |
| 13 | Auth remains interactive: no `auth.json` read during `apply`/`check`; `/login` steps documented as post-apply manual steps | Code grep + docs |
| 14 | `openspec/config.yaml` updated to `go test ./...` runner when Go tests land; `bash -n` retained during staged migration | Config file diff |
| 15 | Existing dirty worktree in `alex-cachyos` repo untouched by `apply`/`check`/`rollback` (rollback uses `git show`/isolated worktree, never `git checkout <tag>` in active tree) | `git status` before/after + rollback test |
| 16 | **Staged migration:** Bash `./apply` preserved and functional while Go CLI is built; Go validation (fixture + `check --dry-run`) passes before any cutover is proposed | Filesystem + `bash -n` + fixture |

## 19. Alternatives considered (not chosen)

- **Vendor package-managed assets verbatim** — rejected; violates Gentle Pi/Gentle AI install authority and `managed-assets.json` integrity (S6/S12).
- **Manage gentle clones via `pi install git:...`** — rejected; Pi reconciliation is destructive (`reset`+`clean`, S2 C4) and loses dirty work.
- **Copy combined `APPEND_SYSTEM.md`** — rejected; ownership split required (explore §3.3, S17); freezes generated contract text.
- **Use DMI/machine-id as catalog keys** — rejected per pre-proposal privacy decision; hostname + `--host` only.
- **Single receipt JSONL** — rejected per pre-proposal; per-run immutable JSON + index is auditable and append-safe.
- **Big-bang Bash→Go replacement** — rejected; staged migration preserves reviewability and a Bash fallback (§3.3, §14).
- **Retire `sdd-research` on generic stable** — rejected; Pi web-enabled predicate requires #3846 + #471 and exact web tools (§12).

## 20. References

- `explore.md` (corrected gatekeeper rerun) · `research.md` S1–S18 · `preproposal.md` rev 2 · `openspec/config.yaml`
- Pi docs: Settings (S1), Packages (S2), usage/AGENTS.md (S17); pi-web-access 0.27.0 (S3/S4); gentle-pi `2.2.0`/installer (S5–S7); gentle-ai `v2.4.0`/`v2.5.0-rc.1`/`sdd-research` spec (S8–S10/S18); trigger rules/intended usage (S11–S12); XDG 0.8 (S13); git checkout/tag/fetch (S14–S16)
- Pi web-enabled retirement tracking: `Gentleman-Programming/gentle-ai#3846` and `Gentleman-Programming/gentle-pi#471`
- `AGENTS.md` (user-owned) vs `APPEND_SYSTEM.md` (package-owned) split — pre-proposal confirmed, §10

---

### Reviewer checklist

- [ ] Catalog `global→role→host` precedence and **exact-version** pinning (desired vs resolved distinction) acceptable (§4, §9)
- [ ] Planner ordering, CLI surface, and **fixture-based validation** (no galaxy-on-generic-VM) acceptable (§5, §18)
- [ ] User-owned `AGENTS.md` vs package-managed `APPEND_SYSTEM.md` split correct (§7, §10)
- [ ] Dirty-worktree refusal (never `--force`) and non-mutating rollback (`git show`/isolated worktree, never `git checkout <tag>` in active tree) approved (§8, §17)
- [ ] Development-channel authority (`main` checkouts) and **local-main runtime gap** with tested integrity-preserving install path acknowledged (§8, §16)
- [ ] RDD explicit-enable + verify, secret-free web-search, interactive auth boundaries clear (§11)
- [ ] Pi web-enabled `sdd-research` retirement predicate (requires #3846 + #471, exact web tools, non-empty grants, manifest+smoke — generic stable alone insufficient) sound (§12)
- [ ] Staged migration (Bash preserved, Go validated alongside) and rollback/success criteria measurable and testable (§3.3, §14, §17, §18)

