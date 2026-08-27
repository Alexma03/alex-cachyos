# Vicinae — default launcher + clipboard on COSMIC

## Install

```bash
./apply --profile galaxy --only vicinae
```

## What it does

1. Installs `vicinae-bin` (AUR via paru)
2. Writes `/etc/environment.d/99-vicinae-cosmic.conf` → `COSMIC_DATA_CONTROL_ENABLED=1` (clipboard on COSMIC)
3. Enables `systemctl --user vicinae.service`
4. Cosmic shortcuts:
   - **Super alone** → disabled (so Super+key works)
   - **Super+Space** → Vicinae launcher
   - **Super+/** → still Launcher (stock) → `vicinae toggle`
   - **Super+V** → clipboard history
5. Overrides Cosmic `Launcher` system action to `vicinae toggle`

## After install

Log out/in once if clipboard history is empty (env var for data-control).

## Remove

```bash
./apply --profile galaxy --only vicinae --remove
```

Restores Cosmic shortcut backups and stops the user service. Leaves the package and environment.d file installed.
