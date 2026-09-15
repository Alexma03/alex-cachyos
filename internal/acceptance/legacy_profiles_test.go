package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type legacyProfile struct {
	Name                 string          `json:"name"`
	Modules              map[string]bool `json:"modules"`
	OverlayProfile       string          `json:"overlay_profile"`
	DesktopVariant       string          `json:"desktop_variant"`
	FingerprintSupported bool            `json:"fingerprint_supported"`
}

func TestProductionProfilesSeparateGalaxyHardwareFromGenericWorkstation(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	load := func(name string) legacyProfile {
		t.Helper()
		data, readErr := os.ReadFile(filepath.Join(root, "profiles", name+".json"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		var profile legacyProfile
		if decodeErr := json.Unmarshal(data, &profile); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		return profile
	}

	galaxy := load("galaxy")
	generic := load("generic")
	if galaxy.DesktopVariant != "galaxy" || !galaxy.Modules["fingerprint"] || !galaxy.FingerprintSupported {
		t.Fatalf("galaxy hardware policy = %#v", galaxy)
	}
	if generic.Name != "generic" || generic.OverlayProfile != "generic" || generic.DesktopVariant != "workstation" || generic.Modules["fingerprint"] || generic.FingerprintSupported {
		t.Fatalf("generic hardware policy = %#v", generic)
	}

	for _, relative := range []string{
		"templates/roles/workstation/niri/config.kdl",
		"templates/roles/workstation/noctalia/settings.toml",
		"templates/roles/workstation/hyprwhspr/config.json",
		"overlays/generic/etc/greetd/config.toml",
		"overlays/generic/etc/noctalia-greeter/greeter.toml",
		"overlays/generic/etc/pam.d/alex-cachyos-login",
	} {
		data, readErr := os.ReadFile(filepath.Join(root, relative))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, forbidden := range []string{"pam_fprintd", "map-to-output", "ddc_bus", "eDP-1", "DP-1", "DP-3", "/home/alex", "AT Translated Set 2 keyboard"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("generic asset %s contains Galaxy value %q", relative, forbidden)
			}
		}
	}
}

func TestGenericLegacyDryRunSkipsFingerprint(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("legacy apply intentionally refuses root")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(root, "apply"), "--profile", "generic", "--dry-run")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generic dry-run: %v\n%s", err, output)
	}
	text := string(output)
	if !strings.Contains(text, "profile=generic") || !strings.Contains(text, "overlay=generic") || !strings.Contains(text, "skip module fingerprint") || !strings.Contains(text, "→ module desktop") {
		t.Fatalf("unexpected generic dry-run:\n%s", text)
	}

	blocked := exec.Command(filepath.Join(root, "apply"), "--profile", "generic", "--only", "fingerprint", "--dry-run")
	blocked.Dir = root
	blockedOutput, blockedErr := blocked.CombinedOutput()
	if blockedErr == nil || !strings.Contains(string(blockedOutput), "does not support the Galaxy fingerprint stack") {
		t.Fatalf("generic fingerprint override was not blocked: err=%v\n%s", blockedErr, blockedOutput)
	}
}
