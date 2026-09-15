# Bootstrap — first steps on a minimal CachyOS install (ISO may ship COSMIC)

## Install

```bash
./apply --profile galaxy --only bootstrap
```

## What it does

1. Runs a full `pacman -Syu` update and installs `paru`, CachyOS Package Installer, `flatpak`, `zsh`, `nano` and UFW
2. Installs **Google Chrome** from AUR (`paru -S google-chrome`)
3. Updates every already-installed AUR package, excluding the locally patched Galaxy fingerprint package
4. Updates system and user Flatpak applications
5. Removes stock **Firefox** (CachyOS default) and leftover **chromium** if present
6. Removes bloat from the initial cleanup (VLC, Alacritty, Plymouth, LTS kernel, extra fonts/codecs/tools, …)
7. Drops `cachyos-zsh-config` + **vim** (zsh stays as a bare binary for Cursor; login shell remains fish)
8. Rewrites `~/.zshrc` to drop the CachyOS zsh source while keeping the `devtools` mise block if present
9. Strips Plymouth from mkinitcpio/GRUB when needed
10. Enables `ananicy-cpp` and UFW when present

Does **not** touch fingerprint, mise/node, or Cosmic core.

## Editors / shells

| Piece | Role |
|-------|------|
| fish | login shell (unchanged) |
| zsh | Cursor agent needs `/usr/bin/zsh` |
| nano | only terminal editor we keep |
| Cursor | real editor |

## Remove

```bash
./apply --profile galaxy --only bootstrap --remove
```

Does **not** reinstall bloat. Only prints a short note.
