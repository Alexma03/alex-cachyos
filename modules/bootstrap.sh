#!/usr/bin/env bash
# Module: bootstrap — first cleanup + Chrome/paru/store/zsh on minimal CachyOS.

module_bootstrap() {
  local tpl="$AO_ROOT/templates/bootstrap"
  local remove=${AO_REMOVE:-0}

  ao_log "bootstrap: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) bootstrap stack"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _bootstrap_remove
  else
    _bootstrap_install "$tpl"
  fi
}

_bootstrap_read_pkgs() {
  local file=$1
  [[ -f $file ]] || ao_die "missing $file"
  grep -vE '^\s*(#|$)' "$file" | sed 's/[[:space:]]*$//'
}

_bootstrap_install() {
  local tpl=$1
  local work want_file rem_file ff_i18n
  local -a want_all rem_all missing_want installed_remove

  ao_has_cmd pkexec || ao_die "pkexec required (polkit)"

  mapfile -t want_all < <(_bootstrap_read_pkgs "$tpl/packages.want")
  mapfile -t rem_all < <(_bootstrap_read_pkgs "$tpl/packages.remove")

  while IFS= read -r ff_i18n; do
    [[ -n $ff_i18n ]] && rem_all+=("$ff_i18n")
  done < <(pacman -Qq 2>/dev/null | grep -E '^firefox-i18n-' || true)

  missing_want=()
  for p in "${want_all[@]}"; do
    pacman -Q "$p" &>/dev/null || missing_want+=("$p")
  done

  installed_remove=()
  for p in "${rem_all[@]}"; do
    pacman -Q "$p" &>/dev/null && installed_remove+=("$p")
  done

  work=$(mktemp -d /tmp/alex-cachyos-bootstrap.XXXXXX)
  want_file=$work/want.txt
  rem_file=$work/remove.txt
  : >"$want_file"
  : >"$rem_file"
  ((${#missing_want[@]})) && printf '%s\n' "${missing_want[@]}" >"$want_file"
  ((${#installed_remove[@]})) && printf '%s\n' "${installed_remove[@]}" >"$rem_file"

  ao_log "bootstrap: pacman want/remove + boot tweaks (pkexec — pon la huella)"
  ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'
    want_file='$want_file'
    rem_file='$rem_file'

    mapfile -t want < <(grep -vE '^\\s*\$' \"\$want_file\" || true)
    mapfile -t rem < <(grep -vE '^\\s*\$' \"\$rem_file\" || true)

    if (( \${#want[@]} )); then
      ao_log \"bootstrap: installing: \${want[*]}\"
      pacman -S --needed --noconfirm \"\${want[@]}\"
    else
      ao_log 'bootstrap: want packages already present'
    fi

    if (( \${#rem[@]} )); then
      ao_log \"bootstrap: removing: \${rem[*]}\"
      if ! pacman -Rns --noconfirm \"\${rem[@]}\" 2>/tmp/alex-cachyos-bootstrap-rm.err; then
        ao_warn 'batch remove had conflicts; retrying per-package'
        for p in \"\${rem[@]}\"; do
          pacman -Q \"\$p\" &>/dev/null || continue
          if ! pacman -Rns --noconfirm \"\$p\" 2>/dev/null; then
            ao_warn \"skip \$p (deps or protected)\"
          fi
        done
      fi
      rm -f /tmp/alex-cachyos-bootstrap-rm.err
    else
      ao_log 'bootstrap: nothing to remove'
    fi

    changed=0
    if [[ -f /etc/mkinitcpio.conf ]] && grep -qE '(^|[[:space:]])plymouth([[:space:]]|\$)' /etc/mkinitcpio.conf; then
      sed -i -E 's/(^|[[:space:]])plymouth([[:space:]])/\\1/g; s/  +/ /g' /etc/mkinitcpio.conf
      changed=1
      ao_log 'bootstrap: removed plymouth from mkinitcpio HOOKS'
    fi
    if [[ -f /etc/default/grub ]] && grep -q 'splash' /etc/default/grub; then
      sed -i -E 's/(^|[[:space:]])splash([[:space:]\"]|$)/\\1/g; s/  +/ /g' /etc/default/grub
      changed=1
      ao_log 'bootstrap: removed splash from GRUB_CMDLINE'
    fi
    if [[ \$changed -eq 1 ]]; then
      mkinitcpio -P
      if [[ -x /usr/bin/grub-mkconfig ]]; then
        grub-mkconfig -o /boot/grub/grub.cfg || ao_warn 'grub-mkconfig failed'
      fi
    fi

    if pacman -Q ananicy-cpp &>/dev/null; then
      systemctl enable --now ananicy-cpp.service
      ao_log 'bootstrap: ananicy-cpp enabled'
    fi
  "
  rm -rf "$work"

  if ! pacman -Q google-chrome &>/dev/null; then
    ao_log "bootstrap: installing google-chrome via paru (may ask fingerprint)"
    ao_has_cmd paru || ao_die "paru missing after install"
    paru -S --needed --noconfirm google-chrome
  else
    ao_log "bootstrap: google-chrome already installed"
  fi

  _bootstrap_fix_zshrc

  ao_log "bootstrap: done — login stays fish; zsh is bare for Cursor; editor=nano"
  ao_log "bootstrap: docs: docs/bootstrap.md"
}

_bootstrap_fix_zshrc() {
  local home=${HOME:?}
  local zshrc=$home/.zshrc
  python3 - "$zshrc" <<'PY'
import pathlib, sys
path = pathlib.Path(sys.argv[1])
BEGIN = "# >>> alex-cachyos/devtools >>>"
END = "# <<< alex-cachyos/devtools <<<"
devtools = ""
text = path.read_text() if path.exists() else ""
if BEGIN in text and END in text:
    _pre, rest = text.split(BEGIN, 1)
    body, _post = rest.split(END, 1)
    devtools = f"{BEGIN}\n{body.strip()}\n{END}\n"
bak = pathlib.Path(str(path) + ".bak.alex-cachyos")
if path.exists() and not bak.exists():
    bak.write_text(text)
new = (
    "# Minimal zshrc for Cursor agent (login shell is fish).\n"
    "# Managed by alex-cachyos/bootstrap.\n"
)
if devtools:
    new += "\n" + devtools
else:
    new += "\n"
if new != text:
    path.write_text(new)
    print(f"==> wrote minimal {path}")
else:
    print(f"==> {path} already minimal")
PY
}

_bootstrap_remove() {
  ao_warn "bootstrap --remove does not reinstall stock bloat"
  ao_log "bootstrap: leave google-chrome / paru / cosmic-store / zsh as-is (safe default)"
}
