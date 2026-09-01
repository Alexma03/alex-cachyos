package assets

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestPortableAndGalaxyTemplateFamiliesAreIsolated(t *testing.T) {
	galaxy := []string{
		"templates/hosts/galaxy/fixedDisplays/niri/config.kdl",
		"templates/hosts/galaxy/fixedDisplays/noctalia/settings.toml",
		"templates/hosts/galaxy/fixedInputDevices/hyprwhspr/config.json",
		"templates/hosts/galaxy/fixedInputDevices/noctalia/settings.toml",
		"templates/hosts/galaxy/literalHomePaths/noctalia/settings.toml",
	}
	for _, name := range galaxy {
		data, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read Galaxy template %q: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("Galaxy template %q is empty", name)
		}
	}

	portable := []string{
		"templates/roles/workstation/niri/config.kdl",
		"templates/roles/workstation/noctalia/settings.toml",
		"templates/roles/workstation/hyprwhspr/config.json",
	}
	for _, name := range portable {
		data, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read workstation template %q: %v", name, err)
		}
		for _, forbidden := range []string{"DP-3", "DP-1", "eDP-1", "/home/alex", "AT Translated Set 2 keyboard", "vicinae-snippet-virtual-keyboard", "battery_hidpp_battery_0"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("workstation template %q leaks %q", name, forbidden)
			}
		}
	}

	for _, old := range []string{
		"templates/niri/config.kdl",
		"templates/noctalia/settings.toml",
		"templates/hyprwhspr/config.json",
		"templates/hosts/galaxy/niri/config.kdl",
		"templates/hosts/galaxy/noctalia/settings.toml",
		"templates/hosts/galaxy/hyprwhspr/config.json",
	} {
		if _, err := FS.ReadFile(old); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("legacy mixed-ownership path %q remains readable: %v", old, err)
		}
	}
}

func TestGalaxyCapabilityFragmentsDoNotCrossRiskBoundaries(t *testing.T) {
	for _, test := range []struct {
		name      string
		paths     []string
		required  []string
		forbidden []string
	}{
		{
			name: "fixed displays",
			paths: []string{
				"templates/hosts/galaxy/fixedDisplays/niri/config.kdl",
				"templates/hosts/galaxy/fixedDisplays/noctalia/settings.toml",
			},
			required:  []string{"DP-1"},
			forbidden: []string{"/home/alex", "AT Translated Set 2 keyboard", "battery_hidpp_battery_0"},
		},
		{
			name: "fixed input devices",
			paths: []string{
				"templates/hosts/galaxy/fixedInputDevices/hyprwhspr/config.json",
				"templates/hosts/galaxy/fixedInputDevices/noctalia/settings.toml",
			},
			required:  []string{"AT Translated Set 2 keyboard", "battery_hidpp_battery_0"},
			forbidden: []string{"/home/alex", "DP-1", "eDP-1"},
		},
		{
			name:      "literal home paths",
			paths:     []string{"templates/hosts/galaxy/literalHomePaths/noctalia/settings.toml"},
			required:  []string{"/home/alex"},
			forbidden: []string{"DP-1", "eDP-1", "AT Translated Set 2 keyboard", "battery_hidpp_battery_0"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var combined string
			for _, name := range test.paths {
				data, err := FS.ReadFile(name)
				if err != nil {
					t.Fatalf("read capability fragment %q: %v", name, err)
				}
				combined += string(data)
			}
			for _, required := range test.required {
				if !strings.Contains(combined, required) {
					t.Fatalf("%s fragments omit %q", test.name, required)
				}
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(combined, forbidden) {
					t.Fatalf("%s fragments expose %q", test.name, forbidden)
				}
			}
		})
	}
}
