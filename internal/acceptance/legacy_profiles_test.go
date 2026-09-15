package acceptance

import (
	"bufio"
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

func TestBootstrapUpdatesManagedSourcesInOrderAndStopsOnFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("legacy apply intentionally refuses root")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	mockBin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "commands.log")
	mock := `#!/usr/bin/env bash
name=${0##*/}
printf '%s %s\n' "$name" "$*" >>"$COMMAND_LOG"
if [[ $name == pacman && $1 == -Q ]]; then exit 1; fi
if [[ $name == paru && $1 == -Sua && ${FAIL_AUR:-0} == 1 ]]; then exit 23; fi
exit 0
`
	for _, name := range []string{"pacman", "pkexec", "paru", "flatpak"} {
		if err := os.WriteFile(filepath.Join(mockBin, name), []byte(mock), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	run := func(failAUR bool) ([]byte, error) {
		t.Helper()
		if err := os.WriteFile(logPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(filepath.Join(root, "apply"), "--profile", "generic", "--only", "bootstrap")
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"AO_APPLY_LOCKED=1",
			"HOME="+t.TempDir(),
			"PATH="+mockBin+":"+os.Getenv("PATH"),
			"COMMAND_LOG="+logPath,
		)
		if failAUR {
			cmd.Env = append(cmd.Env, "FAIL_AUR=1")
		}
		output, runErr := cmd.CombinedOutput()
		log, readErr := os.ReadFile(logPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		return append(output, log...), runErr
	}

	output, err := run(false)
	if err != nil {
		t.Fatalf("mocked bootstrap: %v\n%s", err, output)
	}
	text := string(output)
	previous := -1
	for _, command := range []string{"pacman -Syu", "paru -Sua --noconfirm --ignore libfprint-egismoc-sdcp-git", "pkexec flatpak update --system --noninteractive -y", "flatpak update --user --noninteractive -y"} {
		index := strings.Index(text, command)
		if index <= previous {
			t.Fatalf("update command %q missing or out of order:\n%s", command, text)
		}
		previous = index
	}

	failedOutput, err := run(true)
	if err == nil || strings.Contains(string(failedOutput), "flatpak update") {
		t.Fatalf("AUR failure did not stop before Flatpak: err=%v\n%s", err, failedOutput)
	}
}

func TestDevtoolsTrackCurrentChannels(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	miseConfig, err := os.ReadFile(filepath.Join(root, "templates", "devtools", "mise.config.toml"))
	if err != nil {
		t.Fatal(err)
	}

	tools := map[string]string{}
	inTools := false
	scanner := bufio.NewScanner(strings.NewReader(string(miseConfig)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inTools = line == "[tools]"
			continue
		}
		if !inTools || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			t.Fatalf("invalid tools entry %q", line)
		}
		key = strings.TrimSpace(key)
		if _, duplicate := tools[key]; duplicate {
			t.Fatalf("duplicate tool %q", key)
		}
		tools[key] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"go": "latest", "node": "lts", "npm": "latest", "pnpm": "latest"}
	if len(tools) != len(want) {
		t.Fatalf("mise tools = %#v", tools)
	}
	for tool, channel := range want {
		if tools[tool] != channel {
			t.Fatalf("mise %s channel = %q, want %q", tool, tools[tool], channel)
		}
	}
}
