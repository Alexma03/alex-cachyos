package acceptance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alex-cachyos/internal/app"
	"alex-cachyos/internal/assets"
	"alex-cachyos/internal/catalog"
	"alex-cachyos/internal/platform/cachyos"
	"alex-cachyos/internal/statepath"
)

func TestCatalogFixturesMatchAcceptanceGoldens(t *testing.T) {
	for _, fixture := range []struct{ name, host string }{{"galaxy-fixture", "galaxy"}, {"portable-synthetic", "portable-synthetic"}} {
		t.Run(fixture.name, func(t *testing.T) {
			policy := loadPolicy(t, fixture.host)
			evidence := readyPlatformEvidence(t)
			plan, err := cachyos.BuildHostPlan(policy, evidence)
			if err != nil {
				t.Fatal(err)
			}
			check := app.EvaluateCheck(app.CheckRequest{}, app.CheckSnapshot{})
			rollback, err := app.PlanRollbackInverse(app.ManagedFileRollback{
				Class: app.ManagedCreated, Target: "/fixture/managed", AfterHash: strings.Repeat("a", 64), Live: app.LiveFile{Exists: true, Hash: strings.Repeat("a", 64)},
			})
			if err != nil {
				t.Fatal(err)
			}
			execution, err := ExerciseFixturePlan(context.Background(), plan, fixturePaths(t), policy.Name)
			if err != nil {
				t.Fatal(err)
			}
			report, err := BuildFixtureReport(FixtureInput{Policy: policy, Plan: plan, Check: check, Execution: execution, Rollback: rollback})
			if err != nil {
				t.Fatal(err)
			}
			got, err := RenderFixtureReport(report)
			if err != nil {
				t.Fatal(err)
			}
			goldenPath := filepath.Join("testdata", fixture.name+"-evidence.json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch\n--- got\n%s\n--- want\n%s", got, want)
			}

			encoded := string(got)
			if strings.Contains(encoded, "fixture-secret-value") {
				t.Fatal("secret leaked into evidence")
			}
			if fixture.name == "portable-synthetic" {
				for _, forbidden := range []string{"galaxy", "overlays/galaxy", "/home/alex", "fingerprint.pam", "desktop.fixed-displays", "desktop.fixed-input-devices", "desktop.literal-home-paths", "desktop.cosmic-prune", "bootstrap.packages.install", "bootstrap.packages.remove", "bootstrap.boot.plymouth-edit"} {
					if strings.Contains(encoded, forbidden) {
						t.Fatalf("portable fixture leaks %q: %s", forbidden, encoded)
					}
				}
			}
		})
	}
}

func TestHardwareFreeFixtureConvergenceAndRollbackRefusal(t *testing.T) {
	policy := loadPolicy(t, "portable-synthetic")
	plan, err := cachyos.BuildHostPlan(policy, readyPlatformEvidence(t))
	if err != nil {
		t.Fatal(err)
	}
	execution, err := ExerciseFixturePlan(context.Background(), plan, fixturePaths(t), policy.Name)
	if err != nil {
		t.Fatal(err)
	}
	dryRun, converged := execution.Evidence()
	if !dryRun.StateUnchanged || !dryRun.AuditReceiptPublished || dryRun.MutationCount != 0 || dryRun.NetworkMutationCount != 0 || dryRun.LockAcquisitions != 0 {
		t.Fatalf("dry-run evidence = %#v", dryRun)
	}
	if converged.FirstNoChange || !converged.SecondNoChange || converged.MutationCount == 0 || converged.SecondMutationCount != 0 {
		t.Fatalf("convergence evidence = %#v", converged)
	}
	_, err = app.PlanRollbackInverse(app.ManagedFileRollback{Class: app.ManagedCreated, Target: "/fixture/managed", AfterHash: strings.Repeat("a", 64), Live: app.LiveFile{Exists: true, Hash: strings.Repeat("b", 64)}})
	if err == nil || !strings.Contains(err.Error(), "live content differs") {
		t.Fatalf("rollback mismatch = %v", err)
	}
}

func TestBuildFixtureReportRejectsUndrivenExecution(t *testing.T) {
	_, err := BuildFixtureReport(FixtureInput{})
	if err == nil || !strings.Contains(err.Error(), "not derived from the application path") {
		t.Fatalf("undriven fixture report error = %v", err)
	}
}

func fixturePaths(t *testing.T) statepath.Paths {
	t.Helper()
	runtimeDir := t.TempDir()
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return statepath.Paths{StateHome: filepath.Join(t.TempDir(), "state"), RuntimeDir: runtimeDir}
}

func loadPolicy(t *testing.T, host string) catalog.ResolvedHostPolicy {
	t.Helper()
	read := func(path string) catalog.Document {
		data, err := os.ReadFile(filepath.Join("testdata", "catalog", path))
		if err != nil {
			t.Fatal(err)
		}
		doc, err := catalog.DecodeDocument(data)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return doc
	}
	repository, err := catalog.NewRepository(read("global.yaml"), map[string]catalog.Document{"workstation": read("roles/workstation.yaml")}, map[string]catalog.Document{
		"galaxy":             read("hosts/galaxy-fixture.yaml"),
		"portable-synthetic": read("hosts/portable-synthetic.yaml"),
	}, assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := repository.Resolve(host)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func readyPlatformEvidence(t *testing.T) cachyos.PlatformEvidence {
	t.Helper()
	evidence := cachyos.PlatformEvidence{Capabilities: map[catalog.RiskCapability]cachyos.CapabilityEvidence{}, Bootstrap: cachyos.BootstrapObservation{
		InstalledPackages: map[string]string{"paru": "1", "cosmic-store": "1", "flatpak": "1", "zsh": "1", "google-chrome": "1", "firefox": "1", "plymouth": "1"},
		Boot:              cachyos.BootObservation{MkinitcpioHasPlymouth: true, GrubHasSplash: true, GrubGeneratorAvailable: true},
	}, Fingerprint: fingerprintFixtureEvidence(t), Desktop: cachyos.DesktopObservation{HomeRoot: "/fixture/home", UserName: "fixture"}}
	for _, capability := range catalog.RiskCapabilities() {
		evidence.Capabilities[capability] = cachyos.CapabilityEvidence{State: cachyos.EvidenceReady}
	}
	return evidence
}

func fingerprintFixtureEvidence(t *testing.T) cachyos.FingerprintObservation {
	t.Helper()
	const (
		sourceCommit = "8749008832ee1f313bfca4d3c04340df84b2bc27"
		pkgbuildSHA  = "094cdd3f61a0227c7eec5c2426a20885a35cf2bd23e39d713e903072a02797bf"
		patchSHA     = "dd248cb9225857385f32ce36da887cb039d5ec7bffd54f412a790e898349d03d"
		artifactSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	observation := cachyos.FingerprintObservation{
		BuildRoot: "/fixture/build/fingerprint",
		Pin: &cachyos.FingerprintResolvedPin{
			SourceCommit: sourceCommit, Pkgrel: 1, PKGBUILDSHA256: pkgbuildSHA, PatchSHA256: patchSHA,
			ArtifactName: "libfprint-egismoc-sdcp-git-r100.8749008-1-x86_64.pkg.tar.zst", ArtifactSHA256: artifactSHA, SourceDateEpoch: "1725148800",
		},
		Package: cachyos.FingerprintPackageObservation{Installed: true, Name: "libfprint-egismoc-sdcp-git", SourceCommit: sourceCommit, Pkgrel: 1},
		PAM:     make(map[string]cachyos.FingerprintFileObservation),
	}
	for _, name := range []string{"cosmic-greeter", "greetd", "polkit-1", "su", "su-l", "sudo", "system-local-login"} {
		path := "/etc/pam.d/" + name
		content, err := assets.FS.ReadFile("overlays/galaxy/etc/pam.d/" + name)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(content)
		observation.PAM[path] = cachyos.FingerprintFileObservation{
			Path: path, Exists: true, SHA256: hex.EncodeToString(digest[:]), Mode: 0o644,
			Ownership: cachyos.OwnershipAdopted, BackupPath: path + ".bak.alex-cachyos",
		}
	}
	return observation
}
