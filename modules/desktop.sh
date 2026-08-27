#!/usr/bin/env bash
# Module: desktop — Niri + Noctalia as the default session.
# Login greeter stays cosmic-greeter; COSMIC desktop is optional in the session list only.
# pkexec UI: Quickshell Omarchy-style agent only (templates/quickshell-polkit).

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

  # Only the Quickshell polkit UI — kill competing agents first.
  _desktop_stop_other_polkit_agents
  _desktop_ensure_quickshell_polkit || ao_warn "desktop: no polkit agent yet — first package install needs sudo/pkexec in a real terminal (huella ahí)"

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
  mkdir -p "$home/.config/quickshell/polkit"
  ao_install_user_file "$tpl/quickshell-polkit/shell.qml" "$home/.config/quickshell/polkit/shell.qml"
  ao_install_user_file "$tpl/quickshell-polkit/PolkitModel.js" "$home/.config/quickshell/polkit/PolkitModel.js"

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

  _desktop_ensure_quickshell_polkit || true

  if command -v noctalia >/dev/null; then
    if noctalia msg status &>/dev/null; then
      noctalia msg plugins enable goodroot/noctwhspr >/dev/null || true
      noctalia msg plugins enable felipeartur/ai-usagebar >/dev/null || true
      noctalia msg config-reload >/dev/null || true
    else
      ao_log "desktop: noctalia not running — plugins enable on next niri login from settings.toml"
    fi
  fi

  ao_log "desktop: done — Niri is the default session; COSMIC remains only as a greeter option"
  ao_log "desktop: docs: docs/desktop.md"
}

# Kill every graphical polkit agent that is not our Quickshell UI.
_desktop_stop_other_polkit_agents() {
  systemctl --user disable --now hyprpolkitagent.service 2>/dev/null || true
  pkill -f 'polkit-kde-authentication-agent-1' 2>/dev/null || true
  pkill -f 'polkit-gnome-authentication-agent-1' 2>/dev/null || true
  pkill -x hyprpolkitagent 2>/dev/null || true
  pkill -f 'lxqt-policykit-agent' 2>/dev/null || true
}

# Install/reload Omarchy-style Quickshell polkit; keep Noctalia's agent off.
_desktop_ensure_quickshell_polkit() {
  _desktop_stop_other_polkit_agents
  if ! command -v qs >/dev/null && ! command -v quickshell >/dev/null; then
    return 1
  fi
  # settings.toml template has polkit_agent = false; reload if shell is up.
  if command -v noctalia >/dev/null && noctalia msg status &>/dev/null; then
    noctalia msg config-reload >/dev/null || true
  fi
  pkill -f 'qs -c polkit' 2>/dev/null || true
  sleep 0.2
  if command -v qs >/dev/null; then
    ao_log "desktop: starting Quickshell polkit agent (qs -c polkit)"
    qs -c polkit -n -d || return 1
  else
    ao_log "desktop: starting Quickshell polkit agent"
    quickshell -c polkit -n -d || return 1
  fi
  sleep 0.3
  return 0
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
    # Name= from niri.desktop (cosmic-greeter last_session uses the display Name).
    cat >"$users_file" <<EOF
{
    $uid: (
        uid: $uid,
        last_session: Some("Niri"),
    ),
}
EOF
    printf 'Some(%s)\n' "$uid" >"$gdir/last_user"
    chown -R cosmic-greeter:cosmic-greeter /var/lib/cosmic-greeter/.config 2>/dev/null || true
    ao_log "desktop: cosmic-greeter last_session=Niri"
  fi
}

_desktop_remove() {
  local home=${HOME:?}
  ao_restore_user_file "$home/.config/niri/config.kdl"
  ao_restore_user_file "$home/.local/state/noctalia/settings.toml"
  ao_restore_user_file "$home/.config/hyprwhspr/config.json"
  ao_restore_user_file "$home/.config/quickshell/polkit/shell.qml"
  ao_restore_user_file "$home/.config/quickshell/polkit/PolkitModel.js"
  pkill -f 'qs -c polkit' 2>/dev/null || true
  rm -f "$home/.dmrc"
  systemctl --user disable --now hyprwhspr.service 2>/dev/null || true
  ao_warn "desktop --remove restores config backups; leaves niri/noctalia packages and greeter last_session"
}
