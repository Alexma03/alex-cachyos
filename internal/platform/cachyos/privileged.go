package cachyos

import (
	"os"

	"alex-cachyos/internal/runner"
)

// PrivilegedHelperPolicy is the compiled root-side allowlist. Request JSON can
// select only one of these exact destinations with its exact publication mode.
func PrivilegedHelperPolicy(sourceUID uint32) runner.SelfHelperPolicy {
	destinations := map[string]os.FileMode{
		"/etc/default/grub":                         0o644,
		"/etc/environment.d/99-vicinae-cosmic.conf": 0o644,
		"/etc/greetd/cosmic-greeter.toml":           0o644,
		"/etc/mkinitcpio.conf":                      0o644,
		"/etc/pacman.conf":                          0o644,
		"/etc/pam.d/cosmic-greeter":                 0o644,
		"/etc/pam.d/greetd":                         0o644,
		"/etc/pam.d/polkit-1":                       0o644,
		"/etc/pam.d/su":                             0o644,
		"/etc/pam.d/su-l":                           0o644,
		"/etc/pam.d/sudo":                           0o644,
		"/etc/pam.d/system-local-login":             0o644,
		"/boot/grub/grub.cfg":                       0o644,
	}
	return runner.SelfHelperPolicy{SourceUID: sourceUID, Destinations: destinations}
}
