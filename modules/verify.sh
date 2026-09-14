#!/usr/bin/env bash
# Module: verify — post-apply convergence, package, session and boot checks.

module_verify() {
  local home=${HOME:?}
  local p file expected actual
  local -a required legacy leftovers sessions
  AO_VERIFY_FAILURES=0
  AO_VERIFY_WARNINGS=0

  if [[ ${AO_REMOVE:-0} -eq 1 ]]; then
    ao_warn "verify: skipped in remove mode"
    return 0
  fi
  if [[ ${AO_DRY_RUN:-0} -eq 1 ]]; then
    ao_log "DRY: would verify packages, configs, services, PAM, portals and boot files"
    return 0
  fi

  ao_log "verify: checking the converged Niri system"

  niri validate --config "$home/.config/niri/config.kdl" \
    && _verify_ok "Niri config validates" \
    || _verify_fail "Niri config is invalid"
  noctalia config validate \
    && _verify_ok "Noctalia merged config validates" \
    || _verify_fail "Noctalia config is invalid"

  expected=$(mktemp)
  sed "s|@HOME@|$home|g" "$AO_ROOT/templates/noctalia/config.toml" >"$expected"
  cmp -s "$AO_ROOT/templates/niri/config.kdl" "$home/.config/niri/config.kdl" \
    && _verify_ok "Niri config matches the repository" \
    || _verify_fail "Niri config drifted from the repository"
  cmp -s "$expected" "$home/.config/noctalia/config.toml" \
    && _verify_ok "Noctalia config matches the repository" \
    || _verify_fail "Noctalia config drifted from the repository"
  rm -f "$expected"

  if ao_has_cmd mise && [[ $(mise exec -- pnpm --version 2>/dev/null) =~ ^1[12]\. ]]; then
    _verify_ok "pnpm is selected by mise"
  else
    _verify_fail "mise is not selecting pnpm"
  fi
  if [[ $(PNPM_CONFIG_GLOBAL_BIN_DIR="$home/.local/bin" \
            mise exec -- pnpm config get global-bin-dir 2>/dev/null | tail -n 1) == "$home/.local/bin" ]]; then
    _verify_ok "pnpm global executables target ~/.local/bin"
  else
    _verify_fail "pnpm global executables do not target ~/.local/bin"
  fi

  required=()
  for file in \
    "$AO_ROOT/templates/bootstrap/packages.want" \
    "$AO_ROOT/templates/apps/packages.pacman" \
    "$AO_ROOT/templates/apps/packages.aur" \
    "$AO_ROOT/templates/desktop/packages.pacman"; do
    while IFS= read -r p; do required+=("$p"); done < <(grep -vE '^\s*(#|$)' "$file")
  done
  required+=(google-chrome libfprint-egismoc-sdcp-git fprintd vicinae-bin)
  for p in "${required[@]}"; do
    pacman -Q "$p" &>/dev/null || _verify_fail "missing package: $p"
  done
  [[ $AO_VERIFY_FAILURES -gt 0 ]] || _verify_ok "all declared packages are installed"

  mapfile -t legacy < <(_desktop_legacy_packages)
  leftovers=()
  for p in "${legacy[@]}"; do pacman -Q "$p" &>/dev/null && leftovers+=("$p"); done
  if ((${#leftovers[@]})); then
    _verify_fail "legacy desktop packages remain: ${leftovers[*]}"
  else
    _verify_ok "no legacy desktop packages are installed"
  fi

  leftovers=()
  for file in \
    "$home/.cache/cosmic-settings" \
    "$home/.cache/cosmic-store" \
    "$home/.config/cosmic" \
    "$home/.config/dconf/cosmic" \
    "$home/.local/state/cosmic" \
    "$home/.local/state/cosmic-comp" \
    "$home/.local/state/pop-launcher/cosmic-toplevel.log"; do
    [[ -e $file ]] && leftovers+=("$file")
  done
  if ((${#leftovers[@]})); then
    _verify_fail "legacy user state remains: ${leftovers[*]}"
  elif ao_has_cmd flatpak \
       && flatpak remotes --user --columns=name 2>/dev/null | grep -Fxq cosmic; then
    _verify_fail "the legacy user Flatpak remote remains"
  elif [[ -e $home/.local/share/flatpak/appstream/cosmic \
        || -e $home/.local/share/flatpak/repo/refs/remotes/cosmic \
        || -e $home/.local/share/flatpak/repo/cosmic.trustedkeys.gpg ]] \
       || [[ -d $home/.local/share/flatpak/repo/tmp/cache/summaries \
          && -n $(find "$home/.local/share/flatpak/repo/tmp/cache/summaries" \
                    -maxdepth 1 -type f -name 'cosmic*' -print -quit 2>/dev/null) ]]; then
    _verify_fail "legacy user Flatpak cache state remains"
  else
    _verify_ok "no machine-local legacy user configuration remains"
  fi

  if getent passwd cosmic-greeter >/dev/null \
     || [[ -e /etc/pam.d/cosmic-greeter || -e /var/lib/cosmic-greeter ]]; then
    _verify_fail "legacy greeter account or system state remains"
  else
    _verify_ok "no legacy greeter account or PAM state remains"
  fi

  [[ $(readlink -f /etc/systemd/system/display-manager.service) == /usr/lib/systemd/system/greetd.service ]] \
    && _verify_ok "greetd is the boot display manager" \
    || _verify_fail "display-manager.service is not greetd"
  systemctl is-enabled --quiet greetd.service \
    && _verify_ok "greetd is enabled" \
    || _verify_fail "greetd is not enabled"
  [[ -x /usr/bin/noctalia-greeter-session ]] \
    && _verify_ok "Noctalia Greeter is installed" \
    || _verify_fail "Noctalia Greeter executable is missing"
  [[ -f /usr/share/wayland-sessions/niri.desktop ]] \
    && _verify_ok "Niri session is registered" \
    || _verify_fail "Niri session file is missing"
  sessions=()
  for file in /usr/share/wayland-sessions/*.desktop /usr/share/xsessions/*.desktop; do
    [[ -e $file ]] || continue
    [[ $(basename "$file") == niri.desktop ]] || sessions+=("$file")
  done
  if ((${#sessions[@]})); then
    _verify_fail "additional graphical sessions remain: ${sessions[*]}"
  else
    _verify_ok "Niri is the only registered graphical session"
  fi

  for p in xdg-desktop-portal-gnome xdg-desktop-portal-gtk gnome-keyring; do
    pacman -Q "$p" &>/dev/null || _verify_fail "Niri integration missing: $p"
  done
  systemctl --user is-active --quiet xdg-desktop-portal.service \
    && _verify_ok "the desktop portal broker is running" \
    || _verify_fail "the desktop portal broker is not running"

  cmp -s "$AO_ROOT/overlays/${AO_OVERLAY_PROFILE:-galaxy}/etc/pam.d/alex-cachyos-login" \
    /etc/pam.d/alex-cachyos-login \
    && _verify_ok "the dedicated greeter PAM policy matches the repository" \
    || _verify_fail "the dedicated greeter PAM policy drifted"

  systemctl --user is-enabled --quiet vicinae.service \
    && _verify_ok "Vicinae service is enabled" \
    || _verify_fail "Vicinae service is not enabled"
  systemctl --user is-enabled --quiet hyprwhspr.service \
    && _verify_ok "hyprwhspr service is enabled" \
    || _verify_fail "hyprwhspr service is not enabled"
  pgrep -f 'qs -c polkit' >/dev/null \
    && _verify_ok "the Quickshell polkit agent is running" \
    || _verify_fail "the Quickshell polkit agent is not running"

  fprintd-list "$(id -un)" >/dev/null 2>&1 \
    && _verify_ok "fingerprint enrollment is present" \
    || _verify_warn "no enrolled fingerprint was detected"

  pacman -Dk >/dev/null \
    && _verify_ok "pacman database is consistent" \
    || _verify_fail "pacman database consistency check failed"
  ao_root pacman -Qk >/dev/null \
    && _verify_ok "installed packages have no missing files" \
    || _verify_fail "one or more package-owned files are missing"

  if systemctl --failed --no-legend | grep -q .; then
    _verify_fail "system services are failed"
  else
    _verify_ok "no system service is failed"
  fi
  if systemctl --user --failed --no-legend | grep -q .; then
    _verify_fail "user services are failed"
  else
    _verify_ok "no user service is failed"
  fi

  ao_root bash -c '
    set -euo pipefail
    test -s /boot/vmlinuz-linux-cachyos
    test -s /boot/initramfs-linux-cachyos.img
    grub-script-check /boot/grub/grub.cfg
    systemd-analyze verify greetd.service
  ' && _verify_ok "kernel, initramfs, GRUB and greetd unit validate" \
    || _verify_fail "boot artifact validation failed"

  if [[ -n $(find /etc -xdev -type f -name '*.pacnew' -print -quit 2>/dev/null) ]]; then
    _verify_warn "unmerged .pacnew files exist; review with pacdiff"
  fi

  if systemctl is-active --quiet cosmic-greeter.service 2>/dev/null; then
    _verify_warn "the previous greeter process remains alive for this login; reboot will replace it"
  fi

  if ((AO_VERIFY_FAILURES)); then
    ao_die "verify: $AO_VERIFY_FAILURES failure(s), $AO_VERIFY_WARNINGS warning(s)"
  fi
  ao_log "verify: PASS ($AO_VERIFY_WARNINGS warning(s))"
}

_verify_ok() {
  ao_log "verify: OK — $*"
}

_verify_warn() {
  AO_VERIFY_WARNINGS=$((AO_VERIFY_WARNINGS + 1))
  ao_warn "verify: WARN — $*"
}

_verify_fail() {
  AO_VERIFY_FAILURES=$((AO_VERIFY_FAILURES + 1))
  ao_warn "verify: FAIL — $*"
}
