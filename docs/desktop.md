# Desktop — Niri + Noctalia (default session)

Daily desktop is **Niri + Noctalia**. COSMIC stays only as a greeter login option
(and optional one-off session). Login screen is still **cosmic-greeter** (fingerprint / PAM `login`).

## Install

```bash
./apply --profile galaxy --only desktop
```

Full `./apply --profile galaxy` runs this after apps (so `hyprwhspr` / `ai-usagebar-bin` exist).

## What it does

1. Installs `niri`, `noctalia`, `xwayland-satellite`, `quickshell`
2. Writes the live Galaxy configs:
   - `~/.config/niri/config.kdl` (spawns Noctalia + `qs -c polkit`)
   - `~/.local/state/noctalia/settings.toml` (`shell.polkit_agent = false`)
   - `~/.config/hyprwhspr/config.json`
   - `~/.config/quickshell/polkit/` — Omarchy-style pkexec UI
3. Stops any other polkit agents (hyprpolkitagent, GNOME/KDE/LXQt, Noctalia agent)
4. Sets **Niri** as the default session (`~/.dmrc`, AccountsService, cosmic-greeter `last_session`)
5. `hyprwhspr noctalia install` → bar plugin `goodroot/noctwhspr`
6. Enables Noctalia plugins: `goodroot/noctwhspr`, `felipeartur/ai-usagebar`
7. Enables `hyprwhspr.service` and starts `qs -c polkit`

Niri is not a full DE. `pkexec` needs a graphical polkit agent — this profile uses a
**minimal Quickshell agent** (fingerprint square / password field, Esc to cancel, no Cancel
button). Do not enable Noctalia’s `shell.polkit_agent` or other agents at the same time.

## Shortcuts that live in Niri

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
