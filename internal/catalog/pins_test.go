package catalog

import (
	"errors"
	"strings"
	"testing"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

func TestValidateNpmPinAcceptsExactSemverAndRejectsRangesAndTags(t *testing.T) {
	if err := ValidateNpmPin("pi-web-access", NpmPin{Name: "pi-web-access", Version: "0.27.0"}); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"^1.2.3", "~1.2.3", ">=1.2.3", "latest", "next", "1.2", "1.2.3 || 2.0.0"} {
		t.Run(version, func(t *testing.T) {
			err := ValidateNpmPin("provider", NpmPin{Name: "provider", Version: version})
			assertPinError(t, err, "provider", "/version")
		})
	}
}
func TestValidateNpmSpecRequiresExactPackageVersion(t *testing.T) {
	if err := ValidateNpmSpec("provider", "npm:@scope/provider@1.2.3-beta.1+build.7"); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"provider@1.2.3", "npm:provider@latest", "npm:@scope/provider@^1.2.3"} {
		t.Run(spec, func(t *testing.T) {
			assertPinError(t, ValidateNpmSpec("provider", spec), "provider", "/spec")
		})
	}
}
func validCheckout() (CheckoutPin, CheckoutDestination) {
	return CheckoutPin{
		Remote: "https://example.invalid/team/repository.git",
		Branch: "main",
		Commit: strings.ToUpper(testCommit),
	}, CheckoutDestination{
		Path:           "/home/alex/Projects/upstream-repo",
		ForbiddenRoots: []string{"/etc", "/usr", "/var", "/run"},
	}
}
func TestValidateSourceCheckoutPin(t *testing.T) {
	pin, destination := validCheckout()
	if err := ValidateSourceCheckoutPin("upstream-repo", pin, destination); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(*CheckoutPin, *CheckoutDestination)
		path string
	}{
		{"noncanonical remote", func(p *CheckoutPin, _ *CheckoutDestination) { p.Remote += "/" }, "/remote"},
		{"wrong branch", func(p *CheckoutPin, _ *CheckoutDestination) { p.Branch = "release" }, "/branch"},
		{"short commit", func(p *CheckoutPin, _ *CheckoutDestination) { p.Commit = "abc" }, "/commit"},
		{"relative destination", func(_ *CheckoutPin, d *CheckoutDestination) { d.Path = "Projects/upstream-repo" }, "/destination"},
		{"forbidden destination", func(_ *CheckoutPin, d *CheckoutDestination) { d.Path = "/etc/upstream-repo" }, "/destination"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotPin, gotDestination := validCheckout()
			test.edit(&gotPin, &gotDestination)
			assertPinError(t, ValidateSourceCheckoutPin("upstream-repo", gotPin, gotDestination), "upstream-repo", test.path)
		})
	}
}
func TestValidatePacmanArtifactPinRequiresApprovedSource(t *testing.T) {
	for _, source := range []PacmanArtifactSource{PacmanArtifactCache, PacmanArtifactArchive} {
		pin := PacmanArtifactPin{Package: "example-package", Version: "1:2.3.4-1", Source: source, SHA256: strings.Repeat("a", 64)}
		if err := ValidatePacmanArtifactPin("example", pin); err != nil {
			t.Fatalf("source %q: %v", source, err)
		}
	}
	for _, source := range []PacmanArtifactSource{"", PacmanArtifactRepository, "mirror"} {
		pin := PacmanArtifactPin{Package: "example-package", Version: "2.3.4-1", Source: source, SHA256: strings.Repeat("a", 64)}
		assertPinError(t, ValidatePacmanArtifactPin("example", pin), "example", "/source")
	}
	bad := PacmanArtifactPin{Package: "example-package", Version: ">=2.3.4", Source: PacmanArtifactCache, SHA256: "short"}
	assertPinError(t, ValidatePacmanArtifactPin("example", bad), "example", "/version")
}
func TestValidateAURLocalAndRemoteArtifactPins(t *testing.T) {
	if err := ValidateAURLocalPin("driver", AURLocalPin{SourceCommit: testCommit, PatchSHA256: strings.Repeat("b", 64)}); err != nil {
		t.Fatal(err)
	}
	assertPinError(t, ValidateAURLocalPin("driver", AURLocalPin{SourceCommit: "deadbeef", PatchSHA256: strings.Repeat("b", 64)}), "driver", "/sourceCommit")
	assertPinError(t, ValidateAURLocalPin("driver", AURLocalPin{SourceCommit: testCommit, PatchSHA256: "bad"}), "driver", "/patchSHA256")
	valid := RemoteArtifactPin{URL: "https://downloads.example.invalid/tool-1.2.3.tar.zst", SHA256: strings.Repeat("c", 64)}
	if err := ValidateRemoteArtifactPin("tool", valid); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"http://downloads.example.invalid/tool.tar.zst", "https://downloads.example.invalid/latest.tar.zst", "https://downloads.example.invalid/tool.tar.zst?mirror=1"} {
		assertPinError(t, ValidateRemoteArtifactPin("tool", RemoteArtifactPin{URL: url, SHA256: valid.SHA256}), "tool", "/url")
	}
	assertPinError(t, ValidateRemoteArtifactPin("tool", RemoteArtifactPin{URL: valid.URL, SHA256: "bad"}), "tool", "/sha256")
}
func assertPinError(t *testing.T, err error, pin, path string) {
	t.Helper()
	if err == nil {
		t.Fatal("invalid pin was accepted")
	}
	var pinErr *PinValidationError
	if !errors.As(err, &pinErr) {
		t.Fatalf("error %T is not PinValidationError: %v", err, err)
	}
	if pinErr.Pin != pin || pinErr.Path != path || !strings.Contains(err.Error(), pin) {
		t.Fatalf("pin error = %#v (%v), want pin %q path %q", pinErr, err, pin, path)
	}
}
