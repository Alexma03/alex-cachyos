# Apps — packages + Chrome webapps

## Install

```bash
./apply --profile galaxy --only apps
```

## Packages

| App | Source | Package |
|-----|--------|---------|
| Cursor | CachyOS | `cursor-bin` |
| Warp | AUR | `warp-terminal-bin` |
| Slack | AUR | `slack-desktop` |
| ChatGPT | CachyOS | `chatgpt-desktop-bin` |
| Discord | repos | `discord` |
| LocalSend | CachyOS | `localsend` |
| Tailscale | repos | `tailscale` |
| Docker engine | repos | `docker` |
| Docker Desktop | AUR | `docker-desktop` |
| hyprwhspr | AUR | `hyprwhspr` |
| CodexBar CLI | AUR | `codexbar-cli` |
| NordVPN | AUR | `nordvpn-bin` |
| Chrome | bootstrap | `google-chrome` |

Chrome is installed by **bootstrap**. Docker Desktop provides compose/buildx (do not install those repo packages separately). The engine (`docker.service`) starts at boot for CLI; the Docker Desktop GUI is **not** autostarted (`systemctl --user disable docker-desktop`). Open it from the app menu when you need it.

`hyprwhspr` is local speech-to-text (replaces Spokenly). `codexbar-cli` feeds the Noctalia CodexBar plugin. The **desktop** module enables those plugins on the bar.

## Webapps (Chrome `--app`)

Omarchy-style: `.desktop` → `alex-cachyos-webapp-launch` → `google-chrome-stable --app=URL`.

| Name | URL |
|------|-----|
| WhatsApp | https://web.whatsapp.com |
| Telegram | https://web.telegram.org/k/ |
| Linear | https://linear.app |

Native `telegram-desktop` is removed if present.

## After install

```bash
# Docker (new group → new login / session)
docker ps

# Tailscale
sudo tailscale up

# NordVPN
nordvpn login
nordvpn connect
```

## Remove webapps only

```bash
./apply --profile galaxy --only apps --remove
```
