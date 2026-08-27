# alex-cachyos

Configuración reproducible de **CachyOS** para las máquinas de Alex.

Parte de una instalación limpia de CachyOS y aplica overlays del sistema
(paquetes locales, PAM, greetd, etc.) por **perfil de máquina**. No toca
`/usr/share/omarchy/` ni asume Omarchy.

```bash
git clone https://github.com/Alexma03/alex-cachyos.git
cd alex-cachyos
./apply --profile galaxy
```

## Diseño

| Pieza | Rol |
|-------|-----|
| `profiles/*.json` | Qué módulos activar por máquina |
| `modules/*.sh` | Install / remove de cada feature |
| `packaging/` | PKGBUILDs locales (`provides`/`conflicts` correctos) |
| `overlays/<profile>/etc/` | Ficheros de configuración del sistema |
| `./apply` | CLI idempotente |

### Reglas

- Sustituir libs del sistema con **paquetes pacman** (`provides` + `conflicts`), no con `ninja install` ni `/opt` overlays.
- Ficheros en `/etc` con backup `*.bak.alex-cachyos` y restore en `--remove`.
- Un perfil = una máquina (o familia). El primero: **galaxy**.

## Perfiles

| Perfil | Qué hace |
|--------|----------|
| `galaxy` | Galaxy Book — bootstrap + fingerprint + mise + apps + vicinae |

## Módulos

`bootstrap` · `fingerprint` · `devtools` · `apps` · `vicinae`

```bash
./apply --profile galaxy                      # instalar
./apply --profile galaxy --only bootstrap     # limpieza + Chrome/paru/zsh
./apply --profile galaxy --only fingerprint   # solo huella
./apply --profile galaxy --only devtools      # mise + pnpm/npm
./apply --profile galaxy --only apps          # programas + webapps Chrome
./apply --profile galaxy --only vicinae       # launcher + clipboard
./apply --profile galaxy --only fingerprint --remove
./apply --profile galaxy --dry-run
```

## Bootstrap

Limpieza inicial de CachyOS minimal + Chrome (AUR), paru, cosmic-store, zsh pelado
(para Cursor). Quita Firefox stock y vim/`cachyos-zsh-config`. Login sigue en fish;
editor de terminal: nano.

Ver [docs/bootstrap.md](docs/bootstrap.md).

## Fingerprint (galaxy)

Paquete local `packaging/libfprint-egismoc-sdcp-git`:

- Base: [TenSeventy7/libfprint-egismoc-sdcp](https://github.com/TenSeventy7/libfprint-egismoc-sdcp) **PR #5** (verify reliability)
- Parche local: drop SDCP claim on `close` (evita `AuthorizedIdentity` roto tras idle/restart)
- `provides=(libfprint)` + `conflicts=(libfprint)` → pacman posee los ficheros

Tras instalar:

```bash
fprintd-enroll -f right-index-finger
fprintd-verify
sudo -k && sudo true
```

Ver [docs/fingerprint.md](docs/fingerprint.md).

## Devtools

`mise` + Node LTS + pnpm/npm con defaults anti supply-chain. Dotfiles de usuario
(`~/.config/mise`, `~/.config/pnpm`, `~/.npmrc`), no `/etc`.

Ver [docs/devtools.md](docs/devtools.md).

## Apps

Paquetes (Cursor, Warp, Slack, Docker+Desktop, Tailscale, …) + webapps Chrome
`--app` estilo Omarchy (WhatsApp, Telegram, Linear).

Ver [docs/apps.md](docs/apps.md).

## Vicinae

Launcher + clipboard por defecto (sustituye la UX del Cosmic launcher).
**Super+Space** abre Vicinae; **Super+V** el historial de clipboard.

Ver [docs/vicinae.md](docs/vicinae.md).

## Layout

```
apply
lib/common.sh
lib/webapp.sh
bin/alex-cachyos-webapp-launch
modules/
templates/bootstrap/
templates/devtools/
templates/apps/
templates/vicinae/
profiles/
packaging/libfprint-egismoc-sdcp-git/
overlays/galaxy/etc/{pam.d,greetd}/
docs/
```
