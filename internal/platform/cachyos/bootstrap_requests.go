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
	plan.ExplicitDelta = explicitDelta(want, plan.MissingWanted, installed)
	if err := plan.add(makeRequest("bootstrap.packages.install", "/usr/bin/pacman",
		append([]string{"-Syu", "--needed", "--noconfirm"}, plan.MissingWanted...), runner.ScopeSystem, runner.NetworkRequired, nil)); err != nil {
		return BootstrapRequestPlan{}, err
	}
	if len(plan.RemovalDelta) != 0 {
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
	if input.Boot.MkinitcpioHasPlymouth || input.Boot.GrubHasSplash {
		if err := plan.add(makeRequest("bootstrap.boot.edit", "/usr/bin/python3",
			[]string{"-", "/etc/mkinitcpio.conf", "/etc/default/grub"}, runner.ScopeSystem, runner.NetworkNone, []byte(bootEditScript))); err != nil {
			return BootstrapRequestPlan{}, err
		}
		if err := plan.add(makeRequest("bootstrap.boot.mkinitcpio", "/usr/bin/mkinitcpio", []string{"-P"}, runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
			return BootstrapRequestPlan{}, err
		}
		if input.Boot.GrubHasSplash {
			plan.GRUBPublish = &GRUBPublishDescriptor{SourcePath: grubStagePath, DestinationPath: grubConfigPath, SameDirectory: true, NoDirectOverwrite: true, Mode: 0644, GeneratorAvailable: input.Boot.GrubGeneratorAvailable, GenerationFailure: WarnOnly, Fallback: KeepExisting}
			if input.Boot.GrubGeneratorAvailable {
				if err := plan.add(makeRequest("bootstrap.boot.grub-generate", "/usr/bin/grub-mkconfig", []string{"-o", grubStagePath}, runner.ScopeSystem, runner.NetworkNone, nil)); err != nil {
					return BootstrapRequestPlan{}, err
				}
			}
		}
	}
	for _, service := range serviceRequests(input, installed) {
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
func serviceRequests(input BootstrapInputs, installed map[string]InstalledPackage) []ServiceObservation {
	services := append([]ServiceObservation(nil), input.Services...)
	if len(services) == 0 {
		for _, candidate := range []struct{ packageName, unit string }{{"ananicy-cpp", "ananicy-cpp.service"}, {"ufw", "ufw.service"}} {
			if _, ok := installed[candidate.packageName]; ok {
				services = append(services, ServiceObservation{Unit: candidate.unit, Installed: true})
			}
		}
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Unit < services[j].Unit })
	out, last := make([]ServiceObservation, 0, len(services)), ""
	for _, service := range services {
		if service.Unit == last {
			continue
		}
		last = service.Unit
		if (service.Unit == "ananicy-cpp.service" || service.Unit == "ufw.service") && service.Installed && !(service.Enabled && service.Active) {
			out = append(out, service)
		}
	}
	return out
}
func isLTSPackage(name string) bool { return strings.Contains(strings.ToLower(name), "-lts") }
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

const bootEditScript = `import os, pathlib, re, sys
if len(sys.argv) != 3 or any(not pathlib.Path(x).is_absolute() for x in sys.argv[1:]): raise SystemExit("two absolute boot targets required")
patterns = (re.compile(rb"^([ \t]*HOOKS[ \t]*=[ \t]*)(.*)$"), re.compile(rb"^([ \t]*GRUB_CMDLINE_LINUX(?:_DEFAULT)?[ \t]*=[ \t]*)(.*)$"))
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
def edit(data, pattern, token):
    rows = [(*ending(line),) for line in data.splitlines(keepends=True)]
    matches = [(i, pattern.match(body), body, eol) for i, (body, eol) in enumerate(rows) if pattern.match(body)]
    targets = [item for item in matches if re.search(rb"(?<![A-Za-z0-9_-])" + token + rb"(?![A-Za-z0-9_-])", item[2].partition(b"#")[0])]
    if not matches or len(targets) > 1: raise SystemExit("boot assignment missing or ambiguous")
    if not targets: return data
    i, match, body, eol = targets[0]
    value, changed = remove(match.group(2), token)
    rows[i] = (match.group(1) + value, eol)
    return b"".join(body + eol for body, eol in rows) if changed else data
pending = []
for i, raw in enumerate(sys.argv[1:]):
    path = pathlib.Path(raw)
    if not path.is_file(): raise SystemExit("boot target missing")
    before = path.read_bytes()
    after = edit(before, patterns[i], (b"plymouth", b"splash")[i])
    pending.append((path, before, after))
for path, before, after in pending:
    if after != before:
        temp = path.with_name(path.name + ".alex-cachyos.tmp")
        temp.write_bytes(after); os.chmod(temp, path.stat().st_mode & 0o777); os.replace(temp, path)
`
