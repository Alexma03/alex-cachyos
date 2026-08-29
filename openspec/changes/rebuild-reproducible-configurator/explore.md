# Exploration — rebuild-reproducible-configurator

Status: exploration (no product code implemented).
Revision: corrected against fresh evidence (gatekeeper rerun) — (1) the alex-cachyos
worktree is heavily dirty with pre-existing user work, not clean/tooling-only;
(2) `~/.pi/agent/settings.json` contains only default model/provider/thinking,
changelog marker, packages, and theme — no `session_resources`, `default_mode`,
`enable_continue`, `debug`, or `model_profiles`; (3) `~/.pi/agent/subagents.json`
owns lean/task/continue/debug plus the 23 `model_profiles` (`effort` keys), while
`~/.pi/gentle-ai/models.json` owns the corresponding 23 Gentle AI route entries
(`thinking` keys) — the duplication is model-id + level across two files with
differing key spellings; (4) RDD is globally enabled (user's explicit desired
state), so reproduction must explicitly enable it, not default it off; (5) managed
prompts/chains/support/manifest come from the selected local gentle-pi checkout as
authority, not from verbatim copies; (6) `APPEND_SYSTEM.md` mixes package-managed/
generated contracts with user-owned environment guidance and must not be copied
whole. Sanitized secret constraints and all valid architecture findings from the
first artifact are preserved.

Scope: map the current alex-cachyos Bash implementation and the target Go CLI
architecture, reproduce the non-default Pi/Gentle AI environment across machines,
and identify reuse, replacement, secrets, and operational concerns for the
downstream proposal/spec/design/tasks phases.

## 1. Current alex-cachyos implementation (Bash)

### 1.1 Repository state

- Git: branch `main`, HEAD `8397ddc7aad00af1a5686418a3250db58b93be43`, `origin/main`
  at the same commit; 9 commits; no tags; no local feature branches.
- **Worktree state: heavily dirty with pre-existing user work.** `git status
  --short --branch` shows modified/deleted/untracked changes across `README`,
  `apply`, `bin`, `docs`, `lib`, `modules` (including `modules/verify.sh`),
  `overlays` (including new greetd/Noctalia/PAM assets under
  `overlays/galaxy/etc/{greetd,noctalia-greeter,pam.d}`), `packaging`, `profiles`,
  and `templates`, plus untracked `.codegraph/`, `.gitignore` changes, and
  `openspec/` (including this change's artifacts). This is the user's own
  uncommitted work, not tooling noise, and it must be preserved intact — the
  OpenSpec project context already requires `apply.preserve_dirty_worktree: true`.
  Neither the Go rebuild nor any apply run may clean, stash, or commit these
  changes. A Codex turn-diff checkpoint ref exists under
  `.git/refs/codex/turn-diffs/checkpoints/` pointing at tree `fe29a3e536...`, i.e.
  tooling also captured worktree state; both sources agree the tree is dirty.
- Machine-local ignored runtime state: `.codegraph/` (index DB; self-ignoring via
  its own `.gitignore`) and `.atl/` (skill-registry cache; listed in root
  `.gitignore`). Neither may be committed or cataloged.
- No test files, no test framework, no Makefile/Taskfile, no package manifest, no
  Go module, no CI test command. Repository-wide validation today is
  `bash -n` over the scripts plus Python stdlib JSON parsing of `profiles/*.json`.
  `shellcheck` and `shfmt` are not installed. Strict TDD is disabled until a real
  repository-wide runner exists.

### 1.2 Entrypoint `./apply`

- Bash CLI with `set -euo pipefail`; single-instance `flock` lock at
  `$XDG_RUNTIME_DIR/alex-cachyos-$(id -u).lock` (conflict exit code 75).
- Runs as the normal user; system steps are elevated with `pkexec` (fingerprint
  polkit agent), never `sudo`.
- Options: `--profile NAME` (default `galaxy`), `--only m1,m2`, `--with`,
  `--without`, `--remove`, `--dry-run`, `--check` (alias for `--only verify`),
  `--list`, `-h/--help`.
- `MODULE_ORDER=(bootstrap fingerprint devtools apps vicinae desktop verify)`.
  A profile JSON is parsed by emitting `AO_MOD_*` assignments via `python3` +
  `eval`; unknown module names in a profile are rejected.
- `ao_should_run_module` implements selection: `--only` wins, then `--with`,
  then `--without`, then the profile default.

### 1.3 Modules and shared libraries

| Piece | Role |
|---|---|
| `lib/common.sh` | logging, `ao_has_cmd`, `ao_pacman_mark_explicit_files` (marks declared packages explicit so `-Rns` never prunes them), `ao_need_root`, `ao_root` (pkexec), `ao_install_file`/`ao_restore_file` and user variants with `*.bak.alex-cachyos` backup/restore, `ao_should_run_module` |
| `lib/webapp.sh` | Chrome `--app` webapp install/remove + launcher placement in `~/.local/bin` |
| `modules/bootstrap.sh` | `pacman -Syu`, want/remove package lists, atomic GRUB regeneration, plymouth strip, ananicy-cpp + UFW enable, Chrome via paru, minimal `.zshrc` rewrite (marker blocks), LTS-kernel removal deferral |
| `modules/fingerprint.sh` | builds `packaging/libfprint-egismoc-sdcp-git` (PKGBUILD with pinned `_commit` and pinned patch sha256, `provides`/`conflicts` stock libfprint), installs with `pacman -U`, applies PAM overlays for sudo/polkit, clears `IgnorePkg`, skip-rebuild when installed version already matches pinned commit+pkgrel |
| `modules/devtools.sh` | mise via pacman, user configs `~/.config/mise/config.toml`, `~/.config/pnpm/config.yaml` (with `@HOME@` substitution), `~/.npmrc`; shell activation blocks with marker comments in fish/zsh/bash; mise install node LTS/npm/pnpm 12, corepack disabled; restore strips blocks |
| `modules/apps.sh` | pacman + AUR package lists, docker/tailscale/nordvpn service + group setup, docker-desktop GUI autostart disabled, webapps from `webapps.list`, removes native telegram-desktop when webapp replaces it |
| `modules/vicinae.sh` | installs `vicinae-bin`, enables user service |
| `modules/desktop.sh` | Niri runtime packages, user templates (niri config, noctalia config with `@HOME@`, hyprwhspr, quickshell polkit), greetd + AccountsService + `.dmrc`, prunes the legacy Cosmic stack (packages, PAM, flatpak remotes/cache, user state), restores package-owned PAM files from staged archives, keeps exactly one graphical polkit agent (`qs -c polkit`), enables Noctalia plugins |
| `modules/verify.sh` | post-apply convergence: `niri validate`, `noctalia config validate`, byte-compare user configs to templates, mise/pnpm checks, declared-package presence, legacy-remnant checks, greetd/boot checks, PAM compare, user services, `fprintd-list` presence (warn-only; never reads or stores biometric data), `pacman -Dk` + `pacman -Qk`, failed-unit checks, kernel/initramfs/GRUB checks, `.pacnew` warning |

### 1.4 Profiles, templates, overlays, packaging, docs

- `profiles/galaxy.json`: single profile; all seven modules enabled; `overlay_profile: galaxy`.
- `templates/`: `bootstrap/packages.{want,remove}`, `apps/packages.{pacman,aur}` +
  `webapps.list` (`name|url|icon_url`), `desktop/packages.pacman`,
  `devtools/{mise.config.toml,npmrc,pnpm.config.yaml}`, `niri/config.kdl`
  (Galaxy multi-monitor layout, es layout, binds), `noctalia/config.toml`
  (`@HOME@` substitution), `hyprwhspr/config.json`, `quickshell-polkit/`,
  `vicinae/`.
- `overlays/galaxy/etc/`: `greetd/config.toml`, `noctalia-greeter/greeter.toml`,
  `pam.d/{alex-cachyos-login,polkit-1,sudo}` (fprintd `sufficient` for polkit/sudo).
  These greetd/Noctalia/PAM assets are among the new pre-existing user changes in
  the dirty worktree.
- `packaging/libfprint-egismoc-sdcp-git/`: PKGBUILD pinning upstream PR #5 tip
  `8749008832ee1f313bfca4d3c04340df84b2bc27`, patch
  `0001-egismoc-drop-sdcp-claim-on-close.patch` with pinned sha256, meson build
  with `-D drivers=egismoc`. This is the existing reproducible-pinning pattern.
- `bin/alex-cachyos-webapp-launch`: setsid Chrome `--app` launcher.
- `docs/`: bootstrap, fingerprint, devtools, apps, vicinae, desktop (Spanish and
  English mixed; default technical artifacts for the rebuild are English).
- `.gitignore`: `.atl/` plus user edits in the dirty worktree.

### 1.5 Testing capability

No unit/integration/e2e harness. Validation = `bash -n` + JSON parse
(`openspec/config.yaml` `verify.test_command`). A Go implementation should
introduce `go test` coverage and update `openspec/config.yaml` when it exists.

## 2. Target Go CLI architecture (already selected — to design against, not re-decide)

- Declarative catalog (typed schema, validated) replacing JSON+`eval` and ad-hoc
  package-list parsing.
- Profile precedence: global → role → host. Today there is one flat profile and a
  hardcoded default (`AO_PROFILE=galaxy`); host detection should replace the
  hardcoded default while keeping `galaxy` as the only concrete host profile for now.
- Shared planner: ordered steps with module dependencies (apps before desktop;
  bootstrap first; verify last), replacing `MODULE_ORDER` + shell selection logic.
- Embedded assets: `go:embed` templates/overlays/packaging into the binary;
  removes reliance on repo-relative paths at runtime.
- Git-tag checkpoints: version-pin catalog snapshots with tags (e.g. tag before
  destructive changes, or tag per catalog version) so rollback and reproduce
  reference a known state. Follows the PKGBUILD commit-pin precedent.
- Detect-and-adopt: detect machine identity/profile, detect existing unmanaged
  files and adopt them through the existing `*.bak.alex-cachyos` convention.
- Local receipts: machine-local records of applied steps and file hashes (e.g.
  `~/.local/state/alex-cachyos/`), referencing credential names, never values.
- No biometric data: fingerprint module only checks enrollment presence
  (`fprintd-list`); receipts and catalogs must never contain prints or
  `/var/lib/fprint` content.

## 3. Pi/Gentle AI environment to reproduce (non-default, per machine)

### 3.1 Source checkouts (development branches)

- `~/Projects/gentle-ai`: git clone of `Gentleman-Programming/gentle-ai`, checked
  out at `origin/main` (`782e8dfe6b8ac26607e3239eb1e004ca7db1df0b`); only `main`
  locally; upstream remote refs show many development branches (rdd-wave*, feat/*,
  etc.). Go module `github.com/gentleman-programming/gentle-ai/v2`, binary
  `cmd/gentle-ai`, GoReleaser linux/darwin amd64/arm64, minisign-signed checksums,
  brew/go/binary install methods, channels stable/beta/nightly.
- `~/Projects/gentle-pi`: git clone of `Gentleman-Programming/gentle-pi`, checked
  out at `origin/main` (`a3d87c196268774c8989169e45634e9b46066881`); only `main`
  locally. npm package `gentle-pi` v2.2.0; postinstall
  (`scripts/install-gentle-ai.mjs`) installs a package-local Gentle AI binary
  pinned at `INSTALLER_VERSION = "2.4.0"` (signed release asset with SHA-256 +
  minisign, Go sumdb source-build fallback, install lock + tombstones, version
  bundle directory). `GENTLE_PI_SKIP_GENTLE_AI_INSTALL=1` escape hatch.
- Reproduction requirement: the configurator must be able to clone (or reuse
  existing) development-branch checkouts of both repos, update them
  (`git fetch` + checkout of a pinned branch/commit), and run `pnpm install`
  (gentle-pi) idempotently with its postinstall intact.

### 3.2 Pi package sources (npm, via Pi's package manager)

From `~/.pi/agent/settings.json` `packages` (non-secret structure):

- `npm:pi-commandcode-provider` (model provider)
- `npm:pi-subagents-j0k3r` (subagent support)
- `npm:@juicesharp/rpiv-ask-user-question`
- `npm:pi-web-access` (web research)
- `npm:@juicesharp/rpiv-todo`
- `npm:pi-btw`
- `../../Projects/gentle-pi` (local path package — the harness itself)
- `npm:pi-mcp-adapter` (also present in `~/.pi/npm/package.json` as `^2.6.0`)
- `npm:gentle-engram` (memory persistence)
- `npm:pi-antigravity`

Package versions should be recorded as ranges/pins in the catalog; `pi-mcp-adapter`
is evidence of a concrete resolved pin (`^2.6.0`).

### 3.3 Global prompts and settings

- `~/.pi/agent/APPEND_SYSTEM.md`: **mixed ownership — do not copy the file whole.**
  It contains (a) user-specific environment guidance: pkexec/fingerprint elevation
  rules, CachyOS + niri + Noctalia context, and (b) package-managed/generated
  contract text: the `gentle-ai:agent-routing` implementation-routing block and the
  receipt-driven-development switch documentation, which are supplied/generated by
  gentle-ai/gentle-pi at runtime. Later design must preserve ownership boundaries:
  regenerate the package-owned sections from the installed checkout and carry the
  environment sections as a user-owned overlay (e.g. a separate user append file),
  never treat the combined file as one opaque blob to copy.
- `~/.pi/agent/settings.json`: **minimal and default-only** — `defaultModel:
  gpt-5.6-sol`, `defaultProvider: openai-codex`, `defaultThinkingLevel: medium`,
  `lastChangelogVersion: 0.84.4` (runtime changelog marker — reproduce the config
  shape, not a stale version literal), the `packages` list above, and `theme: dark`.
  It does NOT contain `session_resources`, `default_mode`, `enable_continue`,
  `debug`, or `model_profiles`; those live in `subagents.json` (below).
- `~/.pi/agent/subagents.json`: owns `session_resources: lean`, `default_mode:
  task`, `enable_continue: false`, `debug: false`, plus the 23-entry
  `model_profiles` map (`effort` keys) — see §3.4.
- `~/.pi/gentle-ai/models.json`: the 23 Gentle AI route entries (`thinking` keys)
  corresponding to `subagents.json` `model_profiles` — see §3.4.
- `~/.pi/gentle-ai/persona.json`: `{"mode":"neutral"}`.
- `~/.pi/gentle-ai/background-subagents.json`:
  `{"policy":"on","schema":"gentle-pi.background-subagents/v1"}`.
- `~/.pi/agent/gentle-ai/managed-assets.json`: SHA-256 manifest of the 23 agent
  prompts, 4 chains, and support docs — the harness verifies managed-asset
  integrity against this manifest. **Verification only — never a copy source.**
- `~/.pi/agent/gentle-ai/support/`: `sdd-status-contract.md`, `strict-tdd.md`,
  `strict-tdd-verify.md` — package-managed assets, refreshed by the harness
  install, not copied verbatim.
- `~/.pi/agent/chains/`: `4r-review.chain.md`, `sdd-full.chain.md`,
  `sdd-plan.chain.md`, `sdd-verify.chain.md` — package-managed assets, same rule.

### 3.4 Model routing (23 profiles) — actual duplication

The same 23 profile names → provider-prefixed model ids → level appear in exactly
two files, and the two files use different key spellings for the level:

- `~/.pi/agent/subagents.json` `model_profiles`: keys `gentle-ai-{explore,worker,
  verify}`, `sdd-{init,onboard,explore,research,proposal,spec,design,tasks,apply,
  verify,status,sync,archive}`, `review-{risk,reliability,resilience,readability}`,
  `jd-{judge-a,judge-b,fix-agent}`; each entry is `{"model": "<provider>/<model>",
  "effort": "max|high|xhigh"}` (e.g. `sdd-proposal` → `commandcode/meta/
  muse-spark-1.2-contributor` at `xhigh`).
- `~/.pi/gentle-ai/models.json`: the same 23 keys with the same model ids, but the
  level key is `thinking` instead of `effort`, with corresponding values (`max`,
  `high`, `xhigh`).

`settings.json` does NOT participate in this duplication anymore. The catalog
should model profile → provider/model/effort once, render both file shapes from
that single source of truth, and diff-manage the two so they cannot drift.

### 3.5 Lean/task behavior

`session_resources: lean`, `default_mode: task`, `enable_continue: false`,
`debug: false` are non-default Pi settings and they live in
`~/.pi/agent/subagents.json`; reproduction must set them on each machine, not rely
on Pi defaults.

### 3.6 Receipt-driven development (RDD) global mode and review routes

- **Current machine state: globally enabled.** `gentle-ai review mode status`
  reports `on`, decided by global. Package policy is opt-in / off by default, but
  this user has explicitly chosen enabled as their desired state. Reproduction
  must explicitly enable the switch (write the same global state) rather than
  defaulting it off — silently reproducing the package default would lose the
  user's choice. The switch (`gentle-ai review mode enable|disable|status`) is
  documented in `APPEND_SYSTEM.md`; upstream work `feat/rdd-default-off` and
  `feat/3766-rdd-mode-api` exist. While disabled, delivery follows ordinary
  repository policy and reports `disabled/unmanaged`. Never fabricate approvals;
  receipts are review evidence only.
- Review routes (from gentle-pi `openspec/specs/review-routing/spec.md`): ordinary
  start classifies `base_tree → complete_snapshot_tree` as `trivial | standard |
  full-4R`; ≥401 changed lines or hot paths force `full-4R` with lenses risk,
  resilience, readability, reliability; commit/push never classify or start
  review; receipts are review evidence only, never delivery authority; dangerous-
  command confirmation stays authoritative. Gentle AI supplies runtime-specific
  RDD instructions at runtime (per `skills/gentle-ai/SKILL.md`); the harness never
  invents a review route or gate.

### 3.7 Web research configuration (no credentials)

Sanitized fact (do not read `~/.pi/web-search.json`): it contains provider routing
plus a summary-model selection. Reproduction MUST use a secret-free template with
credential references (provider names/order and summary-model id are catalogable;
keys/tokens are not), and interactive OAuth login remains a per-machine runtime
step, never a catalog step.

### 3.8 Temporary global sdd-research override with retirement condition

- `~/.pi/agent/subagents/sdd-research.md` is a temporary subagent-profile override
  (separate directory from the managed `agent/agents/` assets): front-matter
  `name: sdd-research`, `model: commandcode/deepseek/deepseek-v4-flash`,
  `thinking: max`, tool allowlist `read, grep, find, edit, write, web_search,
  source_check, fetch_content, get_search_content, mem_search,
  mem_get_observation, mem_save`; body = the SDD research executor contract
  (evidence admission `documentation=[fetch_content]`,
  `open-web=[web_search,source_check,fetch_content,get_search_content]`,
  source collection, hybrid OpenSpec/Engram persistence,
  `gentle-ai.sdd-research/v1` artifact schema, pre-proposal schema
  `gentle-ai.sdd-preproposal/v1`, blocked/partial semantics).
- Upstream state: gentle-ai already ships an `openspec/specs/sdd-research/spec.md`
  (Closed Capability Admission, Auditable Evidence Integrity, Hybrid Completion
  and Recovery), and goldens for `sdd-research` agents exist in gentle-ai
  (`testdata/golden/sdd-claude-agent-sdd-research.golden` etc.), i.e. official
  support is near. Retirement condition: when official sdd-research agent support
  lands in a gentle-ai release consumed by the environment, the temporary
  `subagents/sdd-research.md` override MUST be removed and the managed asset used;
  the catalog should encode the condition (e.g. `retire_when: gentle-ai >= <release
  that ships sdd-research agent>`) and the configurator should check it on update.

### 3.9 Managed assets: installation authority and verification

- The selected local gentle-pi checkout (the `../../Projects/gentle-pi` path
  package) is the authority and refresh mechanism for managed assets: installing
  it runs the postinstall that places/updates the package-local Gentle AI binary,
  agents, chains, support docs, and `managed-assets.json` in `~/.pi`. The
  configurator must NOT copy these package-managed files verbatim from a captured
  machine as its primary installation strategy.
- After installation, the configurator should verify managed-asset integrity
  against `~/.pi/agent/gentle-ai/managed-assets.json` (the harness's own manifest
  check), and record filenames + hashes in receipts.
- Genuine user-owned overlays to catalog (not copy from the package): the
  environment sections of `APPEND_SYSTEM.md`, `persona.json`,
  `background-subagents.json`, the temporary `subagents/sdd-research.md` override,
  the `settings.json` minimal shape, the `subagents.json` lean/task/debug/
  model_profiles shape, and the `models.json` rendering.

## 4. Secrets / auth / cache / session files — never in Git or receipts

- `~/.pi/agent/auth.json` (provider credentials/OAuth tokens) — do not read, do
  not catalog.
- `~/.pi/web-search.json` and `~/.pi/web-search-cache/**` (provider routing with
  live credential values + cached results).
- `~/.pi/agent/mcp.json`, `mcp-cache.json`, `models-store.json`,
  `commandcode-models.json` (may embed tokens/model-store state).
- `~/.pi/agent/sessions/**` (session transcripts/history).
- `~/.pi/agent/pi-pretty/**` (frecency/history mdb databases).
- `~/.pi/npm/` node_modules (installed deps; only `package.json` shape is
  catalogable).
- `.engram/chunks/**` in gentle-ai (memory chunks), `.atl/`, `.codegraph/` in
  alex-cachyos, `~/.pi/gentle-ai` only for the non-secret JSON files listed.
- `/var/lib/fprint` (biometric templates — presence check only), keyring content,
  `~/.npmrc` user auth lines if any (the managed devtools `npmrc` template itself
  has none), NordVPN/Tailscale session state.
- Rule for receipts: store paths, hashes, versions, and credential *names*, never
  values.

## 5. Reuse vs. replace

### Reuse (map directly into Go design)

- `pkexec` elevation flow and fingerprint-polkit UX (keep; `ao_root` semantics).
- `*.bak.alex-cachyos` backup/restore adoption contract (keep as the adoption
  convention; implement in Go with a manifest).
- Package-list content and `--needed`/explicit-marking philosophy
  (`ao_pacman_mark_explicit_files`).
- PKGBUILD pinning pattern (pinned `_commit` + pinned patch sha256 +
  provides/conflicts) — the model for catalog version pinning.
- Verify checks inventory (niri validate, noctalia config validate, cmp templates,
  package presence, greetd/boot, PAM compare, failed units, pacnew warning).
- flock single-instance lock, `--dry-run`/`--only`/`--remove`/`--check` CLI
  surface, LTS-kernel deferral guard, atomic GRUB generation, legacy-stack
  pruning order (install new path before removing old).
- Webapp launcher + Chrome `--app` templates; Noctalia `@HOME@` substitution
  approach (replaced by typed templating but same keys).
- Module content knowledge for the Pi reproduction (packages list,
  settings/subagents/models file shapes, `APPEND_SYSTEM.md` user-owned sections,
  managed-assets manifest for verification only).

### Replace (do not layer indefinitely)

- `python3` + `eval` profile parsing → validated typed catalog (global/role/host).
- `MODULE_ORDER` + `ao_should_run_module` → planner with dependency graph and
  explicit ordering constraints (bootstrap → fingerprint/devtools/apps/vicinae →
  desktop → verify).
- Repo-relative file reads at runtime → `go:embed` assets.
- Inline `ao_root bash -c` heredocs with interpolated paths → Go exec with
  `pkexec` argv and embedded, parameterized scripts (avoids quoting bugs).
- grep/awk package-list parsing → structured catalog lists with schemas.
- Manual `mktemp` staging + `sed` substitution → typed template rendering with
  `@HOME@`/`@USER@` tokens and validation.
- Single hardcoded default profile → host detection + precedence resolution,
  keeping `galaxy` as the concrete host profile.
- Drift checks scattered in `verify.sh` → shared drift engine comparing receipts
  (content hashes) and templates.
- Spanish/English doc mix → English technical artifacts (keep existing docs until
  the Go CLI replaces the Bash one).

## 6. Operational concerns for the Go design

- Idempotency: preserve `pacman -Q` pre-checks, `--needed`, skip-rebuild for the
  pinned fingerprint package; planner marks a step satisfied when target state is
  met; receipts record step outcomes with hashes.
- Drift: verify compares live files to embedded templates and receipts; report
  drifted files with the backup path; `--check` stays non-mutating and
  offline-safe.
- Adoption: detect-and-adopt existing unmanaged files with one-time backup;
  hostname-based profile suggestion; adopt existing gentle-ai/gentle-pi clones
  instead of re-cloning.
- Rollback: `--remove` per module restoring backups; receipts must allow
  inverse-plan execution; system-level rollback remains Snapper-backed (desktop
  module already notes Snapper owns machine rollback state).
- Offline: `--dry-run`, `--check`, and config-file steps must work offline;
  package/AUR/build steps require network and should be grouped and reported
  upfront (like today's dry-run messaging).
- Version pinning: catalog pins package lists (optionally versions), PKGBUILD
  commit+sha256, gentle-ai `v2.4.0` via gentle-pi installer, gentle-pi `v2.2.0`,
  pi packages with semver ranges; webapp icon URLs are remote and must degrade
  gracefully (existing favicon fallback).
- Source-checkout updates: gentle-ai/gentle-pi checkouts must support
  fetch + checkout of pinned dev branches/commits, `pnpm install` (gentle-pi
  postinstall installs the package-local Gentle AI binary; handle
  `GENTLE_PI_SKIP_GENTLE_AI_INSTALL` semantics and integrity verification),
  and detect locally modified clones before updating (preserve dirty worktrees).
- RDD reproduction: explicitly set the global review-mode state to the user's
  desired value (enabled) and verify with `gentle-ai review mode status`; do not
  rely on the package's opt-in default-off policy.
- Managed assets: install via the selected gentle-pi checkout; afterwards verify
  integrity against `managed-assets.json`; never vendor package-managed assets as
  the primary install path.
- APPEND_SYSTEM.md ownership: split package-generated contract sections from
  user-owned environment sections; regenerate the former from the installed
  checkout and overlay the latter, so gentle-pi updates do not clobber the
  user's environment guidance and the user's copy never freezes generated
  contract text.
- sdd-research override retirement: encode `retire_when` condition and remove the
  temporary `subagents/sdd-research.md` once the consumed gentle-ai release ships
  the official sdd-research agent; verify managed-assets manifest consistency
  after retirement.
- Pi settings duplication: `subagents.json` `model_profiles` (`effort`) and
  `~/.pi/gentle-ai/models.json` (`thinking`) carry the same 23 profile → model →
  level data under different key spellings; generate both from one catalog source
  and diff-manage.
- Web-search: template with credential references; interactive OAuth remains a
  post-apply runtime step; receipts reference names only.
- No biometric data anywhere in catalog, assets, or receipts; fingerprint verify
  stays presence-only.
- Update `openspec/config.yaml` testing section when Go tests land
  (`go test ./...` + `bash -n`/JSON checks during transition).

## 7. Open questions for downstream phases

1. Receipt location/format: `~/.local/state/alex-cachyos/receipts/*.json` vs
   XDG state dir; single JSONL vs per-step files; retention/cleanup policy.
2. Host detection keys: hostname, `/etc/machine-info`, DMI product — and how
   `galaxy` is selected on the Galaxy Book without hardcoding.
3. Git-tag checkpoint semantics: tag before destructive mutation only, or one tag
   per catalog release; whether checkpoints live in the alex-cachyos repo or a
   machine-local refs namespace.
4. Pi provider credential flow: which providers require interactive OAuth at
   first run and how the configurator detects "authenticated but not cataloged"
   state without reading auth.json.
5. Whether gentle-ai/gentle-pi dev-branch checkouts should default to a pinned
   commit (reproducible) or track a named branch (current behavior).
6. Exact retirement release for the sdd-research override (depends on gentle-ai
   release that ships the official sdd-research agent asset).
7. How `APPEND_SYSTEM.md`'s package-generated vs user-owned sections are split and
   re-merged on gentle-pi updates (user append file vs template merge).
8. Where `gentle-ai review mode` global state lives so reproduction can set and
   verify the user's desired (enabled) state on a fresh machine.

## 8. Evidence references

- alex-cachyos: `apply`, `lib/*.sh`, `modules/*.sh`, `profiles/galaxy.json`,
  `templates/**`, `overlays/galaxy/**`, `packaging/libfprint-egismoc-sdcp-git/*`,
  `docs/*`, `openspec/config.yaml`, `.gitignore`, git refs/logs, dirty-worktree
  status (`git status --short --branch`).
- gentle-ai: `scripts/install.sh`, `.goreleaser.yaml`, `package.json`,
  `openspec/specs/sdd-research/spec.md`, `testdata/golden/sdd-*-sdd-research*`,
  git refs (tags v0.1.1…v1.30.7, origin/main `782e8dfe6b8...`).
- gentle-pi: `package.json` (v2.2.0), `scripts/install-gentle-ai.mjs`
  (`INSTALLER_VERSION=2.4.0`, signed release asset + go-sumdb fallback),
  `scripts/gentle-ai-installer.mjs`, `skills/gentle-ai/SKILL.md`,
  `openspec/specs/review-routing/spec.md`, git refs (origin/main `a3d87c196...`).
- `~/.pi`: `agent/settings.json`, `agent/subagents.json`, `agent/APPEND_SYSTEM.md`,
  `agent/subagents/sdd-research.md`,
  `agent/gentle-ai/{managed-assets.json,support/*}`,
  `gentle-ai/{models.json,persona.json,background-subagents.json}`,
  `agent/chains/*`, `agent/agents/*` (filenames + manifest hashes only),
  `npm/package.json`. Secret/cache/session files were not read (see §4).
