# Delta for CachyOS Platform

## MODIFIED Requirements

### Requirement: CachyOS plus niri plus Noctalia only

Every host MUST remain CachyOS with niri, Noctalia, `noctalia-greeter`, `greetd`, and AccountsService. X11 tooling and alternative compositors/desktops MUST NOT be included. A portable host MUST inherit hardware-independent workstation behavior and MUST NOT consume Galaxy overlays. Fixed outputs/devices/home paths MUST be absent by default; a portable host MAY consume only its own explicit, non-invented values when the corresponding host-owned capability is enabled.

(Previously: The desktop stack scenario was defined only for `galaxy`.)

#### Scenario: Portable stack has no Galaxy assets

- GIVEN a synthetic portable host
- WHEN its desktop plan is built
- THEN niri, Noctalia, greetd, and AccountsService are present
- AND no Galaxy-specific, X11, or alternative-DE asset appears

### Requirement: Biometric polkit elevation

System elevation MUST use `pkexec`, never `sudo`. Fingerprint packages and PAM mutation MUST be planned only when the selected host explicitly opts in; otherwise they MUST be absent.

(Previously: Fingerprint-authenticated polkit and PAM overlays were unconditional.)

#### Scenario: Opted-in biometric elevation

- GIVEN a host explicitly enables fingerprint/PAM behavior and observations satisfy its preconditions
- WHEN a system step elevates
- THEN `pkexec` uses the cataloged fingerprint/PAM path

#### Scenario: Portable default omits biometrics

- GIVEN a host has no fingerprint/PAM opt-in
- WHEN its plan is built
- THEN no biometric package or PAM mutation is present

### Requirement: Module content mapped one-to-one

Galaxy MUST retain typed mappings for `bootstrap`, `fingerprint`, `devtools`, `apps`, `vicinae`, `desktop`, and final `verify`, including their existing package, service, webapp, GRUB/Plymouth, PAM, and desktop behaviors. Other hosts MUST receive only policy-approved steps. Destructive bootstrap actions, fixed hardware configuration, and Cosmic pruning MUST require individual host opt-ins; observations MAY block but MUST NOT authorize them.

(Previously: All mapped Galaxy-sensitive substeps were unconditional within enabled modules.)

#### Scenario: Bootstrap steps render from the catalog

- GIVEN `galaxy` carries the required opt-ins
- WHEN its bootstrap module is planned
- THEN want/remove packages, atomic GRUB, Plymouth, and zsh marker-block steps remain represented

#### Scenario: Legacy Cosmic stack is pruned after the new path exists

- GIVEN a host opts into Cosmic pruning and its niri/Noctalia/greetd target is satisfied
- WHEN the desktop module runs
- THEN Cosmic packages, PAM, remotes, and user state are pruned only after the target exists

#### Scenario: Portable plan excludes risky substeps

- GIVEN a portable host omits all risky opt-ins
- WHEN bootstrap and desktop are planned
- THEN the risky substeps are absent despite matching observations
