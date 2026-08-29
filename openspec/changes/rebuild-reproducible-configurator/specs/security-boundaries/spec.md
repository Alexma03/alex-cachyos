# Security Boundaries Specification

## Purpose

Cross-cutting invariants that apply to every capability: secrets are never read,
embedded, or recorded; biometric data is never collected or persisted; and the
untouched-file list is enforced. These invariants override any conflicting
requirement elsewhere.

## Requirements

### Requirement: No secret values anywhere

The configurator MUST NOT read `~/.pi/agent/auth.json`, `mcp.json`,
`mcp-cache.json`, `models-store.json`, live `web-search.json` values, keyring
content, or any credential cache. It MUST NOT embed credential values in the
catalog, templates, receipts, or tags. Credentials MUST be referenced by name only
(environment-variable or trusted-command references).

#### Scenario: Secret files are never read

- GIVEN any `apply`, `check`, `adopt`, or `rollback` run
- WHEN file access is audited
- THEN none of the secret files and caches listed above were opened

#### Scenario: No credential value appears in persisted artifacts

- GIVEN the catalog, all rendered templates, all receipts, and all tags
- WHEN they are scanned for credential values
- THEN only credential names and references are present

### Requirement: No biometric data anywhere

The configurator MUST NOT collect, store, or log fingerprint templates or any
content of `/var/lib/fprint`. Fingerprint verification MUST be limited to an
enrollment presence check (`fprintd-list`) and MUST be warn-only. The catalog and
receipts MUST NOT contain biometric data.

#### Scenario: Presence check only, warn-only

- GIVEN a host with enrolled fingerprints
- WHEN `check` runs the fingerprint verification
- THEN it performs only an enrollment presence check, emits at most a warning, and stores no biometric content

### Requirement: Untouched runtime state

The configurator MUST NOT modify Pi sessions, the `pi-pretty` frecency databases,
`~/.pi/npm` node_modules, `.engram/chunks`, `.atl`, `.codegraph`, Tailscale or
NordVPN session state, or the keyring. The actively used Pi state directory layout
(`~/.pi/agent/{npm,git}/`) MUST be left as Pi defines it — the configurator MUST
NOT XDG-migrate Pi's own state.

#### Scenario: Sessions and caches are untouched

- GIVEN a full `apply` run
- WHEN Pi session transcripts and cache directories are compared before and after
- THEN they are unchanged

#### Scenario: Pi state layout is not migrated

- GIVEN a host where Pi stores packages under `~/.pi/agent/npm` and git packages under `~/.pi/agent/git`
- WHEN any configurator run completes
- THEN those directories remain at their Pi-defined locations
