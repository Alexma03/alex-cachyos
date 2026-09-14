# Receipts

Configurator state is kept in standard XDG state directories:

```text
$XDG_STATE_HOME/alex-cachyos/
├── receipts/<timestamp>-<shortHash>.json
└── current.json
```

When `XDG_STATE_HOME` is unset or empty, the state root defaults to
`$HOME/.local/state/alex-cachyos/`. An explicit absolute XDG state directory is
used as-is.

Every run writes one immutable receipt. A receipt is staged in its destination
directory, written with mode `0600`, fsynced, published without replacing an
existing receipt, and followed by a directory fsync. Receipt directories use
mode `0700`; earlier receipt files are never rewritten or deleted.

`current.json` is a small pointer containing the run ID, relative receipt path,
and receipt digest. It is staged and fsynced before an atomic rename, so readers
see the previous complete index or the new complete index, never a partial one.
A failed command is still a publishable receipt when storage succeeds.

Receipt evidence keeps desired state separate from resolved state: desired
package pins and policies describe what was requested, while resolved versions
and checkout commits describe what was observed. Receipts contain credential
names only. They contain no secret values, biometric data, or dirty-worktree
diffs.
