// Package pi owns the target-independent Pi desired-state model. It accepts
// catalog authority and typed observations, but never reads a live Pi home.
package pi

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"alex-cachyos/internal/catalog"
)

const gentlePiPackageName = "gentle-pi"

var requiredNPMPackages = []string{
	"@juicesharp/rpiv-ask-user-question",
	"@juicesharp/rpiv-todo",
	"gentle-engram",
	"pi-antigravity",
	"pi-btw",
	"pi-commandcode-provider",
	"pi-mcp-adapter",
	"pi-subagents-j0k3r",
	"pi-web-access",
}

var (
	ErrPackageAuthority = errors.New("invalid Pi package authority")
	ErrPackageProbe     = errors.New("invalid Pi package observation")
)

type PackageSource string

const (
	PackageNPM       PackageSource = "npm"
	PackageLocalPath PackageSource = "local-path"
)

type PackageInstallMode string

const InstallExactCatalogPin PackageInstallMode = "exact-catalog-pin"

type PackageLayout string

const (
	LayoutNone     PackageLayout = "none"
	LayoutAgent    PackageLayout = "agent"
	LayoutLegacy   PackageLayout = "legacy"
	LayoutMultiple PackageLayout = "multiple"
)

type CheckoutReference struct {
	Name     string
	Path     string
	ReadOnly bool
	Pin      catalog.CheckoutPin
}

type PackagePlanInput struct {
	Pins           *catalog.Pins
	SettingsPath   string
	Checkout       CheckoutReference
	IntendedLayout PackageLayout
}

type DesiredPackage struct {
	Name           string
	Source         PackageSource
	Spec           string
	DesiredVersion string
	ResolvedPath   string
	CheckoutCommit string
}

type PackageInstall struct {
	Mode         PackageInstallMode
	Name         string
	Source       PackageSource
	Spec         string
	ResolvedPath string
}

type PackagePlan struct {
	SettingsPath   string
	HomeRoot       string
	IntendedLayout PackageLayout
	Desired        []DesiredPackage
	Installs       []PackageInstall
}

func RequiredPackageNames() []string {
	names := append([]string(nil), requiredNPMPackages...)
	names = append(names, gentlePiPackageName)
	sort.Strings(names)
	return names
}

func BuildPackagePlan(input PackagePlanInput) (PackagePlan, error) {
	home, err := piHomeFromSettings(input.SettingsPath)
	if err != nil {
		return PackagePlan{}, err
	}
	if input.IntendedLayout != LayoutAgent && input.IntendedLayout != LayoutLegacy {
		return PackagePlan{}, packageAuthority("intended package layout must be agent or legacy")
	}
	if input.Pins == nil || len(input.Pins.NPM) != len(requiredNPMPackages) || len(input.Pins.LocalPathPackages) != 1 {
		return PackagePlan{}, packageAuthority("catalog package inventory is incomplete or contains extras")
	}

	desired := make([]DesiredPackage, 0, len(requiredNPMPackages)+1)
	for _, name := range requiredNPMPackages {
		version, ok := input.Pins.NPM[name]
		if !ok {
			return PackagePlan{}, packageAuthority("catalog package inventory is incomplete")
		}
		if err := catalog.ValidateNpmPin(name, catalog.NpmPin{Name: name, Version: version}); err != nil {
			return PackagePlan{}, packageAuthority("catalog npm pin is not exact")
		}
		desired = append(desired, DesiredPackage{Name: name, Source: PackageNPM, Spec: "npm:" + name + "@" + version, DesiredVersion: version})
	}
	for name := range input.Pins.NPM {
		if !containsPackage(requiredNPMPackages, name) {
			return PackagePlan{}, packageAuthority("catalog package inventory contains an unknown package")
		}
	}

	localReference, ok := input.Pins.LocalPathPackages[gentlePiPackageName]
	if !ok || input.Checkout.Name != gentlePiPackageName || !input.Checkout.ReadOnly {
		return PackagePlan{}, packageAuthority("local gentle-pi requires its named read-only checkout")
	}
	if err := catalog.ValidateLocalPiPackagePin(gentlePiPackageName, localReference); err != nil {
		return PackagePlan{}, packageAuthority("local gentle-pi reference is invalid")
	}
	if err := catalog.ValidateSourceCheckoutPin(gentlePiPackageName, input.Checkout.Pin, catalog.CheckoutDestination{Path: input.Checkout.Path}); err != nil {
		return PackagePlan{}, packageAuthority("gentle-pi checkout pin is invalid")
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(input.SettingsPath), filepath.FromSlash(localReference)))
	if !canonicalAbsolute(input.Checkout.Path) || resolved != input.Checkout.Path {
		return PackagePlan{}, packageAuthority("local gentle-pi reference does not resolve to the selected checkout")
	}
	desired = append(desired, DesiredPackage{
		Name: gentlePiPackageName, Source: PackageLocalPath, Spec: localReference,
		ResolvedPath: resolved, CheckoutCommit: input.Checkout.Pin.Commit,
	})
	sort.Slice(desired, func(i, j int) bool { return desired[i].Name < desired[j].Name })

	plan := PackagePlan{SettingsPath: input.SettingsPath, HomeRoot: home, IntendedLayout: input.IntendedLayout, Desired: cloneDesiredPackages(desired)}
	plan.Installs = make([]PackageInstall, len(desired))
	for i, item := range desired {
		plan.Installs[i] = PackageInstall{Mode: InstallExactCatalogPin, Name: item.Name, Source: item.Source, Spec: item.Spec, ResolvedPath: item.ResolvedPath}
	}
	return clonePackagePlan(plan), nil
}

type PackageProbeRequest struct {
	Layout PackageLayout
	Root   string
	Names  []string
}

type PackageProbe interface {
	Probe(context.Context, PackageProbeRequest) (map[string]string, error)
}

type PackageLayoutObservation struct {
	Layout                    PackageLayout
	Root                      string
	ResolvedInstalledVersions map[string]string
}

type PackageObservation struct {
	ActiveLayout              PackageLayout
	Layouts                   []PackageLayoutObservation
	ResolvedInstalledVersions map[string]string
}

func ObservePackages(ctx context.Context, plan PackagePlan, probe PackageProbe) (PackageObservation, error) {
	if probe == nil || !validPackagePlan(plan) {
		return PackageObservation{}, ErrPackageProbe
	}
	if ctx == nil {
		ctx = context.Background()
	}
	names := packageNamesFromPlan(plan)
	definitions := []struct {
		layout PackageLayout
		root   string
	}{
		{LayoutAgent, filepath.Join(plan.HomeRoot, ".pi", "agent", "npm")},
		{LayoutLegacy, filepath.Join(plan.HomeRoot, ".pi", "npm")},
	}
	observation := PackageObservation{ActiveLayout: LayoutNone, Layouts: make([]PackageLayoutObservation, 0, len(definitions))}
	active := make([]PackageLayoutObservation, 0, 2)
	for _, definition := range definitions {
		versions, err := probe.Probe(ctx, PackageProbeRequest{Layout: definition.layout, Root: definition.root, Names: append([]string(nil), names...)})
		if err != nil {
			return PackageObservation{}, fmt.Errorf("%w: probe %s layout", ErrPackageProbe, definition.layout)
		}
		if err := validateObservedVersions(plan, versions); err != nil {
			return PackageObservation{}, err
		}
		item := PackageLayoutObservation{Layout: definition.layout, Root: definition.root, ResolvedInstalledVersions: cloneStringMap(versions)}
		observation.Layouts = append(observation.Layouts, item)
		if len(versions) != 0 {
			active = append(active, item)
		}
	}
	switch len(active) {
	case 0:
		observation.ActiveLayout = LayoutNone
		observation.ResolvedInstalledVersions = map[string]string{}
	case 1:
		observation.ActiveLayout = active[0].Layout
		observation.ResolvedInstalledVersions = cloneStringMap(active[0].ResolvedInstalledVersions)
	default:
		observation.ActiveLayout = LayoutMultiple
		observation.ResolvedInstalledVersions = map[string]string{}
	}
	return clonePackageObservation(observation), nil
}

type PackageDrift struct {
	Name     string
	Desired  string
	Resolved string
	Layout   PackageLayout
}

type PackageCheckReport struct {
	Drift       bool
	LayoutDrift bool
	Packages    []PackageDrift
}

func CheckPackageDrift(plan PackagePlan, observation PackageObservation) PackageCheckReport {
	report := PackageCheckReport{LayoutDrift: observation.ActiveLayout != plan.IntendedLayout}
	for _, desired := range plan.Desired {
		resolved, present := observation.ResolvedInstalledVersions[desired.Name]
		if !present || desired.Source == PackageNPM && resolved != desired.DesiredVersion {
			report.Packages = append(report.Packages, PackageDrift{
				Name: desired.Name, Desired: desiredPin(desired), Resolved: resolved, Layout: observation.ActiveLayout,
			})
		}
	}
	report.Drift = report.LayoutDrift || len(report.Packages) != 0
	return report
}

func desiredPin(item DesiredPackage) string {
	if item.Source == PackageNPM {
		return item.DesiredVersion
	}
	return item.Spec
}

func piHomeFromSettings(path string) (string, error) {
	if !canonicalAbsolute(path) || filepath.Base(path) != "settings.json" {
		return "", packageAuthority("settings path must be canonical and absolute")
	}
	agent := filepath.Dir(path)
	if filepath.Base(agent) != "agent" || filepath.Base(filepath.Dir(agent)) != ".pi" {
		return "", packageAuthority("settings path must identify .pi/agent/settings.json")
	}
	home := filepath.Dir(filepath.Dir(agent))
	if !canonicalAbsolute(home) {
		return "", packageAuthority("settings path does not resolve a safe home")
	}
	return home, nil
}

func canonicalAbsolute(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path && path != string(filepath.Separator) && !strings.ContainsAny(path, "\x00\r\n")
}

func packageAuthority(message string) error {
	return fmt.Errorf("%w: %s", ErrPackageAuthority, message)
}

func containsPackage(names []string, target string) bool {
	index := sort.SearchStrings(names, target)
	return index < len(names) && names[index] == target
}

func packageNamesFromPlan(plan PackagePlan) []string {
	names := make([]string, len(plan.Desired))
	for i, item := range plan.Desired {
		names[i] = item.Name
	}
	return names
}

func validateObservedVersions(plan PackagePlan, versions map[string]string) error {
	desired := make(map[string]DesiredPackage, len(plan.Desired))
	for _, item := range plan.Desired {
		desired[item.Name] = item
	}
	for name, version := range versions {
		item, ok := desired[name]
		if !ok {
			return fmt.Errorf("%w: observer returned an undeclared package", ErrPackageProbe)
		}
		if item.Source == PackageNPM {
			if err := catalog.ValidateNpmPin(name, catalog.NpmPin{Name: name, Version: version}); err != nil {
				return fmt.Errorf("%w: observer returned an invalid resolved version", ErrPackageProbe)
			}
		}
	}
	return nil
}

func validPackagePlan(plan PackagePlan) bool {
	if !canonicalAbsolute(plan.HomeRoot) || !canonicalAbsolute(plan.SettingsPath) || (plan.IntendedLayout != LayoutAgent && plan.IntendedLayout != LayoutLegacy) || len(plan.Desired) != len(requiredNPMPackages)+1 {
		return false
	}
	return true
}

func cloneDesiredPackages(values []DesiredPackage) []DesiredPackage {
	return append([]DesiredPackage(nil), values...)
}

func clonePackagePlan(plan PackagePlan) PackagePlan {
	result := plan
	result.Desired = cloneDesiredPackages(plan.Desired)
	result.Installs = make([]PackageInstall, len(plan.Installs))
	copy(result.Installs, plan.Installs)
	return result
}

func clonePackageObservation(observation PackageObservation) PackageObservation {
	result := observation
	result.ResolvedInstalledVersions = cloneStringMap(observation.ResolvedInstalledVersions)
	result.Layouts = make([]PackageLayoutObservation, len(observation.Layouts))
	for i, item := range observation.Layouts {
		result.Layouts[i] = PackageLayoutObservation{Layout: item.Layout, Root: item.Root, ResolvedInstalledVersions: cloneStringMap(item.ResolvedInstalledVersions)}
	}
	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for name, value := range values {
		result[name] = value
	}
	return result
}
