# Bootstrap — first steps on a minimal CachyOS + COSMIC install

## Install

```bash
./apply --profile galaxy --only bootstrap
```

## What it does

1. Installs `paru`, `cosmic-store`, `flatpak`, `zsh`, `nano`
2. Installs **Google Chrome** from AUR (`paru -S google-chrome`)
3. Removes stock **Firefox** (CachyOS default) and leftover **chromium** if present
4. Removes bloat from the initial cleanup (VLC, Alacritty, Plymouth, LTS kernel, extra fonts/codecs/tools, …)
5. Drops `cachyos-zsh-config` + **vim** (zsh stays as a bare binary for Cursor; login shell remains fish)
6. Rewrites `~/.zshrc` to drop the CachyOS zsh source while keeping the `devtools` mise block if present
7. Strips Plymouth from mkinitcpio/GRUB when needed
8. Enables `ananicy-cpp` when the package is present

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
