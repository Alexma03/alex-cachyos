#!/usr/bin/env bash
# Module: apps — desktop packages + Chrome webapps (WhatsApp / Telegram / Linear).

module_apps() {
  local tpl="$AO_ROOT/templates/apps"
  local remove=${AO_REMOVE:-0}

  # shellcheck source=lib/webapp.sh
  source "$AO_ROOT/lib/webapp.sh"

  ao_log "apps: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) apps + webapps"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _apps_remove "$tpl"
  else
    _apps_install "$tpl"
  fi
}

_apps_read_list() {
  local file=$1
  [[ -f $file ]] || ao_die "missing $file"
  grep -vE '^\s*(#|$)' "$file" | sed 's/[[:space:]]*$//'
}

_apps_install() {
  local tpl=$1
  local work pac_file aur_file
  local -a pac aur missing_pac missing_aur
  local line name url icon user

  ao_has_cmd pkexec || ao_die "pkexec required (polkit)"
  pacman -Q google-chrome &>/dev/null || ao_die "google-chrome required (run bootstrap first)"

  mapfile -t pac < <(_apps_read_list "$tpl/packages.pacman")
  mapfile -t aur < <(_apps_read_list "$tpl/packages.aur")

  missing_pac=()
  for p in "${pac[@]}"; do
    pacman -Q "$p" &>/dev/null || missing_pac+=("$p")
  done
  missing_aur=()
  for p in "${aur[@]}"; do
    pacman -Q "$p" &>/dev/null || missing_aur+=("$p")
  done

  work=$(mktemp -d /tmp/alex-cachyos-apps.XXXXXX)
  pac_file=$work/pacman.txt
  : >"$pac_file"
  ((${#missing_pac[@]})) && printf '%s\n' "${missing_pac[@]}" >"$pac_file"

  user=$(id -un)

  ao_log "apps: pacman packages + docker/tailscale/nordvpn services (pkexec — huella)"
  ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'
    mapfile -t want < <(grep -vE '^\\s*\$' '$pac_file' || true)

    # Prefer Chrome webapps over native Telegram if present.
    if pacman -Q telegram-desktop &>/dev/null; then
      ao_log 'apps: removing telegram-desktop (using Chrome webapp instead)'
      pacman -Rns --noconfirm telegram-desktop || ao_warn 'could not remove telegram-desktop'
    fi

    if (( \${#want[@]} )); then
      ao_log \"apps: pacman -S \${want[*]}\"
      pacman -S --needed --noconfirm \"\${want[@]}\"
    else
      ao_log 'apps: pacman packages already present'
    fi

    if pacman -Q docker &>/dev/null; then
      systemctl enable --now docker.service
      usermod -aG docker '$user' || true
      ao_log 'apps: docker enabled + user in docker group'
    fi

    if pacman -Q tailscale &>/dev/null; then
      systemctl enable --now tailscaled.service
      ao_log 'apps: tailscaled enabled (run: tailscale up)'
    fi
  "
  rm -rf "$work"

  if ((${#missing_aur[@]})); then
    ao_has_cmd paru || ao_die "paru required (bootstrap)"
    ao_log "apps: paru -S ${missing_aur[*]} (puede pedir huella varias veces)"
    paru -S --needed --noconfirm "${missing_aur[@]}"
  else
    ao_log "apps: AUR packages already present"
  fi

  # Package preset enables the GUI at login; keep CLI docker.service only.
  if systemctl --user cat docker-desktop.service &>/dev/null; then
    systemctl --user disable --now docker-desktop.service 2>/dev/null || true
    ao_log "apps: docker-desktop GUI will not autostart (launch from app menu)"
  fi

  # NordVPN service after AUR install
  if pacman -Q nordvpn-bin &>/dev/null; then
    ao_log "apps: enable nordvpnd (pkexec — huella)"
    ao_root bash -c "
      set -euo pipefail
      source '$AO_ROOT/lib/common.sh'
      systemctl enable --now nordvpnd.service 2>/dev/null || systemctl enable --now nordvpn.service 2>/dev/null || ao_warn 'nordvpn service unit not found'
      usermod -aG nordvpn '$user' 2>/dev/null || true
    "
  fi

  ao_webapp_install_launcher
  while IFS='|' read -r name url icon; do
    [[ -n ${name:-} ]] || continue
    ao_webapp_install "$name" "$url" "${icon:-}"
  done < <(_apps_read_list "$tpl/webapps.list")

  # Refresh desktop database if available
  if command -v update-desktop-database >/dev/null 2>&1; then
    update-desktop-database "$HOME/.local/share/applications" 2>/dev/null || true
  fi

  ao_log "apps: done — webapps in launcher; docker group needs re-login"
  ao_log "apps: docs: docs/apps.md"
}

_apps_remove() {
  local tpl=$1
  local name url icon
  # shellcheck source=lib/webapp.sh
  source "$AO_ROOT/lib/webapp.sh"
  while IFS='|' read -r name url icon; do
    [[ -n ${name:-} ]] || continue
    ao_webapp_remove "$name"
  done < <(_apps_read_list "$tpl/webapps.list")
  ao_warn "apps --remove only drops webapp .desktop entries; packages left installed"
}
