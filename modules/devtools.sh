#!/usr/bin/env bash
# Module: devtools — mise + Node (LTS) + pnpm/npm supply-chain defaults (user home).
#
# Does NOT remove pacman nodejs (cursor-bin depends on it).

module_devtools() {
  local tpl="$AO_ROOT/templates/devtools"
  local remove=${AO_REMOVE:-0}

  ao_log "devtools: $([[ $remove -eq 1 ]] && echo remove || echo install)"

  if [[ $AO_DRY_RUN -eq 1 ]]; then
    ao_log "DRY: would $([[ $remove -eq 1 ]] && echo remove || echo install) mise/node/pnpm/npm stack"
    return 0
  fi

  if [[ $remove -eq 1 ]]; then
    _devtools_remove
  else
    _devtools_install "$tpl"
  fi
}

_devtools_install() {
  local tpl=$1
  local home=${HOME:?}
  local tmp

  [[ -d $tpl ]] || ao_die "missing templates: $tpl"

  if ! pacman -Q mise &>/dev/null; then
    ao_log "devtools: installing mise (pkexec — pon la huella)"
    ao_root pacman -S --needed --noconfirm mise
  else
    ao_log "devtools: mise already installed"
  fi

  mkdir -p "$home/.config/mise" "$home/.config/pnpm" "$home/.local/bin"

  ao_install_user_file "$tpl/mise.config.toml" "$home/.config/mise/config.toml"
  tmp=$(mktemp)
  sed "s|@HOME@|$home|g" "$tpl/pnpm.config.yaml" >"$tmp"
  ao_install_user_file "$tmp" "$home/.config/pnpm/config.yaml"
  rm -f "$tmp"
  ao_install_user_file "$tpl/npmrc" "$home/.npmrc"

  _devtools_shell_activate "$home"

  ao_log "devtools: mise install (node/npm/pnpm)"
  # Ensure shims resolve even if this shell never activated mise.
  export PATH="${home}/.local/share/mise/shims:${PATH}"
  eval "$(mise activate bash)"
  mise install
  if command -v corepack >/dev/null 2>&1; then
    corepack disable 2>/dev/null || true
  fi

  ao_log "devtools: done — open a new shell, then: node -v && pnpm -v && npm -v"
  ao_log "devtools: docs: docs/devtools.md"
}

_devtools_remove() {
  local home=${HOME:?}
  ao_restore_user_file "$home/.config/mise/config.toml"
  ao_restore_user_file "$home/.config/pnpm/config.yaml"
  ao_restore_user_file "$home/.npmrc"
  _devtools_shell_strip "$home"
  ao_log "devtools: configs restored/stripped (pacman mise + nodejs left installed)"
}

_devtools_shell_activate() {
  local home=$1
  python3 - "$home" <<'PY'
import pathlib, re, sys

home = pathlib.Path(sys.argv[1])
BEGIN = "# >>> alex-cachyos/devtools >>>"
END = "# <<< alex-cachyos/devtools <<<"

blocks = {
    home / ".zshrc": '''eval "$(mise activate zsh)"
# After mise: global CLIs; mise keeps winning for node/pnpm
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$PATH:$HOME/.local/bin" ;;
esac
''',
    home / ".bashrc": '''eval "$(mise activate bash)"
# After mise: global CLIs; mise keeps winning for node/pnpm
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) export PATH="$PATH:$HOME/.local/bin" ;;
esac
''',
    home / ".config/fish/config.fish": '''mise activate fish | source
# After mise: global CLIs (pnpm add -g → here), mise keeps winning for node/pnpm
fish_add_path --append --path "$HOME/.local/bin"
''',
}

# Loose lines from the earlier manual setup (pre-markers).
LOOSE = re.compile(
    r"^(?:mise activate fish \| source|"
    r'eval "\$\(mise activate (?:zsh|bash)\)"|'
    r"# After mise:.*|"
    r'fish_add_path --append --path "\$HOME/\.local/bin"|'
    r"case \":\$PATH:\" in|"
    r'  \*\":\$HOME/\.local/bin:\"\*\) ;;|'
    r'  \*\) export PATH=\"\$PATH:\$HOME/\.local/bin\" ;;|'
    r"esac)\s*$"
)

def upsert(path: pathlib.Path, body: str) -> str:
    path.parent.mkdir(parents=True, exist_ok=True)
    text = path.read_text() if path.exists() else ""
    block = f"{BEGIN}\n{body.rstrip()}\n{END}"

    if BEGIN in text:
        pre, rest = text.split(BEGIN, 1)
        if END in rest:
            _, post = rest.split(END, 1)
        else:
            post = ""
        new = pre.rstrip("\n") + "\n\n" + block + ("\n" + post.lstrip("\n") if post.strip() else "\n")
        action = "updated"
    else:
        lines = [ln for ln in text.splitlines() if not LOOSE.match(ln)]
        # Drop orphan blank runs left by stripping
        cleaned = "\n".join(lines).rstrip() + "\n"
        new = cleaned + ("\n" if cleaned.strip() else "") + block + "\n"
        action = "appended"

    if new != text:
        if path.exists() and not path.with_suffix(path.suffix + ".bak.alex-cachyos").exists():
            # Only for shell files we may backup once under .bak.alex-cachyos beside file
            bak = pathlib.Path(str(path) + ".bak.alex-cachyos")
            if not bak.exists():
                bak.write_text(text)
        path.write_text(new)
    return action

for path, body in blocks.items():
    action = upsert(path, body)
    print(f"==> {action} shell block in {path}")
PY
}

_devtools_shell_strip() {
  local home=$1
  python3 - "$home" <<'PY'
import pathlib, sys
home = pathlib.Path(sys.argv[1])
BEGIN = "# >>> alex-cachyos/devtools >>>"
END = "# <<< alex-cachyos/devtools <<<"
for path in (home / ".zshrc", home / ".bashrc", home / ".config/fish/config.fish"):
    if not path.exists():
        continue
    text = path.read_text()
    if BEGIN not in text:
        continue
    pre, rest = text.split(BEGIN, 1)
    post = rest.split(END, 1)[1] if END in rest else ""
    path.write_text((pre.rstrip() + "\n" + post.lstrip("\n")).lstrip("\n") or "")
    print(f"==> stripped shell block from {path}")
PY
}
