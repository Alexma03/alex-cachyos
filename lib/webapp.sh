# Shared webapp helpers (Chrome --app), Omarchy-inspired.

ao_webapp_chrome() {
  command -v google-chrome-stable 2>/dev/null \
    || command -v google-chrome 2>/dev/null \
    || command -v chromium 2>/dev/null \
    || true
}

# Install launcher into ~/.local/bin so .desktop entries survive outside the repo.
ao_webapp_install_launcher() {
  local src="$AO_ROOT/bin/alex-cachyos-webapp-launch"
  local dst="${HOME:?}/.local/bin/alex-cachyos-webapp-launch"
  [[ -f $src ]] || ao_die "missing $src"
  mkdir -p "$(dirname "$dst")"
  install -m 755 "$src" "$dst"
  ao_log "webapp launcher → $dst"
}

# ao_webapp_install NAME URL [ICON_URL]
ao_webapp_install() {
  local name=$1 url=$2 icon_ref=${3:-}
  local home=${HOME:?}
  local apps=$home/.local/share/applications
  local icons=$apps/icons
  local desktop launcher icon_path safe

  [[ -n $name && -n $url ]] || ao_die "webapp needs name and url"
  if [[ ! $url =~ ^[a-zA-Z][a-zA-Z0-9+.-]*: ]]; then
    url="https://$url"
  fi

  mkdir -p "$apps" "$icons"
  launcher=$home/.local/bin/alex-cachyos-webapp-launch
  [[ -x $launcher ]] || ao_webapp_install_launcher

  safe=${name// /-}
  icon_path=$icons/$safe.png
  if [[ -z $icon_ref ]]; then
    icon_ref="https://www.google.com/s2/favicons?domain=${url}&sz=128"
  fi
  if [[ $icon_ref =~ ^https?:// ]]; then
    if ! curl -fsSL -o "$icon_path" "$icon_ref" || [[ ! -s $icon_path ]]; then
      ao_warn "favicon failed for $name — desktop entry without custom icon"
      icon_path=web-browser
    fi
  else
    icon_path=$icons/$icon_ref
  fi

  desktop=$apps/${safe}.desktop
  cat >"$desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=$name
Comment=$name (Chrome webapp — alex-cachyos)
Exec=$launcher $url
Icon=$icon_path
Terminal=false
StartupNotify=true
Categories=Network;
EOF
  chmod +x "$desktop"
  ao_log "webapp installed: $desktop"
}

ao_webapp_remove() {
  local name=$1
  local home=${HOME:?}
  local safe=${name// /-}
  rm -f "$home/.local/share/applications/${safe}.desktop"
  rm -f "$home/.local/share/applications/icons/${safe}.png"
  ao_log "webapp removed: $name"
}
