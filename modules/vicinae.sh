#!/usr/bin/env bash
# Module: vicinae — default launcher + clipboard (replaces Cosmic launcher UX).

module_vicinae() {
  local tpl="$AO_ROOT/templates/vicinae"
  local remove=${AO_REMOVE:-0}

  ao_log "vicinae: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) vicinae"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _vicinae_remove
  else
    _vicinae_install "$tpl"
  fi
}

_vicinae_install() {
  local tpl=$1
  local home=${HOME:?}
  local shortcuts=$home/.config/cosmic/com.system76.CosmicSettings.Shortcuts/v1
  local stock=/usr/share/cosmic/com.system76.CosmicSettings.Shortcuts/v1/system_actions

  ao_has_cmd pkexec || ao_die "pkexec required (polkit)"

  if ! pacman -Q vicinae-bin &>/dev/null && ! pacman -Q vicinae &>/dev/null; then
    ao_has_cmd paru || ao_die "paru required (bootstrap)"
    ao_log "vicinae: installing vicinae-bin (paru — puede pedir huella)"
    paru -S --needed --noconfirm vicinae-bin
  else
    ao_log "vicinae: package already installed"
  fi

  ao_log "vicinae: COSMIC clipboard env (pkexec — huella)"
  ao_root install -D -m 644 "$tpl/99-vicinae-cosmic.conf" /etc/environment.d/99-vicinae-cosmic.conf

  mkdir -p "$shortcuts"
  if [[ -f $stock ]]; then
    if [[ -f $shortcuts/system_actions && ! -f $shortcuts/system_actions.bak.alex-cachyos ]]; then
      cp -a "$shortcuts/system_actions" "$shortcuts/system_actions.bak.alex-cachyos"
    elif [[ ! -f $shortcuts/system_actions ]]; then
      : # no prior user file
    fi
    cp -a "$stock" "$shortcuts/system_actions"
    sed -i 's|Launcher: "cosmic-launcher",|Launcher: "vicinae toggle",|' "$shortcuts/system_actions"
    ao_log "vicinae: Cosmic Launcher action → vicinae toggle"
  else
    ao_warn "missing $stock — skip system_actions override"
  fi

  ao_install_user_file "$tpl/cosmic-shortcuts-custom" "$shortcuts/custom"

  systemctl --user enable --now vicinae.service
  ao_log "vicinae: user service enabled"

  ao_log "vicinae: done — Super+Space launcher, Super+V clipboard, Super alone disabled"
  ao_log "vicinae: logout/login once so COSMIC_DATA_CONTROL_ENABLED applies if clipboard was empty"
  ao_log "vicinae: docs: docs/vicinae.md"
}

_vicinae_remove() {
  local home=${HOME:?}
  local shortcuts=$home/.config/cosmic/com.system76.CosmicSettings.Shortcuts/v1

  systemctl --user disable --now vicinae.service 2>/dev/null || true
  ao_restore_user_file "$shortcuts/custom"
  ao_restore_user_file "$shortcuts/system_actions"
  ao_log "vicinae: enable --remove does not uninstall the package or /etc/environment.d (safe default)"
}
