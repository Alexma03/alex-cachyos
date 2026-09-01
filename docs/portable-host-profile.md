# Portable Host Profile Evidence

This amendment proves host-policy and asset authorization with synthetic fixtures. It does **not** declare a real portable machine, hardware identifier, hostname, or live acceptance result.

## Safe workflow

1. Select a catalog-known profile with `--host HOST`.
2. Omit `--integration-target` for deterministic fixture evidence. The evidence kind is `fixture`, and no live verifier port is called.
3. Use `--integration-target HOST` only for an intentional live check. The target must resolve to the exact same canonical host as `--host`; a mismatch fails before live hardware checks.
4. Treat the synthetic `portable-synthetic` fixture as test data only. Production host values remain deferred.

Fixture evidence uses an allowlist of identifiers and statuses. It never includes command output, environment values, file contents, credentials, or biometric data. Live evidence additionally requires an immutable receipt identity; the receipt v1 schema is unchanged.

The amendment acceptance report is intentionally scoped as `portable-profile-amendment-three-module-fixture`. It proves the catalog, CachyOS policy, and evidence boundary added here. It does not substitute for the parent change's complete seven-module or live-host acceptance evidence.

## Typed desktop subset

The current WU-14 subset resolves the authoritative desktop package names from the embedded `templates/desktop/packages.pacman` asset and expands the former coarse workstation step into package, role-file, Quickshell-polkit, `.dmrc`, niri validation, Noctalia validation, and package-observation operations. Role files are rendered below the caller-observed home; host-owned fixed display, input-device, literal-home, and Cosmic-prune behavior remains separately authorized and default-deny.

User-file publication uses one-time adoption backups and same-directory atomic replacement. The production verification adapter can run only after the existing exact host/`--integration-target` gate admits it. Its presence is **not** live-host evidence: no real target, receipt, package transaction, or desktop file was exercised by this slice.

## Parent integration contract

| Parent work unit | Amendment handoff | Completion condition in parent |
|---|---|---|
| WU-5 | Multi-host catalog authority and exact pin identities | Add reviewed production host documents without copying synthetic values |
| WU-6 | Galaxy and portable fixture/golden shape | Run the complete seven-module fixture suite |
| WU-14 | Host-owned risk gates and role/host asset isolation | Integrate the complete desktop/verify factory surface |
| WU-20 | Canonical traceability and live-evidence gate | Record final output SHA and immutable live receipt ID on an explicitly selected target |

All four parent rows remain incomplete until this amendment has a passing verify report and native status recommends archive. Parent artifacts are not edited by this amendment.

## Hardware-free checks

```sh
go test ./internal/cli ./internal/app ./internal/acceptance -count=1
go test ./internal/acceptance -run 'TestCatalogFixturesMatchAcceptanceGoldens|TestHardwareFreeFixtureConvergenceAndRollbackRefusal' -v -count=1
```

The goldens record merge order, exact fixture pins, stable plan order/digests, offline check, dry-run non-mutation, first/second convergence, rollback action, risk opt-ins, and portable non-leakage. Dry-run and convergence fields come from the resolved plan running through the real application/executor orchestration with in-memory ports and isolated temporary state; the dry-run succeeds while the application lock is deliberately held, publishes only its audit receipt, and performs no mutation. Live mutation is deliberately deferred to parent WU-20.
