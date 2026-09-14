#!/usr/bin/env bash
# Module: fingerprint — SDCP libfprint package + sudo/polkit PAM overlays.
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
  local builddir pkgfile commit pkgrel installed_version

  [[ -f $pkgdir/PKGBUILD ]] || ao_die "missing $pkgdir/PKGBUILD"
  ao_has_cmd makepkg || ao_die "makepkg required (pacman base-devel)"
  ao_has_cmd pkexec || ao_die "pkexec required (polkit)"

  commit=$(sed -nE 's/^_commit=([[:xdigit:]]+).*/\1/p' "$pkgdir/PKGBUILD")
  pkgrel=$(sed -nE 's/^pkgrel=([0-9]+).*/\1/p' "$pkgdir/PKGBUILD")
  installed_version=$(pacman -Q libfprint-egismoc-sdcp-git 2>/dev/null | awk '{print $2}' || true)
  if [[ -n $commit && -n $pkgrel \
        && $installed_version == *".${commit:0:7}-$pkgrel" ]]; then
    ao_log "fingerprint: pinned package already current ($installed_version); skipping rebuild"
    _fingerprint_configure_root "$overlay" ""
    _fingerprint_done
    return 0
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
  [[ -d $overlay/pam.d ]] || ao_die "missing overlay $overlay/pam.d"

  _fingerprint_configure_root "$overlay" "$pkgfile"
  _fingerprint_done
}

_fingerprint_configure_root() {
  local overlay=$1 pkgfile=$2

  ao_log "fingerprint: install package + PAM (pkexec — pon la huella en el diálogo)"
  ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'

    if grep -qE '^IgnorePkg[[:space:]]*=.*[[:space:]]libfprint([[:space:]]|\$)' /etc/pacman.conf \\
       || grep -qE '^IgnorePkg[[:space:]]*=[[:space:]]*libfprint([[:space:]]|\$)' /etc/pacman.conf; then
      sed -i -E 's/^(IgnorePkg[[:space:]]*=.*)([[:space:]]*)\\blibfprint\\b([[:space:]]*)/\\1\\3/; s/  +/ /g; s/= /= /' /etc/pacman.conf
      sed -i -E 's/^IgnorePkg[[:space:]]*=[[:space:]]*\$/#IgnorePkg =/' /etc/pacman.conf
      ao_log 'cleared IgnorePkg libfprint'
    fi

    if [[ -n '$pkgfile' ]]; then
      pacman -U --noconfirm '$pkgfile'
    fi
    pacman -Q fprintd &>/dev/null || pacman -S --needed --noconfirm fprintd usbutils

    # Login uses the dedicated alex-cachyos-login service from desktop. Do not
    # add fprintd to system-local-login: Noctalia drives lock biometrics itself.
    for f in sudo polkit-1; do
      [[ -f '$overlay/pam.d/'\$f ]] || continue
      ao_install_file '$overlay/pam.d/'\$f '/etc/pam.d/'\$f
    done
    systemctl try-restart fprintd.service 2>/dev/null || true
  "
}

_fingerprint_done() {
  ao_log "fingerprint: package + PAM installed"
  ao_log "fingerprint: enroll with:  fprintd-enroll -f right-index-finger"
  ao_log "fingerprint: verify with:  fprintd-verify && sudo -k && sudo true"
}

_fingerprint_remove() {
  local overlay=$1

  ao_log "fingerprint: restore PAM + stock libfprint (pkexec — pon la huella)"
  ao_root bash -c "
    set -euo pipefail
    source '$AO_ROOT/lib/common.sh'
    for f in sudo polkit-1; do
      ao_restore_file '/etc/pam.d/'\$f
    done
    if pacman -Q libfprint-egismoc-sdcp-git &>/dev/null; then
      pacman -Rdd --noconfirm libfprint-egismoc-sdcp-git
      pacman -S --needed --noconfirm libfprint
    fi
    systemctl try-restart fprintd.service 2>/dev/null || true
  "
  ao_log "fingerprint: removed (enrolled prints in /var/lib/fprint left untouched)"
}
