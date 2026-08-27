# Fingerprint — Galaxy Book (`1c7a:05a1`)

## Problema

El sensor Egis/LighTuning Match-on-Chip necesita **SDCP**. Con `libfprint` stock:

1. `fprintd-enroll` parece completar
2. `fprintd-verify` / `sudo` fallan o borran la huella

Además, si se reutiliza un claim SDCP en caché tras cerrar el USB, aparece:

`SDCP AuthorizedIdentity verification failed`

## Solución (Arch-correcta)

Instalar el PKGBUILD de este repo (no `ninja install` sobre ficheros de un paquete):

```bash
./apply --profile galaxy --only fingerprint
```

Eso:

1. Construye `libfprint-egismoc-sdcp-git` (PR #5 + patch claim-on-close)
2. Lo instala con `pacman -U` (`provides`/`conflicts` stock `libfprint`)
3. Aplica overlays PAM + `cosmic-greeter.toml`

## Enroll

```bash
fprintd-delete "$USER"   # opcional, limpia metadatos locales
fprintd-enroll -f right-index-finger
fprintd-verify           # debe decir verify-match (hazlo 2 veces)
sudo -k && sudo true
```

Si el chip tenía una huella huérfana: `enroll-duplicate` → hace falta
`examples/clear-storage` del fork (documentado en el módulo si hace falta).

## Quitar

```bash
./apply --profile galaxy --only fingerprint --remove
```

Restaura backups `*.bak.alex-cachyos` y reinstala `libfprint` stock.

## Actualizaciones

Al actualizar el paquete local (rebuild del PKGBUILD), pacman sustituye la lib
de forma limpia. No uses `IgnorePkg` para esto.
