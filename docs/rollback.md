# Rollback safety

Rollback is a new forward operation. It never edits an earlier receipt and it
never rewinds the active configurator worktree.

## Request forms

```text
alex-cachyos rollback --receipt <run-id>
alex-cachyos rollback --tag catalog-vX.Y.Z
alex-cachyos apply --remove <module>[,<module>...]
```

Exactly one rollback source is required. A receipt selects the inverses already
recorded for one run. A catalog tag selects a prior immutable desired-state
snapshot. `--remove` limits normal planning to typed removal/inverse operations
for the selected modules; it is not an arbitrary file deletion switch.

The command parser and typed application boundary are available during the
staged migration. Production rollback execution remains fail-closed until the
repository has both the production catalog composition and the receipt-by-ID /
tagged-catalog adapters. This prevents fixture data from becoming machine
authority.

## Preconditions

Every managed-file inverse checks live state immediately before mutation:

- A configurator-created file is removed only when its live hash equals the
  receipt's recorded after-hash.
- An adopted file is restored only when the live hash still equals the
  recorded after-hash and its one-time `*.bak.alex-cachyos` backup is present
  and valid.
- A package-owned file delegates restoration to its package operation; a user
  backup is never treated as package authority.
- Any missing, unsafe, unreadable, or changed target blocks that inverse. The
  configurator does not overwrite post-apply user edits.

A successful rollback publishes a new immutable receipt with `rollbackOf` or
`reappliedCatalogTag`. The source receipt remains byte-identical.

## System rollback boundary

The configurator restores only the file/configuration state represented by
typed inverses. Filesystem snapshots, package-database recovery, boot-wide
recovery, and other system-wide rollback remain owned by Snapper. The command
reports this delegation; it does not reimplement or invoke an implicit system
rollback.

## Git and dirty-worktree guarantee

Tagged catalogs are read with Git object reads. When an older binary/schema is
required, it runs from a detached configurator-owned worktree below XDG state.
The active checkout is never cleaned, reset, stashed, committed, force-checked
out, or switched to the tag. Cleanup removes only a marked, clean temporary
worktree; otherwise it is left intact for inspection.

Before any real rollback, take or confirm the relevant Snapper checkpoint and
read the selected receipt with `alex-cachyos receipt show` or
`alex-cachyos status`.
