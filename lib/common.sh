#!/usr/bin/env bash
# Shared helpers for alex-cachyos apply.

ao_log()  { printf '==> %s\n' "$*"; }
ao_warn() { printf '!!  %s\n' "$*" >&2; }
ao_die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

ao_has_cmd() { command -v "$1" >/dev/null 2>&1; }

ao_need_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    ao_die "this step needs root (re-run with sudo/pkexec, or from apply which elevates)"
  fi
}

# Elevate via pkexec so polkit/fingerprint GUI can prompt.
ao_root() {
  if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
    "$@"
  else
    ao_has_cmd pkexec || ao_die "pkexec required (polkit)"
    pkexec "$@"
  fi
}

# Backup once, then install file from overlay.
ao_install_file() {
  local src=$1 dst=$2
  [[ -f $src ]] || ao_die "missing overlay file: $src"
  if [[ -f $dst && ! -f ${dst}.bak.alex-cachyos ]]; then
    cp -a "$dst" "${dst}.bak.alex-cachyos"
  fi
  install -D -m 644 "$src" "$dst"
  ao_log "installed $dst"
}

# Restore backup if present, else remove our unmanaged file.
ao_restore_file() {
  local dst=$1
  if [[ -f ${dst}.bak.alex-cachyos ]]; then
    mv -f "${dst}.bak.alex-cachyos" "$dst"
    ao_log "restored $dst"
  elif [[ -f $dst ]]; then
    # Only remove files we created that no package owns? Safer: leave and warn.
    ao_warn "no backup for $dst — left in place"
  fi
}

# User-home install (same backup scheme; no root).
ao_install_user_file() {
  local src=$1 dst=$2
  [[ -f $src ]] || ao_die "missing template: $src"
  mkdir -p "$(dirname "$dst")"
  if [[ -f $dst && ! -f ${dst}.bak.alex-cachyos ]]; then
    cp -a "$dst" "${dst}.bak.alex-cachyos"
  fi
  install -D -m 644 "$src" "$dst"
  ao_log "installed $dst"
}

ao_restore_user_file() {
  ao_restore_file "$1"
}

ao_should_run_module() {
  local name=$1
  local default_enabled=$2
  if [[ -n ${AO_ONLY:-} ]]; then
    [[ ",${AO_ONLY}," == *",${name},"* ]] && return 0
    return 1
  fi
  local forced
  for forced in "${AO_WITH[@]:-}"; do
    [[ $forced == "$name" ]] && return 0
  done
  for forced in "${AO_WITHOUT[@]:-}"; do
    [[ $forced == "$name" ]] && return 1
  done
  [[ $default_enabled -eq 1 ]]
}
