#!/usr/bin/env bash
# Module: vicinae — Niri launcher + clipboard history.

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
  : "$tpl"

  if ! pacman -Q vicinae-bin &>/dev/null && ! pacman -Q vicinae &>/dev/null; then
    ao_has_cmd paru || ao_die "paru required (bootstrap)"
    ao_log "vicinae: installing vicinae-bin (paru — puede pedir huella)"
    paru -S --needed --noconfirm vicinae-bin
  else
    ao_log "vicinae: package already installed"
  fi

  systemctl --user enable --now vicinae.service
  ao_log "vicinae: user service enabled"

  ao_log "vicinae: done — Niri binds Super+Space to the launcher"
  ao_log "vicinae: docs: docs/vicinae.md"
}

_vicinae_remove() {
  systemctl --user disable --now vicinae.service 2>/dev/null || true
  ao_log "vicinae: service disabled (package left installed)"
}
