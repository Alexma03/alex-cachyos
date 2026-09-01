package cachyos

import "testing"

func TestPrivilegedHelperPolicyUsesExactKnownDestinations(t *testing.T) {
	policy := PrivilegedHelperPolicy(1000)
	for _, path := range []string{"/etc/default/grub", "/etc/mkinitcpio.conf", "/boot/grub/grub.cfg", "/etc/pam.d/polkit-1"} {
		if mode, ok := policy.Destinations[path]; !ok || mode.Perm() != 0o644 {
			t.Fatalf("policy[%q] = %04o, %v", path, mode.Perm(), ok)
		}
	}
	for _, broad := range []string{"/", "/etc", "/boot"} {
		if _, ok := policy.Destinations[broad]; ok {
			t.Fatalf("broad destination %q is allowlisted", broad)
		}
	}
}
