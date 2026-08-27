# Desktop — Niri + Noctalia (default session)

COSMIC stays installed as a greeter option. Niri is the session this profile logs into.

## Install

```bash
./apply --profile galaxy --only desktop
```

Full `./apply --profile galaxy` runs this after apps (so `hyprwhspr` / `codexbar-cli` exist).

## What it does

1. Installs `niri`, `noctalia`, `xwayland-satellite` (not the `cachyos-niri-noctalia` meta)
2. Writes the live Galaxy configs:
   - `~/.config/niri/config.kdl`
   - `~/.local/state/noctalia/settings.toml`
   - `~/.config/hyprwhspr/config.json`
3. Sets **Niri** as the default session (`~/.dmrc`, AccountsService, cosmic-greeter `last_session`)
4. `hyprwhspr noctalia install` → bar plugin `goodroot/noctwhspr` (local STT, replaces Spokenly)
5. Enables Noctalia plugins: CodexBar meter, noctwhspr, battery-graph
6. Enables `hyprwhspr.service` (user)

Login screen stays **cosmic-greeter** (fingerprint / PAM `login`). Pick COSMIC in the session list if you want it for a one-off.

## Shortcuts that live in Niri (not COSMIC)

| Bind | Action |
|------|--------|
| Super+Space | Vicinae |
| Super+T | cosmic-term |
| Super+O | overview |
| Super+Shift+S | Noctalia settings |
| Super+Tab / Alt+Tab | recent windows (cursor stays put) |

## Remove

```bash
./apply --profile galaxy --only desktop --remove
```

Restores config backups. Leaves packages and the greeter last-session setting.
