#!/usr/bin/env bash
# Module: desktop — Niri + Noctalia as the default session (COSMIC stays installed).

module_desktop() {
  local tpl="$AO_ROOT/templates"
  local remove=${AO_REMOVE:-0}

  ao_log "desktop: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) niri + noctalia + default session"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _desktop_remove
  else
    _desktop_install "$tpl"
  fi
}

_desktop_read_list() {
  local file=$1
  [[ -f $file ]] || ao_die "missing $file"
  grep -vE '^\s*(#|$)' "$file" | sed 's/[[:space:]]*$//'
}

_desktop_install() {
  local tpl=$1
  local home=${HOME:?}
  local user work pac_file
  local -a pac missing

  ao_has_cmd pkexec || ao_die "pkexec required (polkit)"

  mapfile -t pac < <(_desktop_read_list "$tpl/desktop/packages.pacman")
  missing=()
  for p in "${pac[@]}"; do
    pacman -Q "$p" &>/dev/null || missing+=("$p")
  done

  work=$(mktemp -d /tmp/alex-cachyos-desktop.XXXXXX)
  pac_file=$work/pacman.txt
  : >"$pac_file"
  ((${#missing[@]})) && printf '%s\n' "${missing[@]}" >"$pac_file"
  user=$(id -un)

  if ((${#missing[@]})); then
    ao_log "desktop: niri/noctalia packages (pkexec — huella)"
    ao_root bash -c "
      set -euo pipefail
      source '$AO_ROOT/lib/common.sh'
      mapfile -t want < <(grep -vE '^\\s*\$' '$pac_file' || true)
      ao_log \"desktop: pacman -S \${want[*]}\"
      pacman -S --needed --noconfirm \"\${want[@]}\"
    "
  else
    ao_log "desktop: niri/noctalia already present"
  fi
  rm -rf "$work"

  ao_install_user_file "$tpl/niri/config.kdl" "$home/.config/niri/config.kdl"
  mkdir -p "$home/.local/state/noctalia"
  ao_install_user_file "$tpl/noctalia/settings.toml" "$home/.local/state/noctalia/settings.toml"
  mkdir -p "$home/.config/hyprwhspr"
  ao_install_user_file "$tpl/hyprwhspr/config.json" "$home/.config/hyprwhspr/config.json"

  printf '%s\n' '[Desktop]' 'Session=niri' >"$home/.dmrc"
  ao_log "desktop: ~/.dmrc Session=niri"

  ao_log "desktop: default session in greeter (pkexec — huella)"
  if ! ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'
    source '$AO_ROOT/modules/desktop.sh'
    _desktop_root_default_session '$user'
  "; then
    ao_warn "desktop: greeter last_session not written (pkexec failed); ~/.dmrc is niri"
  fi

  if command -v niri >/dev/null && [[ -f $home/.config/niri/config.kdl ]]; then
    niri validate --config "$home/.config/niri/config.kdl" >/dev/null
  fi

  if pacman -Q hyprwhspr &>/dev/null; then
    ao_log "desktop: hyprwhspr noctalia plugin + user service"
    hyprwhspr noctalia install
    systemctl --user enable --now hyprwhspr.service
  else
    ao_warn "desktop: hyprwhspr not installed — run apps module for local STT + noctwhspr"
  fi

  if command -v noctalia >/dev/null; then
    if noctalia msg status &>/dev/null; then
      noctalia msg plugins enable salemsayed/codexbar-meter >/dev/null || true
      noctalia msg plugins enable goodroot/noctwhspr >/dev/null || true
      noctalia msg plugins enable frai3mega/battery-graph >/dev/null || true
      noctalia msg config-reload >/dev/null || true
    else
      ao_log "desktop: noctalia not running — plugins enable on next niri login from settings.toml"
    fi
  fi

  ao_log "desktop: done — Niri is the default session; COSMIC remains as a greeter option"
  ao_log "desktop: docs: docs/desktop.md"
}

# Runs as root (inside ao_root). Prefer Niri in AccountsService + cosmic-greeter last_session.
_desktop_root_default_session() {
  local user=$1
  local uid as gdir users_file
  uid=$(id -u "$user")
  as=/var/lib/AccountsService/users/$user
  mkdir -p /var/lib/AccountsService/users
  if [[ -f $as ]]; then
    grep -q '^\[User\]' "$as" || sed -i '1i[User]' "$as"
    if grep -q '^Session=' "$as"; then
      sed -i 's/^Session=.*/Session=niri/' "$as"
    else
      printf '\nSession=niri\n' >>"$as"
    fi
    if grep -q '^XSession=' "$as"; then
      sed -i 's/^XSession=.*/XSession=niri/' "$as"
    else
      printf 'XSession=niri\n' >>"$as"
    fi
  else
    printf '%s\n' '[User]' 'Session=niri' 'XSession=niri' 'SystemAccount=false' >"$as"
  fi
  ao_log "desktop: AccountsService Session=niri for $user"

  gdir=/var/lib/cosmic-greeter/.config/cosmic/com.system76.CosmicGreeter/v1
  if [[ -d /var/lib/cosmic-greeter ]]; then
    mkdir -p "$gdir"
    users_file=$gdir/users
    if [[ -f $users_file && ! -f ${users_file}.bak.alex-cachyos ]]; then
      cp -a "$users_file" "${users_file}.bak.alex-cachyos"
    fi
    cat >"$users_file" <<EOF
{
    $uid: (
        uid: $uid,
        last_session: Some("niri"),
    ),
}
EOF
    printf 'Some(%s)\n' "$uid" >"$gdir/last_user"
    chown -R cosmic-greeter:cosmic-greeter /var/lib/cosmic-greeter/.config 2>/dev/null || true
    ao_log "desktop: cosmic-greeter last_session=niri"
  fi
}

_desktop_remove() {
  local home=${HOME:?}
  ao_restore_user_file "$home/.config/niri/config.kdl"
  ao_restore_user_file "$home/.local/state/noctalia/settings.toml"
  ao_restore_user_file "$home/.config/hyprwhspr/config.json"
  rm -f "$home/.dmrc"
  systemctl --user disable --now hyprwhspr.service 2>/dev/null || true
  ao_warn "desktop --remove restores config backups; leaves niri/noctalia packages and greeter last_session"
}
