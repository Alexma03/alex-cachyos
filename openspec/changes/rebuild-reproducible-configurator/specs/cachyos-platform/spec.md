# CachyOS Platform Specification

## Purpose

The desktop platform boundary: CachyOS Linux with the niri Wayland compositor and
the Noctalia shell/greeter, elevated with `pkexec` and fingerprint-authenticated
polkit. The Bash module content (bootstrap, fingerprint, devtools, apps, vicinae,
desktop, verify) is mapped 1:1 into typed catalog steps. No other distribution,
compositor, or display protocol is in scope.

## Requirements

### Requirement: CachyOS plus niri plus Noctalia only

The catalog and planner MUST support exactly CachyOS Linux with the niri compositor
(Wayland) and the Noctalia shell plus `noctalia-greeter`, `greetd`, and
`AccountsService`/`.dmrc`. The system MUST NOT include X11 tooling (for example
`xdotool` or `xrandr`) or alternative compositors/desktops (Hyprland, GNOME, KDE,
Cosmic).

#### Scenario: Catalog declares the niri plus Noctalia stack

- GIVEN the merged catalog for host `galaxy`
- WHEN the desktop module plan is built
- THEN it contains niri, Noctalia, greetd, and AccountsService steps and no X11 or alternative-DE package

### Requirement: Biometric polkit elevation

System elevation MUST use `pkexec` with the fingerprint polkit agent; PAM overlays
MUST configure fprintd as `sufficient` for polkit and sudo. The system MUST NOT
invoke `sudo` for elevation.

#### Scenario: Elevation authenticates via fingerprint polkit

- GIVEN a host with enrolled fingerprints and the PAM overlays applied
- WHEN a system step elevates
- THEN elevation goes through `pkexec` and the polkit dialog accepts fingerprint authentication

### Requirement: Module content mapped one-to-one

Each existing Bash module's behavior MUST be preserved as typed catalog steps:
`bootstrap` (system update, want/remove lists, atomic GRUB regeneration, Plymouth
strip, ananicy-cpp, UFW, Chrome via paru, zsh marker blocks, LTS kernel removal
deferral), `fingerprint` (pinned PKGBUILD build and install), `devtools` (mise plus
pnpm/npm configs), `apps` (pacman and AUR lists, docker/tailscale/nordvpn services,
webapps with favicon fallback), `vicinae` (`vicinae-bin` plus user service), and
`desktop` (niri/Noctalia/hyprwhspr/quickshell-polkit/greetd/Cosmic
prune/AccountsService/polkit/Noctalia plugins), with `verify` last.

#### Scenario: Bootstrap steps render from the catalog

- GIVEN the merged catalog for host `galaxy`
- WHEN the bootstrap module is planned
- THEN the plan includes the want/remove package lists, atomic GRUB step, Plymouth strip, and the zsh marker-block rewrite, matching the Bash module's behavior

#### Scenario: Legacy Cosmic stack is pruned after the new path exists

- GIVEN a host with the legacy Cosmic stack present
- WHEN the desktop module runs
- THEN the new desktop path is installed before Cosmic packages, PAM, flatpak remotes, and user state are pruned

### Requirement: Explicit package ownership

Declared packages MUST be marked explicit so `pacman -Rns` never prunes them, and
installs MUST use `--needed` semantics with pre-install presence checks, preserving
the existing explicit-marking philosophy.

#### Scenario: Declared package survives a dependency cleanup

- GIVEN a host where a declared catalog package is installed and marked explicit
- WHEN `pacman -Rns` runs for an unneeded dependency chain
- THEN the declared package is not removed

### Requirement: Fingerprint package reproducible pinning

The fingerprint module MUST build the pinned PKGBUILD (pinned upstream commit plus
pinned patch sha256, providing and conflicting stock libfprint), install via
`pacman -U`, and skip rebuild when the installed version already matches the pinned
commit and pkgrel.

#### Scenario: Skip rebuild when the pinned version is installed

- GIVEN a host where the installed fingerprint package matches the pinned commit and pkgrel
- WHEN `apply` runs the fingerprint module
- THEN the build and install steps are skipped and reported as satisfied

### Requirement: Webapp install with icon fallback

Webapps MUST be installed from `name|url|icon_url` entries with a Chrome `--app`
launcher in `~/.local/bin`; when a remote icon fetch fails, the install MUST degrade
gracefully to a favicon fallback rather than failing the module.

#### Scenario: Remote icon failure falls back to favicon

- GIVEN a webapp entry whose remote icon URL is unreachable
- WHEN the apps module installs the webapp
- THEN the webapp installs with a favicon-derived icon and the module does not fail
