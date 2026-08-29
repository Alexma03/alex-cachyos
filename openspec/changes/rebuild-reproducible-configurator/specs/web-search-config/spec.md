# Web Search Configuration Specification

## Purpose

`~/.pi/web-search.json` is emitted from a secret-free template using
environment-variable or trusted-command credential references. Literal credential
values are never versioned; interactive and zero-config authentication flows remain
per-machine runtime steps.

## Requirements

### Requirement: Secret-free credential references

The web-search configuration template MUST use environment-variable references
(`$NAME` or `${NAME}`) or trusted-command references (`!command`) for provider
credentials, applying interpolation to provider credentials only. The catalog and
templates MUST NOT contain literal credential values. An explicit reference source
MUST NOT silently fall back to a stale credential when it fails; it fails that
provider locally.

#### Scenario: Template contains references, not literals

- GIVEN the rendered `~/.pi/web-search.json`
- WHEN its credential fields are inspected
- THEN each holds a `$NAME`/`${NAME}` or `!command` reference and no literal credential value

#### Scenario: No silent fallback on resolver failure

- GIVEN an explicit command reference that fails to resolve
- WHEN a provider request runs
- THEN that provider fails locally rather than falling back to a stale credential

### Requirement: Environment-variable precedence is not broken

The template MUST NOT override the documented environment-variable equivalents
(`OPENAI_API_KEY`, `EXA_API_KEY`, `GEMINI_API_KEY`, `PERPLEXITY_API_KEY`,
`CLOUDFLARE_API_KEY`) with literal values; explicit reference sources override
legacy environment variables per the extension's documented semantics.

#### Scenario: Env equivalents keep precedence over absent config values

- GIVEN a provider with no explicit reference in the rendered config and the corresponding environment variable set in the environment
- WHEN a web-search request runs for that provider
- THEN the environment-variable credential is used

### Requirement: Interactive and zero-config auth remain runtime steps

Zero-config flows (Exa MCP, Codex auth reuse via `/login`, Kimi via `/login
kimi-coding`) and opt-in browser-cookie profiles MUST remain interactive
per-machine runtime steps and MUST NOT be cataloged or automated. The configurator
MUST NOT read `web-search.json` live values or the web-search cache.

#### Scenario: Interactive logins are documented, not automated

- GIVEN the reproduction flow
- WHEN it completes
- THEN Codex and Kimi logins are documented as post-apply manual steps and no automated login attempt occurred
