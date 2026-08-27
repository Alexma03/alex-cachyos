#!/usr/bin/env bash
# Module: fingerprint — SDCP libfprint package + PAM/greetd overlays.
#
# Install: build packaging/libfprint-egismoc-sdcp-git, install, apply overlays.
# Remove:  ./apply --profile galaxy --only fingerprint --remove
#          or AO_REMOVE=1 module_fingerprint

module_fingerprint() {
  local pkgdir="$AO_ROOT/packaging/libfprint-egismoc-sdcp-git"
  local overlay="$AO_ROOT/overlays/${AO_OVERLAY_PROFILE:-galaxy}/etc"
  local remove=${AO_REMOVE:-0}

  ao_log "fingerprint: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) fingerprint stack"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _fingerprint_remove "$overlay"
  else
    _fingerprint_install "$pkgdir" "$overlay"
  fi
}

_fingerprint_install() {
  local pkgdir=$1 overlay=$2
  local builddir pkgfile

  [[ -f $pkgdir/PKGBUILD ]] || ao_die "missing $pkgdir/PKGBUILD"
  ao_has_cmd makepkg || ao_die "makepkg required (pacman base-devel)"

  # Drop IgnorePkg=libfprint if present — the SDCP package conflicts with stock.
  if grep -qE '^IgnorePkg\s*=.*\blibfprint\b' /etc/pacman.conf; then
    ao_log "fingerprint: clearing IgnorePkg libfprint from pacman.conf"
    sudo sed -i -E 's/^(IgnorePkg\s*=.*)(\blibfprint\b)(.*)/\1\3/; s/  +/ /g; s/= /= /' /etc/pacman.conf
    # If line is empty-ish, comment it
    sudo sed -i -E 's/^IgnorePkg\s*=\s*$/#IgnorePkg =/' /etc/pacman.conf
  fi

  builddir=$(mktemp -d /tmp/alex-cachyos-fprint.XXXXXX)
  # shellcheck disable=SC2064
  trap "rm -rf '$builddir'" RETURN
  cp -a "$pkgdir"/. "$builddir"/
  ao_log "fingerprint: building package (needs network + base-devel)"
  (
    cd "$builddir"
    makepkg -sf --noconfirm
  ) || ao_die "makepkg failed"

  pkgfile=$(find "$builddir" -maxdepth 1 -name 'libfprint-egismoc-sdcp-git-*.pkg.tar.*' | head -1)
  [[ -n $pkgfile ]] || ao_die "built package not found"
  ao_log "fingerprint: installing $(basename "$pkgfile")"
  sudo pacman -U --noconfirm "$pkgfile"

  # Ensure fprintd present
  if ! pacman -Q fprintd &>/dev/null; then
    sudo pacman -S --needed --noconfirm fprintd usbutils
  fi

  # PAM / greetd overlays
  [[ -d $overlay/pam.d ]] || ao_die "missing overlay $overlay/pam.d"
  for f in sudo polkit-1 cosmic-greeter system-local-login greetd su su-l; do
    [[ -f $overlay/pam.d/$f ]] || continue
    sudo bash -c "source '$AO_ROOT/lib/common.sh'; ao_install_file '$overlay/pam.d/$f' '/etc/pam.d/$f'"
  done
  if [[ -f $overlay/greetd/cosmic-greeter.toml ]]; then
    sudo bash -c "source '$AO_ROOT/lib/common.sh'; ao_install_file '$overlay/greetd/cosmic-greeter.toml' '/etc/greetd/cosmic-greeter.toml'"
  fi

  sudo systemctl try-restart fprintd.service 2>/dev/null || true

  ao_log "fingerprint: package + PAM installed"
  ao_log "fingerprint: enroll with:  fprintd-enroll -f right-index-finger"
  ao_log "fingerprint: verify with:  fprintd-verify && sudo -k && sudo true"
}

_fingerprint_remove() {
  local overlay=$1

  ao_log "fingerprint: restoring PAM/greetd backups"
  for f in sudo polkit-1 cosmic-greeter system-local-login greetd su su-l; do
    sudo bash -c "source '$AO_ROOT/lib/common.sh'; ao_restore_file '/etc/pam.d/$f'"
  done
  sudo bash -c "source '$AO_ROOT/lib/common.sh'; ao_restore_file '/etc/greetd/cosmic-greeter.toml'"

  if pacman -Q libfprint-egismoc-sdcp-git &>/dev/null; then
    ao_log "fingerprint: removing SDCP package and restoring stock libfprint"
    sudo pacman -Rdd --noconfirm libfprint-egismoc-sdcp-git
    sudo pacman -S --needed --noconfirm libfprint
  fi

  sudo systemctl try-restart fprintd.service 2>/dev/null || true
  ao_log "fingerprint: removed (enrolled prints in /var/lib/fprint left untouched)"
}
