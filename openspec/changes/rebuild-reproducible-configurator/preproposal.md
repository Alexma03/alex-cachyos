# Pre-Proposal — rebuild-reproducible-configurator

schema: gentle-ai.sdd-preproposal/v1
revision: 2
change_name: rebuild-reproducible-configurator
artifact_store: openspec
exploration_ref: openspec/changes/rebuild-reproducible-configurator/explore.md

## Research request

- Source classes: `documentation`, `open-web`
- Declared grants: `documentation=[fetch_content]`; `open-web=[web_search, source_check, fetch_content, get_search_content]`
- Product decisions requested: confirmed (orchestrator-owned)

## Admission outcome

- Status: admitted
- Observed exact grants used: `documentation` → `fetch_content`; `open-web` → `web_search`, `fetch_content`, `get_search_content` (all within declared grants; `source_check` declared but not invoked)
- No tool outside declared grants was used; no credentials, auth files, keys, tokens, cookies, sessions, or caches were read or reproduced.

## Evidence references

- Research artifact: `openspec/changes/rebuild-reproducible-configurator/research.md` (revision 1, outcome done)
- Source IDs: S1–S18 (Pi docs settings/packages, pi-web-access README (GitHub+npm), gentle-pi package.json/install script/README, gentle-ai sdd-research spec/intended-usage/trigger-rules/releases v2.4.0 & v2.5.0-rc.1/issue #3588, XDG Base Directory Spec 0.8, git-checkout/git-tag/git-fetch)

## Product decisions

- status: confirmed
- Reproduce the complete non-default Pi/Gentle AI environment, not only subagent routing.
- Install Pi packages from explicit pinned npm specs; use the local gentle-pi checkout as a path package.
- Manage `gentle-ai` and `gentle-pi` as adopted local checkouts tracking their `main` branches, refuse destructive updates on dirty worktrees, and record each resolved commit in receipts.
- Store one immutable JSON receipt per configurator execution under `$XDG_STATE_HOME/alex-cachyos/`, with a separate current index.
- Resolve the host profile from hostname with an explicit `--host` override; never use sensitive machine identifiers as catalog keys.
- Create annotated Git checkpoint tags per accepted catalog version; receipts record the catalog tag applied by each machine.
- Version user-owned global environment instructions as an `AGENTS.md` asset installed to `~/.pi/agent/AGENTS.md`; leave `APPEND_SYSTEM.md` fully package-managed by Gentle Pi.
- Use a hybrid credential flow: environment-variable or trusted-command references for API credentials, plus interactive per-machine OAuth/login steps. Never version credential values.
- Explicitly enable and verify global RDD mode after installation.
- Install the temporary Pi `sdd-research` override now and retire it automatically when the consumed stable Gentle AI/Gentle Pi runtime exposes official equivalent support.
- Treat package-managed agents, chains, support files, and manifests as outputs of the selected gentle-pi checkout, verified after install rather than vendored.

## Proposal readiness

- proposal_ready: true (selected research is done, evidence references are valid, and product decisions are confirmed)
