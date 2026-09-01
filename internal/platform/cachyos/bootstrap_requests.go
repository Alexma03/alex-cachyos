package cachyos

import (
	"alex-cachyos/internal/runner"
	"alex-cachyos/templates"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

type InstalledPackage struct {
	Name     string
	Explicit bool
}
type ServiceObservation struct {
	Unit                       string
	Installed, Enabled, Active bool
}
type BootObservation struct {
	MkinitcpioHasPlymouth                 bool
	GrubHasSplash, GrubGeneratorAvailable bool
}
type BootstrapInputs struct {
	InstalledPackages []InstalledPackage
	FirefoxI18N       []string
	Services          []ServiceObservation
	ChromeInstalled   bool
	Boot              BootObservation
}
type GenerationFailurePolicy string
type FallbackPolicy string

const WarnOnly GenerationFailurePolicy = "warn-only"
const KeepExisting FallbackPolicy = "keep-existing"

type GRUBPublishDescriptor struct {
	SourcePath, DestinationPath      string
	SameDirectory, NoDirectOverwrite bool
	Mode                             uint32
	GeneratorAvailable               bool
	GenerationFailure                GenerationFailurePolicy
	Fallback                         FallbackPolicy
}
type AtomicPublishDescriptor = GRUBPublishDescriptor
type BootstrapRequestPlan struct {
	Requests                                   []runner.CommandRequest
	MissingWanted, RemovalDelta, ExplicitDelta []string
	GRUBPublish                                *GRUBPublishDescriptor
}

const (
	grubStagePath  = "/boot/grub/.grub.cfg.alex-cachyos.staged"
	grubConfigPath = "/boot/grub/grub.cfg"
)

func BuildBootstrapRequestPlan(input BootstrapInputs) (BootstrapRequestPlan, error) {
	return buildBootstrapRequestPlan(input, allBootstrapAuthorizations())
}

func buildBootstrapRequestPlan(input BootstrapInputs, authorization bootstrapAuthorizations) (BootstrapRequestPlan, error) {
	want, err := embeddedPackageList("bootstrap/packages.want")
	if err != nil {
		return BootstrapRequestPlan{}, err
	}
	remove, err := embeddedPackageList("bootstrap/packages.remove")
	if err != nil {
		return BootstrapRequestPlan{}, err
	}
	installed, err := installedIndex(input)
	if err != nil {
		return BootstrapRequestPlan{}, err
	}
	plan := BootstrapRequestPlan{}
	for _, name := range want {
		if _, ok := installed[name]; !ok {
			plan.MissingWanted = append(plan.MissingWanted, name)
		}
	}
	plan.RemovalDelta = removalDelta(remove, input.FirefoxI18N, installed)
	explicitTargets := append(append([]string(nil), want...), "google-chrome")
	missingForExplicit := plan.MissingWanted
	if !authorization.SystemUpdate {
		missingForExplicit = nil
	}
	plan.ExplicitDelta = explicitDelta(explicitTargets, missingForExplicit, installed)
	if authorization.SystemUpdate {
		if err := plan.add(makeRequest("bootstrap.packages.install", "/usr/bin/pacman",
			append([]string{"-Syu", "--needed", "--noconfirm"}, plan.MissingWanted...), runner.ScopeSystem, runner.NetworkRequired, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	if authorization.PackageRemoval && len(plan.RemovalDelta) != 0 {
		if err := plan.add(makeRequest("bootstrap.packages.remove", "/usr/bin/pacman",
			append([]string{"-Rns", "--noconfirm"}, plan.RemovalDelta...), runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	if len(plan.ExplicitDelta) != 0 {
		if err := plan.add(makeRequest("bootstrap.packages.explicit", "/usr/bin/pacman",
			append([]string{"-D", "--asexplicit"}, plan.ExplicitDelta...), runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	if authorization.BootMutation && input.Boot.MkinitcpioHasPlymouth {
		if err := plan.add(makeRequest(bootstrapBootPlymouthEdit, "/usr/bin/python3",
			[]string{"-", mkinitcpioConfigPath}, runner.ScopeSystem, runner.NetworkNone, []byte(plymouthEditScript))); err != nil {
			return BootstrapRequestPlan{}, err
		}
		if err := plan.add(makeRequest(bootstrapBootMkinitcpio, "/usr/bin/mkinitcpio", []string{"-P"}, runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	if authorization.BootMutation && input.Boot.GrubHasSplash {
		if err := plan.add(makeRequest(bootstrapBootGRUBEdit, "/usr/bin/python3",
			[]string{"-", grubDefaultPath}, runner.ScopeSystem, runner.NetworkNone, []byte(grubEditScript))); err != nil {
			return BootstrapRequestPlan{}, err
		}
		plan.GRUBPublish = &GRUBPublishDescriptor{SourcePath: grubStagePath, DestinationPath: grubConfigPath, SameDirectory: true, NoDirectOverwrite: true, Mode: 0644, GeneratorAvailable: input.Boot.GrubGeneratorAvailable, GenerationFailure: WarnOnly, Fallback: KeepExisting}
		if input.Boot.GrubGeneratorAvailable {
			if err := plan.add(makeRequest(bootstrapBootGRUBGenerate, "/usr/bin/grub-mkconfig", []string{"-o", grubStagePath}, runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
				return BootstrapRequestPlan{}, err
			}
		}
	}
	services, err := serviceRequests(input, installed)
	if err != nil {
		return BootstrapRequestPlan{}, err
	}
	for _, service := range services {
		name := strings.TrimSuffix(service.Unit, ".service")
		if err := plan.add(makeRequest("bootstrap.service."+name, "/usr/bin/systemctl", []string{"enable", "--now", service.Unit}, runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	if !input.ChromeInstalled {
		if _, ok := installed["google-chrome"]; ok {
			input.ChromeInstalled = true
		}
	}
	if !input.ChromeInstalled {
		if err := plan.add(makeRequest("bootstrap.chrome.install", "/usr/bin/paru", []string{"-S", "--needed", "--noconfirm", "google-chrome"}, runner.ScopeUser, runner.NetworkRequired, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
	}
	return plan, nil
}
func (p *BootstrapRequestPlan) add(request runner.CommandRequest, err error) error {
	if err != nil {
		return err
	}
	if err := runner.ValidateCommandRequest(request); err != nil {
		return err
	}
	request.Argv = append([]string(nil), request.Argv...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	p.Requests = append(p.Requests, request)
	return nil
}
func makeRequest(operation, executable string, argv []string, scope runner.Scope, network runner.NetworkPolicy, stdin []byte) (runner.CommandRequest, error) {
	r := runner.CommandRequest{Operation: operation, Executable: executable, Argv: argv, Cwd: "/", Stdin: stdin, Scope: scope, Network: network, OutputPolicy: runner.OutputCaptureRedacted, Timeout: runner.MaxTimeout, OutputLimit: 1 << 20}
	return r, runner.ValidateCommandRequest(r)
}
func embeddedPackageList(name string) ([]string, error) {
	data, err := fs.ReadFile(templates.FS, name)
	if err != nil {
		return nil, fmt.Errorf("read embedded %s: %w", name, err)
	}
	var values []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !validPackageName(line) {
			return nil, fmt.Errorf("invalid package name in %s: %q", name, line)
		}
		values = append(values, line)
	}
	return sortedUnique(values), nil
}
func installedIndex(input BootstrapInputs) (map[string]InstalledPackage, error) {
	out := make(map[string]InstalledPackage, len(input.InstalledPackages)+len(input.FirefoxI18N))
	for _, item := range input.InstalledPackages {
		if !validPackageName(item.Name) {
			return nil, fmt.Errorf("invalid installed package name: %q", item.Name)
		}
		if _, exists := out[item.Name]; exists {
			return nil, fmt.Errorf("duplicate installed package: %q", item.Name)
		}
		out[item.Name] = item
	}
	for _, name := range sortedUnique(input.FirefoxI18N) {
		if !strings.HasPrefix(name, "firefox-i18n-") || len(name) == len("firefox-i18n-") || !validPackageName(name) {
			return nil, fmt.Errorf("invalid firefox-i18n package name: %q", name)
		}
		if _, exists := out[name]; !exists {
			out[name] = InstalledPackage{Name: name}
		}
	}
	return out, nil
}
func removalDelta(static, dynamic []string, installed map[string]InstalledPackage) []string {
	values := make([]string, 0, len(static)+len(dynamic))
	for _, name := range append(append([]string(nil), static...), dynamic...) {
		if _, ok := installed[name]; ok && !isLTSPackage(name) {
			values = append(values, name)
		}
	}
	return sortedUnique(values)
}
func explicitDelta(want, missing []string, installed map[string]InstalledPackage) []string {
	values := append([]string(nil), missing...)
	for _, name := range want {
		if item, ok := installed[name]; ok && !item.Explicit {
			values = append(values, name)
		}
	}
	return sortedUnique(values)
}
func serviceRequests(input BootstrapInputs, installed map[string]InstalledPackage) ([]ServiceObservation, error) {
	present := make(map[string]bool, len(installed))
	for name := range installed {
		present[name] = true
	}
	services, err := normalizeServiceObservations(input.Services, present)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceObservation, 0, len(services))
	for _, service := range services {
		if service.Installed && !(service.Enabled && service.Active) {
			out = append(out, service)
		}
	}
	return out, nil
}

var cachyosLTSNames = []string{"linux-cachyos-lts", "linux-cachyos-lts-headers"}

func isLTSPackage(name string) bool {
	switch name {
	case "linux-cachyos-lts", "linux-cachyos-lts-headers":
		return true
	default:
		return false
	}
}

func normalizeServiceObservations(observed []ServiceObservation, installed map[string]bool) ([]ServiceObservation, error) {
	services := make([]ServiceObservation, 0, len(observed)+len(cachyosManagedServices))
	for _, service := range observed {
		unit := normalizeUnit(service.Unit)
		packageName := servicePackageName(unit)
		if packageName == "" {
			continue
		}
		services = append(services, ServiceObservation{
			Unit:      unit,
			Installed: installed[packageName],
			Enabled:   service.Enabled,
			Active:    service.Active,
		})
	}
	sort.Slice(services, func(i, j int) bool {
		if services[i].Unit != services[j].Unit {
			return services[i].Unit < services[j].Unit
		}
		if services[i].Enabled != services[j].Enabled {
			return !services[i].Enabled
		}
		return !services[i].Active
	})
	for i := 1; i < len(services); i++ {
		if services[i-1].Unit != services[i].Unit {
			continue
		}
		if services[i-1].Enabled != services[i].Enabled || services[i-1].Active != services[i].Active {
			return nil, fmt.Errorf("contradictory service observations for %q", services[i].Unit)
		}
		return nil, fmt.Errorf("duplicate service observation for %q", services[i].Unit)
	}

	seen := make(map[string]bool, len(services))
	for _, service := range services {
		seen[service.Unit] = true
	}
	for _, candidate := range cachyosManagedServices {
		if installed[candidate.packageName] && !seen[candidate.unit] {
			services = append(services, ServiceObservation{Unit: candidate.unit, Installed: true})
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Unit < services[j].Unit })
	return append([]ServiceObservation(nil), services...), nil
}

var cachyosManagedServices = []struct{ packageName, unit string }{
	{packageName: "ananicy-cpp", unit: "ananicy-cpp.service"},
	{packageName: "ufw", unit: "ufw.service"},
}

func validPackageName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("@._+:-", c) {
			continue
		}
		return false
	}
	return true
}
func sortedUnique(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	unique := out[:0]
	for _, value := range out {
		if len(unique) == 0 || value != unique[len(unique)-1] {
			unique = append(unique, value)
		}
	}
	return unique
}

const plymouthEditScript = `import os, pathlib, re, sys
if len(sys.argv) != 2 or not pathlib.Path(sys.argv[1]).is_absolute(): raise SystemExit("one absolute mkinitcpio target required")
path = pathlib.Path(sys.argv[1])
pattern = re.compile(rb"^([ \t]*HOOKS[ \t]*=[ \t]*)(.*)$")
def ending(line):
    for e in (b"\r\n", b"\n", b"\r"):
        if line.endswith(e): return line[:-len(e)], e
    return line, b""
def remove(value, token):
    head, mark, tail = value.partition(b"#")
    found = list(re.finditer(rb"(?<![A-Za-z0-9_-])" + token + rb"(?![A-Za-z0-9_-])", head))
    if not found: return value, False
    out = bytearray(head)
    for match in reversed(found):
        left, right = match.span()
        if right < len(out) and out[right:right + 1] in (b" ", b"\t"): right += 1
        elif left and out[left - 1:left] in (b" ", b"\t"): left -= 1
        del out[left:right]
    return bytes(out) + mark + tail, True
def edit(data):
    rows = [(*ending(line),) for line in data.splitlines(keepends=True)]
    matches = [(i, pattern.match(body), body, eol) for i, (body, eol) in enumerate(rows) if pattern.match(body)]
    targets = [item for item in matches if re.search(rb"(?<![A-Za-z0-9_-])plymouth(?![A-Za-z0-9_-])", item[2].partition(b"#")[0])]
    if not matches or len(targets) > 1: raise SystemExit("mkinitcpio HOOKS assignment missing or ambiguous")
    if not targets: return data
    i, match, body, eol = targets[0]
    value, changed = remove(match.group(2), b"plymouth")
    rows[i] = (match.group(1) + value, eol)
    return b"".join(body + eol for body, eol in rows) if changed else data
if not path.is_file(): raise SystemExit("mkinitcpio target missing")
before = path.read_bytes()
after = edit(before)
if after != before:
    temp = path.with_name(path.name + ".alex-cachyos.tmp")
    temp.write_bytes(after); os.chmod(temp, path.stat().st_mode & 0o777); os.replace(temp, path)
`

const grubEditScript = `import os, pathlib, re, sys
if len(sys.argv) != 2 or not pathlib.Path(sys.argv[1]).is_absolute(): raise SystemExit("one absolute GRUB target required")
path = pathlib.Path(sys.argv[1])
pattern = re.compile(rb"^([ \t]*GRUB_CMDLINE_LINUX(?:_DEFAULT)?[ \t]*=[ \t]*)(.*)$")
def ending(line):
    for e in (b"\r\n", b"\n", b"\r"):
        if line.endswith(e): return line[:-len(e)], e
    return line, b""
def remove(value, token):
    head, mark, tail = value.partition(b"#")
    found = list(re.finditer(rb"(?<![A-Za-z0-9_-])" + token + rb"(?![A-Za-z0-9_-])", head))
    if not found: return value, False
    out = bytearray(head)
    for match in reversed(found):
        left, right = match.span()
        if right < len(out) and out[right:right + 1] in (b" ", b"\t"): right += 1
        elif left and out[left - 1:left] in (b" ", b"\t"): left -= 1
        del out[left:right]
    return bytes(out) + mark + tail, True
def edit(data):
    rows = [(*ending(line),) for line in data.splitlines(keepends=True)]
    matches = [(i, pattern.match(body), body, eol) for i, (body, eol) in enumerate(rows) if pattern.match(body)]
    targets = [item for item in matches if re.search(rb"(?<![A-Za-z0-9_-])splash(?![A-Za-z0-9_-])", item[2].partition(b"#")[0])]
    if not matches or len(targets) > 1: raise SystemExit("GRUB command-line assignment missing or ambiguous")
    if not targets: return data
    i, match, body, eol = targets[0]
    value, changed = remove(match.group(2), b"splash")
    rows[i] = (match.group(1) + value, eol)
    return b"".join(body + eol for body, eol in rows) if changed else data
if not path.is_file(): raise SystemExit("GRUB target missing")
before = path.read_bytes()
after = edit(before)
if after != before:
    temp = path.with_name(path.name + ".alex-cachyos.tmp")
    temp.write_bytes(after); os.chmod(temp, path.stat().st_mode & 0o777); os.replace(temp, path)
`
