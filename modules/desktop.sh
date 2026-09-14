#!/usr/bin/env bash
# Module: desktop — Niri + Noctalia, Noctalia Greeter and Niri-native portals.

module_desktop() {
  local tpl="$AO_ROOT/templates"
  local overlay="$AO_ROOT/overlays/${AO_OVERLAY_PROFILE:-galaxy}/etc"
  local remove=${AO_REMOVE:-0}

  ao_log "desktop: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) the Niri-only desktop"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _desktop_remove
  else
    _desktop_install "$tpl" "$overlay"
  fi
}

_desktop_read_list() {
  local file=$1
  [[ -f $file ]] || ao_die "missing $file"
  grep -vE '^\s*(#|$)' "$file" | sed 's/[[:space:]]*$//'
}

_desktop_legacy_packages() {
  cat <<'EOF'
cosmic-applets
cosmic-app-library
cosmic-bg
cosmic-comp
cosmic-files
cosmic-greeter
cosmic-icon-theme
cosmic-idle
cosmic-launcher
cosmic-monitor
cosmic-notifications
cosmic-osd
cosmic-panel
cosmic-player
cosmic-randr
cosmic-screenshot
cosmic-session
cosmic-settings
cosmic-settings-daemon
cosmic-sound-theme
cosmic-store
cosmic-terminal
cosmic-text-editor
cosmic-wallpapers
cosmic-workspaces
pop-icon-theme
pop-launcher
xdg-desktop-portal-cosmic
EOF
}

_desktop_install() {
  local tpl=$1 overlay=$2
  local home=${HOME:?}
  local user work pac_file noctalia_tmp
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

  # Keep the working graphical agent alive while package installation prompts.
  _desktop_stop_other_polkit_agents
  _desktop_ensure_quickshell_polkit || ao_warn "desktop: graphical polkit agent is not running"

  if ((${#missing[@]})); then
    ao_log "desktop: installing the complete Niri runtime (pkexec — huella)"
    ao_root bash -c "
      set -euo pipefail
      mapfile -t want < <(grep -vE '^\\s*\$' '$pac_file' || true)
      pacman -S --needed --noconfirm \"\${want[@]}\"
    "
  else
    ao_log "desktop: Niri runtime packages already present"
  fi

  ao_install_user_file "$tpl/niri/config.kdl" "$home/.config/niri/config.kdl"

  noctalia_tmp=$work/noctalia-config.toml
  sed "s|@HOME@|$home|g" "$tpl/noctalia/config.toml" >"$noctalia_tmp"
  ao_install_user_file "$noctalia_tmp" "$home/.config/noctalia/config.toml"
  # settings.toml is Noctalia-owned mutable state and overrides declarative config.
  # Removing it makes every apply converge to templates/noctalia/config.toml.
  rm -f "$home/.local/state/noctalia/settings.toml" \
        "$home/.local/state/noctalia/settings.toml.bak.alex-cachyos"

  ao_install_user_file "$tpl/hyprwhspr/config.json" "$home/.config/hyprwhspr/config.json"
  ao_install_user_file "$tpl/quickshell-polkit/shell.qml" "$home/.config/quickshell/polkit/shell.qml"
  ao_install_user_file "$tpl/quickshell-polkit/PolkitModel.js" "$home/.config/quickshell/polkit/PolkitModel.js"

  printf '%s\n' '[Desktop]' 'Session=niri' >"$home/.dmrc"

  ao_log "desktop: configuring greetd and removing the old desktop stack (pkexec — huella)"
  ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'
    source '$AO_ROOT/modules/desktop.sh'
    _desktop_root_configure '$user' '$overlay' '$AO_ROOT'
  "

  # User-owned remnants from prior desktop experiments are not inputs to Niri.
  rm -rf "$home/.cache/cosmic-settings" \
         "$home/.cache/cosmic-store" \
         "$home/.config/cosmic" \
         "$home/.config/dconf/cosmic" \
         "$home/.config/hypr" \
         "$home/.config/hyprpolkitagent" \
         "$home/.local/state/cosmic" \
         "$home/.local/state/cosmic-comp"
  rm -f "$home/.local/state/pop-launcher/cosmic-toplevel.log"
  if ao_has_cmd flatpak && flatpak remotes --user --columns=name 2>/dev/null | grep -Fxq cosmic; then
    flatpak remote-delete --user --force cosmic
  fi
  rm -rf "$home/.local/share/flatpak/appstream/cosmic" \
         "$home/.local/share/flatpak/repo/refs/remotes/cosmic"
  rm -f "$home/.local/share/flatpak/repo/cosmic.trustedkeys.gpg"
  if [[ -d $home/.local/share/flatpak/repo/tmp/cache/summaries ]]; then
    find "$home/.local/share/flatpak/repo/tmp/cache/summaries" -maxdepth 1 \
      -type f -name 'cosmic*' -delete
  fi

  niri validate --config "$home/.config/niri/config.kdl"
  noctalia config validate "$home/.config/noctalia/config.toml"

  if pacman -Q hyprwhspr &>/dev/null; then
    ao_log "desktop: installing noctwhspr plugin + user service"
    hyprwhspr noctalia install
    systemctl --user enable --now hyprwhspr.service
  else
    ao_warn "desktop: hyprwhspr missing — run the apps module"
  fi

  systemctl --user enable --now vicinae.service 2>/dev/null || true
  _desktop_ensure_quickshell_polkit || ao_die "desktop: Quickshell polkit agent failed to start"

  if noctalia msg status &>/dev/null; then
    noctalia msg plugins enable goodroot/noctwhspr
    noctalia msg plugins enable felipeartur/ai-usagebar
    noctalia msg config-reload
  else
    ao_log "desktop: Noctalia will load the declarative config at next Niri login"
  fi

  # Re-select portal implementations after replacing the old backend.
  systemctl --user restart xdg-desktop-portal.service 2>/dev/null || true

  rm -rf "$work"

  ao_log "desktop: done — Niri is the only installed graphical session"
  ao_log "desktop: reboot once to switch the running greeter process to Noctalia Greeter"
}

_desktop_root_configure() {
  local user=$1 overlay=$2 repo=$3
  local as state_dir
  local -a legacy installed

  [[ -x /usr/bin/noctalia-greeter-session ]] || ao_die "noctalia-greeter-session missing"
  getent passwd greeter >/dev/null || ao_die "greetd greeter user missing"

  _desktop_restore_legacy_pam

  # Protect every package declared by the repository before pruning the old
  # desktop's dependency tree.
  ao_pacman_mark_explicit_files \
    "$repo/templates/bootstrap/packages.want" \
    "$repo/templates/apps/packages.pacman" \
    "$repo/templates/apps/packages.aur" \
    "$repo/templates/desktop/packages.pacman"

  ao_install_file "$overlay/greetd/config.toml" /etc/greetd/config.toml
  ao_install_file "$overlay/pam.d/alex-cachyos-login" /etc/pam.d/alex-cachyos-login

  state_dir=/var/lib/noctalia-greeter
  install -d -m 0750 -o greeter -g greeter "$state_dir"
  install -m 0644 -o greeter -g greeter \
    "$overlay/noctalia-greeter/greeter.toml" "$state_dir/greeter.toml"

  as=/var/lib/AccountsService/users/$user
  install -d -m 0755 /var/lib/AccountsService/users
  if [[ -f $as ]]; then
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

  # Change the boot-time alias without stopping the greeter that owns this login.
  systemctl disable cosmic-greeter.service 2>/dev/null || true
  systemctl enable --force greetd.service

  mapfile -t legacy < <(_desktop_legacy_packages)
  installed=()
  for p in "${legacy[@]}"; do
    pacman -Q "$p" &>/dev/null && installed+=("$p")
  done
  if ((${#installed[@]})); then
    pacman -Rns --noconfirm "${installed[@]}"
  fi

  if command -v flatpak >/dev/null 2>&1 \
     && flatpak remotes --system --columns=name 2>/dev/null | grep -Fxq cosmic; then
    flatpak remote-delete --system --force cosmic
  fi
  rm -rf /var/lib/flatpak/appstream/cosmic \
         /var/lib/flatpak/repo/refs/remotes/cosmic
  rm -f /var/lib/flatpak/repo/cosmic.trustedkeys.gpg
  if [[ -d /var/lib/flatpak/repo/tmp/cache/summaries ]]; then
    find /var/lib/flatpak/repo/tmp/cache/summaries -maxdepth 1 \
      -type f -name 'cosmic*' -delete
  fi

  rm -f /etc/environment.d/99-vicinae-cosmic.conf \
        /etc/greetd/cosmic-greeter.toml \
        /etc/greetd/cosmic-greeter.toml.bak.alex-cachyos \
        /etc/pam.d/cosmic-greeter \
        /etc/pam.d/cosmic-greeter.bak.alex-cachyos
  if getent passwd cosmic-greeter >/dev/null && ! pgrep -u cosmic-greeter >/dev/null; then
    userdel -r cosmic-greeter 2>/dev/null || true
  fi
  rm -rf /var/lib/cosmic-greeter

  # Cached archives are not rollback state (Snapper holds that) and would be
  # the last machine-local copies of the removed desktop stack.
  find /var/cache/pacman/pkg -maxdepth 1 -type f \
    \( -name 'cosmic-*.pkg.tar.*' -o -name 'xdg-desktop-portal-cosmic-*.pkg.tar.*' \) \
    -delete
  systemctl daemon-reload

  [[ $(readlink -f /etc/systemd/system/display-manager.service) == /usr/lib/systemd/system/greetd.service ]] \
    || ao_die "display-manager.service does not point to greetd"
}

# Previous versions replaced package-owned PAM files wholesale. Restore their
# current package versions before installing the dedicated login PAM service.
_desktop_restore_legacy_pam() {
  local work pkg member dst archive spec
  local -a specs=(
    'greetd:etc/pam.d/greetd:/etc/pam.d/greetd'
    'pambase:etc/pam.d/system-local-login:/etc/pam.d/system-local-login'
    'util-linux:etc/pam.d/su:/etc/pam.d/su'
    'util-linux:etc/pam.d/su-l:/etc/pam.d/su-l'
  )

  [[ -e /etc/pam.d/greetd.bak.alex-cachyos \
     || -e /etc/pam.d/system-local-login.bak.alex-cachyos \
     || -e /etc/pam.d/su.bak.alex-cachyos \
     || -e /etc/pam.d/su-l.bak.alex-cachyos ]] || return 0

  work=$(mktemp -d /tmp/alex-cachyos-pam.XXXXXX)
  # pacman drops privileges to DownloadUser=alpm for network transfers.  The
  # staged files live in an alpm-owned child, but that user must be able to
  # traverse the mktemp parent (created as 0700 by default).
  chmod 0755 "$work"
  for spec in "${specs[@]}"; do
    IFS=: read -r pkg member dst <<<"$spec"
    pacman -Sw --noconfirm --cachedir "$work" "$pkg" >/dev/null
    archive=$(find "$work" -maxdepth 1 -type f \
      -name "$pkg-*.pkg.tar.*" ! -name '*.sig' -print -quit)
    [[ -n $archive ]] || ao_die "could not stage $pkg to restore $dst"
    bsdtar -tf "$archive" | grep -Fxq "$member" \
      || ao_die "$archive does not contain $member"
    bsdtar -xOf "$archive" "$member" | install -D -m 0644 /dev/stdin "$dst"
  done
  rm -rf "$work"
  rm -f /etc/pam.d/greetd.bak.alex-cachyos \
        /etc/pam.d/system-local-login.bak.alex-cachyos \
        /etc/pam.d/su.bak.alex-cachyos \
        /etc/pam.d/su-l.bak.alex-cachyos
}

# Keep one graphical authentication agent: the small Quickshell UI in this repo.
_desktop_stop_other_polkit_agents() {
  systemctl --user disable --now hyprpolkitagent.service 2>/dev/null || true
  pkill -f 'polkit-kde-authentication-agent-1' 2>/dev/null || true
  pkill -f 'polkit-gnome-authentication-agent-1' 2>/dev/null || true
  pkill -x hyprpolkitagent 2>/dev/null || true
  pkill -f 'lxqt-policykit-agent' 2>/dev/null || true
}

_desktop_ensure_quickshell_polkit() {
  _desktop_stop_other_polkit_agents
  if ! command -v qs >/dev/null && ! command -v quickshell >/dev/null; then
    return 1
  fi
  pkill -f 'qs -c polkit' 2>/dev/null || true
  sleep 0.2
  if command -v qs >/dev/null; then
    qs -c polkit -n -d || return 1
  else
    quickshell -c polkit -n -d || return 1
  fi
  sleep 0.3
}

_desktop_remove() {
  local home=${HOME:?}
  ao_restore_user_file "$home/.config/niri/config.kdl"
  ao_restore_user_file "$home/.config/noctalia/config.toml"
  ao_restore_user_file "$home/.config/hyprwhspr/config.json"
  ao_restore_user_file "$home/.config/quickshell/polkit/shell.qml"
  ao_restore_user_file "$home/.config/quickshell/polkit/PolkitModel.js"
  pkill -f 'qs -c polkit' 2>/dev/null || true
  systemctl --user disable --now hyprwhspr.service 2>/dev/null || true
  ao_warn "desktop --remove restores user configs; it keeps the safe Niri boot path installed"
}
