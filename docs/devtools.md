# Devtools — mise + Node LTS + pnpm/npm with supply-chain defaults

## Install

```bash
./apply --profile galaxy --only devtools
```

## What it does

1. Installs `mise` via pacman (pkexec)
2. Writes user configs:
   - `~/.config/mise/config.toml` — `node=lts`, `npm=latest`, `pnpm=latest`
   - `~/.config/pnpm/config.yaml` — cooldown / trust / strict builds
   - `~/.npmrc` — same policy for npm
3. Activates mise in fish / zsh / bash + appends `~/.local/bin` to PATH
4. Runs `mise install` and `corepack disable`

**Does not** remove pacman `nodejs` (required by `cursor-bin`).

## Verify

```bash
# new shell, or:
eval "$(mise activate bash)"
node -v          # mise LTS, not only /usr/bin
which node       # …/mise/installs/…
pnpm -v
npm -v
```

## Friction (intentional)

| Symptom | Fix |
|---------|-----|
| pnpm fails on build scripts | `pnpm approve-builds` or project `onlyBuiltDependencies` |
| too-new package (< 3 days) | wait, or `minimumReleaseAgeExclude` / `min-release-age-exclude` |
| npm wants scripts | `npm install-scripts approve` / `deny` |

## Remove

```bash
./apply --profile galaxy --only devtools --remove
```

Restores backed-up configs / strips managed shell blocks. Leaves pacman `mise` and `nodejs` installed (safe default).
