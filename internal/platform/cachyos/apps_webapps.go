package cachyos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"alex-cachyos/internal/assets"
	"alex-cachyos/internal/planner"
)

const (
	webAppsListAsset    = "templates/apps/webapps.list"
	webAppLauncherAsset = "data/bin/alex-cachyos-webapp-launch"

	webAppLauncherName            = "alex-cachyos-webapp-launch"
	webAppApplicationsRelative    = "share/applications"
	webAppIconRelative            = "share/applications/icons"
	webAppDesktopExtension        = ".desktop"
	webAppIconExtension           = ".png"
	filesystemComponentLimitBytes = 255

	webAppExecutableMode uint32 = 0o755
	webAppIconMode       uint32 = 0o644
)

const (
	// MaxWebAppIconBytes bounds the encoded response before it is decoded.
	MaxWebAppIconBytes int64 = 1 << 20
	// MaxWebAppIconDimension prevents a small compressed image from requesting an
	// unreasonable decoded dimension.
	MaxWebAppIconDimension = 4096
	// MaxWebAppIconPixels bounds the decoded pixel count in addition to each axis.
	MaxWebAppIconPixels int64 = 16 * 1024 * 1024
	// MaxWebAppFilenameComponentBytes is the filesystem component limit used when
	// validating generated desktop and icon names.
	MaxWebAppFilenameComponentBytes = filesystemComponentLimitBytes
)

const (
	// WebAppLauncherMode is the required mode for the copied launcher.
	WebAppLauncherMode uint32 = webAppExecutableMode
	// WebAppIconMode is the mode for fetched PNG icon files.
	WebAppIconMode uint32 = webAppIconMode
)

var (
	// ErrInvalidWebApp identifies a malformed or unsafe list entry.
	ErrInvalidWebApp = errors.New("invalid webapp definition")
	// ErrInvalidWebAppRoots identifies a path root that is not a clean absolute
	// containment boundary.
	ErrInvalidWebAppRoots = errors.New("invalid webapp path roots")
	// ErrWebAppRootsRequired prevents callers from silently rendering a path using
	// an ambient home directory or a tilde-prefixed placeholder.
	ErrWebAppRootsRequired = errors.New("webapp path roots are required")
	// ErrWebAppIconFetch identifies a webapp for which both icon attempts failed.
	ErrWebAppIconFetch = errors.New("webapp icon fetch failed")
	// ErrWebAppIconFetcher identifies a missing injected icon-fetch seam.
	ErrWebAppIconFetcher = errors.New("webapp icon fetcher unavailable")
	// ErrInvalidWebAppPNG identifies bytes that are not a bounded, fully decoded
	// PNG image.
	ErrInvalidWebAppPNG = errors.New("invalid webapp PNG icon")
	// ErrInvalidFileOwnership identifies an observation that cannot safely bind an
	// inverse for a managed file.
	ErrInvalidFileOwnership = errors.New("invalid managed-file ownership observation")
)

// WebAppRoots are the two caller-owned absolute path boundaries used for a plan.
// InstallRoot is the concrete directory containing the generated bin/ and
// share/applications/ trees and must remain inside HomeRoot. No path is inferred
// from the process environment.
type WebAppRoots struct {
	HomeRoot    string
	InstallRoot string
}

// These aliases keep the path-root terminology available to callers without
// introducing another mutable representation.
type WebAppPathRoots = WebAppRoots
type WebAppInstallRoots = WebAppRoots
type WebAppPathConfig = WebAppRoots
type WebAppPaths = WebAppRoots

type resolvedWebAppPaths struct {
	launcher     string
	applications string
	icons        string
}

// NewWebAppRoots validates and returns the roots used by all path-bearing plan
// constructors.
func NewWebAppRoots(homeRoot, installRoot string) (WebAppRoots, error) {
	roots := WebAppRoots{HomeRoot: homeRoot, InstallRoot: installRoot}
	if _, err := validateWebAppRoots(roots); err != nil {
		return WebAppRoots{}, err
	}
	return roots, nil
}

// ValidateWebAppRoots is the functional spelling of WebAppRoots.Validate.
func ValidateWebAppRoots(homeRoot, installRoot string) error {
	_, err := NewWebAppRoots(homeRoot, installRoot)
	return err
}

// NewWebAppPaths is the path-oriented spelling of NewWebAppRoots.
func NewWebAppPaths(homeRoot, installRoot string) (WebAppPaths, error) {
	return NewWebAppRoots(homeRoot, installRoot)
}

// ValidateWebAppPaths is the path-oriented spelling of ValidateWebAppRoots.
func ValidateWebAppPaths(homeRoot, installRoot string) error {
	return ValidateWebAppRoots(homeRoot, installRoot)
}

// Validate checks that both roots are clean absolute paths and that the install
// boundary is contained by the supplied home boundary.
func (r WebAppRoots) Validate() error {
	_, err := validateWebAppRoots(r)
	return err
}

// WebAppLauncherPath resolves the absolute launcher path for roots.
func WebAppLauncherPath(roots WebAppRoots) (string, error) {
	paths, err := validateWebAppRoots(roots)
	if err != nil {
		return "", err
	}
	return paths.launcher, nil
}

// WebAppApplicationsPath resolves the absolute desktop-entry directory for roots.
func WebAppApplicationsPath(roots WebAppRoots) (string, error) {
	paths, err := validateWebAppRoots(roots)
	if err != nil {
		return "", err
	}
	return paths.applications, nil
}

// WebAppIconDirectory resolves the absolute icon directory for roots.
func WebAppIconDirectory(roots WebAppRoots) (string, error) {
	paths, err := validateWebAppRoots(roots)
	if err != nil {
		return "", err
	}
	return paths.icons, nil
}

func validateWebAppRoots(roots WebAppRoots) (resolvedWebAppPaths, error) {
	home, err := validateAbsoluteCleanRoot("home root", roots.HomeRoot)
	if err != nil {
		return resolvedWebAppPaths{}, err
	}
	install, err := validateAbsoluteCleanRoot("install root", roots.InstallRoot)
	if err != nil {
		return resolvedWebAppPaths{}, err
	}
	if !pathContainedBy(home, install) {
		return resolvedWebAppPaths{}, fmt.Errorf("%w: install root is outside home root", ErrInvalidWebAppRoots)
	}

	launcher, err := joinContained(install, "bin", webAppLauncherName)
	if err != nil {
		return resolvedWebAppPaths{}, err
	}
	applications, err := joinContained(install, webAppApplicationsRelative)
	if err != nil {
		return resolvedWebAppPaths{}, err
	}
	icons, err := joinContained(install, webAppIconRelative)
	if err != nil {
		return resolvedWebAppPaths{}, err
	}
	for label, path := range map[string]string{
		"launcher path":       launcher,
		"applications path":   applications,
		"icon directory path": icons,
	} {
		if !pathContainedBy(home, path) || !pathContainedBy(install, path) {
			return resolvedWebAppPaths{}, fmt.Errorf("%w: %s escapes a supplied root", ErrInvalidWebAppRoots, label)
		}
	}
	return resolvedWebAppPaths{launcher: launcher, applications: applications, icons: icons}, nil
}

func validateAbsoluteCleanRoot(label, root string) (string, error) {
	if root == "" || !utf8.ValidString(root) || hasControl(root) || !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: %s must be an absolute UTF-8 path", ErrInvalidWebAppRoots, label)
	}
	clean := filepath.Clean(root)
	if clean != root {
		return "", fmt.Errorf("%w: %s must be clean", ErrInvalidWebAppRoots, label)
	}
	return clean, nil
}

func joinContained(root string, elements ...string) (string, error) {
	candidate := filepath.Clean(filepath.Join(append([]string{root}, elements...)...))
	if !filepath.IsAbs(candidate) || candidate != filepath.Join(append([]string{root}, elements...)...) || !pathContainedBy(root, candidate) {
		return "", fmt.Errorf("%w: generated path escapes install root", ErrInvalidWebAppRoots)
	}
	return filepath.ToSlash(candidate), nil
}

func pathContainedBy(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) || relative == ".." {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// WebApp is one validated name|url|icon_url declaration. Declaration order is
// meaningful and is retained in every generated plan.
type WebApp struct {
	Name    string
	URL     string
	IconURL string
}

// WebAppDefinition is a descriptive alias for callers that prefer catalog
// terminology.
type WebAppDefinition = WebApp

// WebAppEntry is a descriptive alias for plan-oriented callers.
type WebAppEntry = WebApp

// WebAppAssets contains the two embedded inputs consumed by the webapp domain.
// Every returned byte slice is owned by the caller and may be mutated safely.
type WebAppAssets struct {
	List     []byte
	Launcher []byte
}

// LoadWebAppAssets reads the list and launcher only from the embedded asset
// filesystem. It never consults repository-relative paths or the live host.
func LoadWebAppAssets() (WebAppAssets, error) {
	list, err := fs.ReadFile(assets.FS, webAppsListAsset)
	if err != nil {
		return WebAppAssets{}, fmt.Errorf("read embedded webapp list: %w", err)
	}
	launcher, err := fs.ReadFile(assets.FS, webAppLauncherAsset)
	if err != nil {
		return WebAppAssets{}, fmt.Errorf("read embedded webapp launcher: %w", err)
	}
	return WebAppAssets{List: cloneBytes(list), Launcher: cloneBytes(launcher)}, nil
}

// LoadEmbeddedWebApps loads and validates the embedded webapp declarations in
// declaration order.
func LoadEmbeddedWebApps() ([]WebApp, error) {
	loaded, err := LoadWebAppAssets()
	if err != nil {
		return nil, err
	}
	return ParseWebApps(loaded.List)
}

// ParseWebAppList is an explicit list-oriented spelling of ParseWebApps.
func ParseWebAppList(data []byte) ([]WebApp, error) { return ParseWebApps(data) }

// ParseWebApps validates an ordered name|url|icon_url list. Blank and comment
// lines are ignored; every other line must contain exactly three non-empty
// fields. All rows are validated before a caller can begin icon fetching.
func ParseWebApps(data []byte) ([]WebApp, error) {
	if !utf8.Valid(data) {
		return nil, webAppDefinitionError(0, "list", "invalid UTF-8")
	}

	rows := strings.Split(string(data), "\n")
	apps := make([]WebApp, 0, len(rows))
	seenBasenames := make(map[string]struct{}, len(rows))
	for lineNumber, raw := range rows {
		line := strings.TrimSuffix(raw, "\r")
		if hasControl(line) {
			return nil, webAppDefinitionError(lineNumber+1, "line", "control character")
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) != 3 {
			return nil, webAppDefinitionError(lineNumber+1, "line", "expected name|url|icon_url")
		}
		for _, field := range fields {
			if strings.TrimSpace(field) != field {
				return nil, webAppDefinitionError(lineNumber+1, "line", "surrounding whitespace")
			}
		}
		name := fields[0]
		webURL := fields[1]
		iconURL := fields[2]
		for _, field := range []struct {
			name  string
			value string
		}{
			{name: "name", value: name},
			{name: "url", value: webURL},
			{name: "icon_url", value: iconURL},
		} {
			if field.value == "" {
				return nil, webAppDefinitionError(lineNumber+1, field.name, "field is empty")
			}
			if hasControl(field.value) {
				return nil, webAppDefinitionError(lineNumber+1, field.name, "control character")
			}
		}

		basename, err := safeWebAppBasename(name)
		if err != nil {
			return nil, webAppDefinitionError(lineNumber+1, "name", "unsafe name")
		}
		if _, exists := seenBasenames[basename]; exists {
			return nil, webAppDefinitionError(lineNumber+1, "name", "duplicate name")
		}
		if err := validateWebAppURL(webURL, "url"); err != nil {
			return nil, webAppDefinitionError(lineNumber+1, "url", "must be an absolute HTTP(S) URL")
		}
		if err := validateWebAppURL(iconURL, "icon_url"); err != nil {
			return nil, webAppDefinitionError(lineNumber+1, "icon_url", "must be an absolute HTTP(S) URL")
		}

		seenBasenames[basename] = struct{}{}
		apps = append(apps, WebApp{Name: name, URL: webURL, IconURL: iconURL})
	}
	return cloneWebApps(apps), nil
}

// WebAppDefinitionError identifies the safe location and static reason for a
// rejected declaration. It deliberately never includes the raw field value.
type WebAppDefinitionError struct {
	Line   int
	Field  string
	Reason string
}

func (e *WebAppDefinitionError) Error() string {
	if e == nil {
		return ErrInvalidWebApp.Error()
	}
	if e.Line <= 0 {
		return fmt.Sprintf("%s: %s: %s", ErrInvalidWebApp, e.Field, e.Reason)
	}
	return fmt.Sprintf("%s at line %d: %s: %s", ErrInvalidWebApp, e.Line, e.Field, e.Reason)
}

func (e *WebAppDefinitionError) Unwrap() error { return ErrInvalidWebApp }

func webAppDefinitionError(line int, field, reason string) error {
	return &WebAppDefinitionError{Line: line, Field: field, Reason: reason}
}

// IconFetchRequest is the bounded request passed to the injected icon seam.
// The URL has already passed HTTP(S), host, control-character, and credential
// checks before it reaches a fetcher.
type IconFetchRequest struct {
	URL      string
	MaxBytes int64
}

// IconFetcher is the network boundary for this domain. Implementations own
// transport, timeout, and response-body reading; the domain independently
// enforces MaxBytes and PNG validation on returned bytes.
type IconFetcher interface {
	Fetch(context.Context, IconFetchRequest) ([]byte, error)
}

// IconFetcherFunc adapts a function to IconFetcher.
type IconFetcherFunc func(context.Context, IconFetchRequest) ([]byte, error)

// Fetch implements IconFetcher.
func (f IconFetcherFunc) Fetch(ctx context.Context, request IconFetchRequest) ([]byte, error) {
	if f == nil {
		return nil, ErrWebAppIconFetcher
	}
	return f(ctx, request)
}

// URLIconFetcherFunc adapts a function that wants the URL and byte bound as
// separate arguments to IconFetcher.
type URLIconFetcherFunc func(context.Context, string, int64) ([]byte, error)

// Fetch implements IconFetcher.
func (f URLIconFetcherFunc) Fetch(ctx context.Context, request IconFetchRequest) ([]byte, error) {
	if f == nil {
		return nil, ErrWebAppIconFetcher
	}
	return f(ctx, request.URL, request.MaxBytes)
}

// SimpleIconFetcherFunc adapts a context-aware URL function that performs its
// own bounded read.
type SimpleIconFetcherFunc func(context.Context, string) ([]byte, error)

// Fetch implements IconFetcher.
func (f SimpleIconFetcherFunc) Fetch(ctx context.Context, request IconFetchRequest) ([]byte, error) {
	if f == nil {
		return nil, ErrWebAppIconFetcher
	}
	return f(ctx, request.URL)
}

// IconOutcome records which ordered attempt supplied a valid PNG.
type IconOutcome string

const (
	IconOutcomeDeclared        IconOutcome = "declared-icon"
	IconOutcomeFaviconFallback IconOutcome = "favicon-fallback"
)

// IconFetchOutcome and WebAppIconOutcome are descriptive aliases for the typed
// outcome carried by an install and its managed icon descriptor.
type IconFetchOutcome = IconOutcome
type WebAppIconOutcome = IconOutcome

// DeclaredIcon and FaviconFallback are concise aliases for the typed outcomes.
const (
	DeclaredIcon           = IconOutcomeDeclared
	FaviconFallback        = IconOutcomeFaviconFallback
	OutcomeDeclaredIcon    = IconOutcomeDeclared
	OutcomeFaviconFallback = IconOutcomeFaviconFallback
)

// IconResolution is the single result of the icon state machine. Bytes are a
// defensive copy and URL is the selected source URL.
type IconResolution struct {
	Outcome IconOutcome
	URL     string
	Bytes   []byte
}

func (r IconResolution) Clone() IconResolution {
	r.Bytes = cloneBytes(r.Bytes)
	return r
}

// IconFailureReason is a closed, sanitized reason suitable for structured logs.
type IconFailureReason string

const IconFailureUnavailableOrInvalid IconFailureReason = "unavailable-or-invalid-png"

// IconFetchError is returned only after both the declared icon and derived
// same-origin favicon attempts fail. It contains only bounded numeric metadata
// and a closed reason; no declaration, name, URL, bytes, or transport error is
// retained.
type IconFetchError struct {
	AppIndex int
	Attempts int
	Reason   IconFailureReason
}

func (e *IconFetchError) Error() string {
	if e == nil {
		return ErrWebAppIconFetch.Error()
	}
	return fmt.Sprintf("%s for webapp declaration %d", ErrWebAppIconFetch, e.AppIndex+1)
}

func (e *IconFetchError) Unwrap() error { return ErrWebAppIconFetch }

// WebAppIconError is a descriptive alias for IconFetchError.
type WebAppIconError = IconFetchError

// FileOwnershipKind is supplied by the observe phase after it has inspected a
// target. It is intentionally absent from desired-file descriptors.
type FileOwnershipKind string

const (
	OwnershipCreated     FileOwnershipKind = "created"
	OwnershipAdopted     FileOwnershipKind = "adopted"
	FileOwnershipCreated FileOwnershipKind = OwnershipCreated
	FileOwnershipAdopted FileOwnershipKind = OwnershipAdopted
	FileCreated          FileOwnershipKind = OwnershipCreated
	FileAdopted          FileOwnershipKind = OwnershipAdopted
)

// FileOwnershipObservation is the typed, post-observation input to inverse
// binding. Target is intentionally not repeated: the descriptor path is the
// one canonical target.
type FileOwnershipObservation struct {
	Kind       FileOwnershipKind
	BackupPath string
}

// OwnershipObservation is a concise alias for the post-observation input.
type OwnershipObservation = FileOwnershipObservation

// ManagedFileInverse is produced only after a typed ownership observation.
type ManagedFileInverse struct {
	Source FileOwnershipKind
	planner.InverseDescriptor
}

// ManagedFileDescriptor describes desired bytes without performing publication.
// Content is copied into the descriptor; inverse data is deliberately not
// predeclared because it depends on the later ownership observation.
type ManagedFileDescriptor struct {
	Path      string
	Content   []byte
	Mode      uint32
	Kind      string
	Outcome   IconOutcome
	SourceURL string
}

// DesiredManagedFile is the terminology used by planner-facing callers.
type DesiredManagedFile = ManagedFileDescriptor

// WebAppFileDescriptor and ManagedFile are compatibility aliases for the same
// single desired-file value.
type WebAppFileDescriptor = ManagedFileDescriptor
type ManagedFile = ManagedFileDescriptor

// Clone returns a defensive copy of the descriptor and its content.
func (d ManagedFileDescriptor) Clone() ManagedFileDescriptor {
	d.Content = cloneBytes(d.Content)
	return d
}

// BindInverse converts an observed ownership state into the one valid typed
// inverse for this descriptor.
func (d ManagedFileDescriptor) BindInverse(observation FileOwnershipObservation) (ManagedFileInverse, error) {
	if err := validateManagedFilePath(d.Path); err != nil {
		return ManagedFileInverse{}, err
	}
	switch observation.Kind {
	case OwnershipCreated:
		if observation.BackupPath != "" {
			return ManagedFileInverse{}, fmt.Errorf("%w: created file cannot carry a backup", ErrInvalidFileOwnership)
		}
		value, err := json.Marshal(struct {
			Path string `json:"path"`
		}{Path: d.Path})
		if err != nil {
			return ManagedFileInverse{}, fmt.Errorf("%w: encode created inverse", ErrInvalidFileOwnership)
		}
		return ManagedFileInverse{
			Source: OwnershipCreated,
			InverseDescriptor: planner.InverseDescriptor{
				Operation: planner.Operation("remove-file"),
				Value:     value,
			},
		}, nil
	case OwnershipAdopted:
		if err := validateManagedFilePath(observation.BackupPath); err != nil {
			return ManagedFileInverse{}, fmt.Errorf("%w: invalid adoption backup", ErrInvalidFileOwnership)
		}
		expectedBackup := d.Path + ".bak.alex-cachyos"
		if observation.BackupPath != expectedBackup {
			return ManagedFileInverse{}, fmt.Errorf("%w: adoption backup does not match target", ErrInvalidFileOwnership)
		}
		value, err := json.Marshal(struct {
			Path   string `json:"path"`
			Backup string `json:"backup"`
		}{Path: d.Path, Backup: observation.BackupPath})
		if err != nil {
			return ManagedFileInverse{}, fmt.Errorf("%w: encode adopted inverse", ErrInvalidFileOwnership)
		}
		return ManagedFileInverse{
			Source: OwnershipAdopted,
			InverseDescriptor: planner.InverseDescriptor{
				Operation: planner.Operation("restore-backup"),
				Value:     value,
			},
		}, nil
	default:
		return ManagedFileInverse{}, fmt.Errorf("%w: unsupported ownership kind", ErrInvalidFileOwnership)
	}
}

// PlannerInverse returns the planner carrier after inverse binding. It cannot
// manufacture an inverse without the typed observation.
func (d ManagedFileDescriptor) PlannerInverse(observation FileOwnershipObservation) (planner.InverseDescriptor, error) {
	inverse, err := d.BindInverse(observation)
	if err != nil {
		return planner.InverseDescriptor{}, err
	}
	inverse.Value = cloneBytes(inverse.Value)
	return inverse.InverseDescriptor, nil
}

// BindManagedFileInverse is the functional spelling of ManagedFileDescriptor.BindInverse.
func BindManagedFileInverse(file ManagedFileDescriptor, observation FileOwnershipObservation) (ManagedFileInverse, error) {
	return file.BindInverse(observation)
}

func validateManagedFilePath(path string) error {
	if path == "" || !utf8.ValidString(path) || hasControl(path) || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%w: target must be a clean absolute path", ErrInvalidFileOwnership)
	}
	return nil
}

// WebAppInstall contains the two canonical generated files for one declaration.
// Paths and icon metadata live on those descriptors rather than in parallel
// path/outcome fields.
type WebAppInstall struct {
	WebApp   WebApp
	BaseName string
	Desktop  ManagedFileDescriptor
	Icon     ManagedFileDescriptor
}

func (i WebAppInstall) Clone() WebAppInstall {
	i.Desktop = i.Desktop.Clone()
	i.Icon = i.Icon.Clone()
	return i
}

// WebAppPlan is an immutable-by-convention desired plan. The launcher and each
// install own the only stored file descriptors; Files returns defensive views in
// stable launcher, desktop, icon order instead of retaining a duplicate slice.
type WebAppPlan struct {
	Launcher ManagedFileDescriptor
	Installs []WebAppInstall
}

// Files returns defensive copies of every generated file in deterministic order.
func (p WebAppPlan) Files() []ManagedFileDescriptor {
	files := make([]ManagedFileDescriptor, 0, 1+2*len(p.Installs))
	files = append(files, p.Launcher.Clone())
	for _, install := range p.Installs {
		files = append(files, install.Desktop.Clone(), install.Icon.Clone())
	}
	return files
}

// ManagedFiles is a descriptive method alias; it does not retain another mutable
// representation of the file list.
func (p WebAppPlan) ManagedFiles() []ManagedFileDescriptor { return p.Files() }

// Clone returns a deep copy of a generated plan.
func (p WebAppPlan) Clone() WebAppPlan {
	copyOf := WebAppPlan{Launcher: p.Launcher.Clone()}
	if p.Installs != nil {
		copyOf.Installs = make([]WebAppInstall, len(p.Installs))
	}
	for i, install := range p.Installs {
		copyOf.Installs[i] = install.Clone()
	}
	return copyOf
}

// WebAppInstallPlan is an alias used by installation-oriented callers.
type WebAppInstallPlan = WebAppPlan

// BuildWebAppPlan loads the embedded declarations and launcher. A validated
// absolute root pair is mandatory; no ambient HOME or tilde path is used.
func BuildWebAppPlan(fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	return BuildWebAppPlanWithContext(context.Background(), fetcher, roots...)
}

// BuildWebAppPlanWithRoots is the explicit root-first constructor.
func BuildWebAppPlanWithRoots(roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanWithContext(context.Background(), fetcher, roots)
}

// BuildWebAppPlanWithContext is BuildWebAppPlan with caller-controlled context
// propagation to the injected icon seam.
func BuildWebAppPlanWithContext(ctx context.Context, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	loaded, err := LoadWebAppAssets()
	if err != nil {
		return WebAppPlan{}, err
	}
	return buildWebAppPlan(ctx, loaded.List, loaded.Launcher, fetcher, roots...)
}

// BuildWebAppPlanWithContextAndRoots is the explicit context/root-first spelling.
func BuildWebAppPlanWithContextAndRoots(ctx context.Context, roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanWithContext(ctx, fetcher, roots)
}

// BuildWebAppPlanFromList builds from supplied list bytes while still loading
// the launcher from embedded assets. The list remains pure input data.
func BuildWebAppPlanFromList(list []byte, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	return BuildWebAppPlanFromListWithContext(context.Background(), list, fetcher, roots...)
}

// BuildWebAppPlanFromListWithRoots is the explicit root-first spelling.
func BuildWebAppPlanFromListWithRoots(list []byte, roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanFromListWithContext(context.Background(), list, fetcher, roots)
}

// BuildWebAppPlanFromListWithContext is the context-aware list constructor.
func BuildWebAppPlanFromListWithContext(ctx context.Context, list []byte, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	launcher, err := fs.ReadFile(assets.FS, webAppLauncherAsset)
	if err != nil {
		return WebAppPlan{}, fmt.Errorf("read embedded webapp launcher: %w", err)
	}
	return buildWebAppPlan(ctx, list, launcher, fetcher, roots...)
}

// BuildWebAppPlanFromListWithContextAndRoots is the explicit context/root-first spelling.
func BuildWebAppPlanFromListWithContextAndRoots(ctx context.Context, list []byte, roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanFromListWithContext(ctx, list, fetcher, roots)
}

// BuildWebAppPlanFromEntries builds from already parsed entries. The entries are
// validated again so callers cannot bypass safety or duplicate checks.
func BuildWebAppPlanFromEntries(entries []WebApp, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	return BuildWebAppPlanFromEntriesWithContext(context.Background(), entries, fetcher, roots...)
}

// BuildWebAppPlanFromEntriesWithRoots is the explicit root-first constructor.
func BuildWebAppPlanFromEntriesWithRoots(entries []WebApp, roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanFromEntriesWithContext(context.Background(), entries, fetcher, roots)
}

// BuildWebAppPlanFromEntriesWithContext is the context-aware entry-point for
// already parsed definitions.
func BuildWebAppPlanFromEntriesWithContext(ctx context.Context, entries []WebApp, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	launcher, err := fs.ReadFile(assets.FS, webAppLauncherAsset)
	if err != nil {
		return WebAppPlan{}, fmt.Errorf("read embedded webapp launcher: %w", err)
	}
	validated, err := validateWebApps(entries)
	if err != nil {
		return WebAppPlan{}, err
	}
	return buildWebAppPlanFromValidated(ctx, validated, launcher, fetcher, roots...)
}

// BuildWebAppPlanFromEntriesWithContextAndRoots is the explicit context/root-first spelling.
func BuildWebAppPlanFromEntriesWithContextAndRoots(ctx context.Context, entries []WebApp, roots WebAppRoots, fetcher IconFetcher) (WebAppPlan, error) {
	return BuildWebAppPlanFromEntriesWithContext(ctx, entries, fetcher, roots)
}

func buildWebAppPlan(ctx context.Context, list, launcher []byte, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	entries, err := ParseWebApps(list)
	if err != nil {
		return WebAppPlan{}, err
	}
	return buildWebAppPlanFromValidated(ctx, entries, launcher, fetcher, roots...)
}

func buildWebAppPlanFromValidated(ctx context.Context, entries []WebApp, launcher []byte, fetcher IconFetcher, roots ...WebAppRoots) (WebAppPlan, error) {
	ctx = normalizedContext(ctx)
	paths, err := resolveRootArgument(roots)
	if err != nil {
		return WebAppPlan{}, err
	}
	if fetcher == nil {
		return WebAppPlan{}, ErrWebAppIconFetcher
	}

	plan := WebAppPlan{Launcher: newManagedFile(paths.launcher, launcher, webAppExecutableMode, "launcher")}
	plan.Installs = make([]WebAppInstall, 0, len(entries))
	for index, entry := range entries {
		basename, err := safeWebAppBasename(entry.Name)
		if err != nil {
			// validateWebApps already checked this; retain a fail-closed guard for
			// future callers that add a construction path.
			return WebAppPlan{}, webAppDefinitionError(index+1, "name", "unsafe name")
		}
		icon, err := resolveWebAppIcon(ctx, index, entry, fetcher)
		if err != nil {
			return WebAppPlan{}, err
		}

		desktopPath, err := joinContained(paths.applications, basename+webAppDesktopExtension)
		if err != nil {
			return WebAppPlan{}, err
		}
		iconPath, err := joinContained(paths.icons, basename+webAppIconExtension)
		if err != nil {
			return WebAppPlan{}, err
		}
		desktop := newManagedFile(desktopPath, renderWebAppDesktopAtPaths(entry, basename, paths.launcher, iconPath), webAppExecutableMode, "desktop")
		iconFile := newManagedFile(iconPath, icon.Bytes, webAppIconMode, "icon")
		iconFile.Outcome = icon.Outcome
		iconFile.SourceURL = icon.URL
		plan.Installs = append(plan.Installs, WebAppInstall{
			WebApp:   entry,
			BaseName: basename,
			Desktop:  desktop,
			Icon:     iconFile,
		})
	}
	return plan, nil
}

func resolveRootArgument(roots []WebAppRoots) (resolvedWebAppPaths, error) {
	if len(roots) == 0 {
		return resolvedWebAppPaths{}, ErrWebAppRootsRequired
	}
	if len(roots) != 1 {
		return resolvedWebAppPaths{}, fmt.Errorf("%w: exactly one root pair is required", ErrInvalidWebAppRoots)
	}
	return validateWebAppRoots(roots[0])
}

func normalizedContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func resolveWebAppIcon(ctx context.Context, index int, entry WebApp, fetcher IconFetcher) (IconResolution, error) {
	ctx = normalizedContext(ctx)
	if err := ctx.Err(); err != nil {
		return IconResolution{}, err
	}
	primary := IconFetchRequest{URL: entry.IconURL, MaxBytes: MaxWebAppIconBytes}
	content, primaryErr := fetchIcon(ctx, fetcher, primary)
	if primaryErr == nil && validPNG(content) {
		return IconResolution{Outcome: IconOutcomeDeclared, URL: primary.URL, Bytes: cloneBytes(content)}, nil
	}
	if err := contextError(ctx, primaryErr); err != nil {
		return IconResolution{}, err
	}

	fallback, err := DeriveWebAppFaviconURL(entry.URL)
	if err != nil {
		return IconResolution{}, newIconFetchError(index)
	}
	if err := ctx.Err(); err != nil {
		return IconResolution{}, err
	}
	fallbackRequest := IconFetchRequest{URL: fallback, MaxBytes: MaxWebAppIconBytes}
	content, fallbackErr := fetchIcon(ctx, fetcher, fallbackRequest)
	if fallbackErr == nil && validPNG(content) {
		return IconResolution{Outcome: IconOutcomeFaviconFallback, URL: fallbackRequest.URL, Bytes: cloneBytes(content)}, nil
	}
	if err := contextError(ctx, fallbackErr); err != nil {
		return IconResolution{}, err
	}
	return IconResolution{}, newIconFetchError(index)
}

func newIconFetchError(index int) *IconFetchError {
	return &IconFetchError{AppIndex: index, Attempts: 2, Reason: IconFailureUnavailableOrInvalid}
}

func contextError(ctx context.Context, fetchErr error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(fetchErr, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(fetchErr, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}

func fetchIcon(ctx context.Context, fetcher IconFetcher, request IconFetchRequest) ([]byte, error) {
	ctx = normalizedContext(ctx)
	if fetcher == nil {
		return nil, ErrWebAppIconFetcher
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, err := fetcher.Fetch(ctx, request)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(content)) > request.MaxBytes {
		return nil, ErrInvalidWebAppPNG
	}
	return cloneBytes(content), nil
}

// DeriveWebAppFaviconURL returns the deterministic /favicon.ico URL at the
// validated webapp origin. It discards path, query, and fragment components.
func DeriveWebAppFaviconURL(webURL string) (string, error) {
	if err := validateWebAppURL(webURL, "url"); err != nil {
		return "", err
	}
	parsed, err := url.Parse(webURL)
	if err != nil {
		return "", errors.New("invalid webapp URL")
	}
	return (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: parsed.Host, Path: "/favicon.ico"}).String(), nil
}

// WebAppFaviconURL is a concise alias for DeriveWebAppFaviconURL.
func WebAppFaviconURL(webURL string) (string, error) { return DeriveWebAppFaviconURL(webURL) }

// ResolveWebAppIcon exposes the ordered two-attempt icon state machine for one
// validated declaration. A standalone call reports the declaration as index 0.
func ResolveWebAppIcon(ctx context.Context, entry WebApp, fetcher IconFetcher) (IconResolution, error) {
	validated, err := validateWebApps([]WebApp{entry})
	if err != nil {
		return IconResolution{}, err
	}
	return resolveWebAppIcon(normalizedContext(ctx), 0, validated[0], fetcher)
}

// SafeWebAppBasename returns the deterministic filename stem used for both the
// desktop entry and the PNG icon. It accepts only names that cannot introduce a
// path component or desktop-file control character.
func SafeWebAppBasename(name string) (string, error) { return safeWebAppBasename(name) }

func safeWebAppBasename(name string) (string, error) {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, `/\\`) {
		return "", errors.New("unsafe webapp name")
	}
	if hasControl(name) || !utf8.ValidString(name) {
		return "", errors.New("unsafe webapp name")
	}

	var builder strings.Builder
	separator := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if separator && builder.Len() != 0 {
				builder.WriteByte('-')
			}
			builder.WriteRune(unicode.ToLower(r))
			separator = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			separator = builder.Len() != 0
		default:
			return "", errors.New("unsafe webapp name")
		}
	}
	basename := builder.String()
	if basename == "" || basename == "." || basename == ".." || !utf8.ValidString(basename) {
		return "", errors.New("unsafe webapp name")
	}
	if len([]byte(basename+webAppDesktopExtension)) > filesystemComponentLimitBytes ||
		len([]byte(basename+webAppIconExtension)) > filesystemComponentLimitBytes {
		return "", errors.New("webapp basename exceeds filesystem component limit")
	}
	return basename, nil
}

// RenderWebAppDesktop validates one declaration and renders its canonical
// desktop-entry bytes without writing them. One validated root pair is required.
func RenderWebAppDesktop(entry WebApp, roots ...WebAppRoots) ([]byte, error) {
	validated, err := validateWebApps([]WebApp{entry})
	if err != nil {
		return nil, err
	}
	paths, err := resolveRootArgument(roots)
	if err != nil {
		return nil, err
	}
	basename, err := safeWebAppBasename(validated[0].Name)
	if err != nil {
		return nil, webAppDefinitionError(1, "name", "unsafe name")
	}
	iconPath, err := joinContained(paths.icons, basename+webAppIconExtension)
	if err != nil {
		return nil, err
	}
	return renderWebAppDesktopAtPaths(validated[0], basename, paths.launcher, iconPath), nil
}

// RenderWebAppDesktopWithRoots is the explicit root-first spelling.
func RenderWebAppDesktopWithRoots(roots WebAppRoots, entry WebApp) ([]byte, error) {
	return RenderWebAppDesktop(entry, roots)
}

func renderWebAppDesktopAtPaths(entry WebApp, basename, launcherPath, iconPath string) []byte {
	var builder strings.Builder
	builder.WriteString("[Desktop Entry]\n")
	builder.WriteString("Version=1.0\n")
	builder.WriteString("Type=Application\n")
	builder.WriteString("Name=")
	builder.WriteString(entry.Name)
	builder.WriteByte('\n')
	builder.WriteString("Comment=")
	builder.WriteString(entry.Name)
	builder.WriteString(" (Chrome webapp — alex-cachyos)\n")
	builder.WriteString("Exec=")
	builder.WriteString(encodeDesktopExecPath(launcherPath))
	builder.WriteByte(' ')
	builder.WriteString(encodeDesktopExecArgument(entry.URL))
	builder.WriteByte('\n')
	builder.WriteString("Icon=")
	builder.WriteString(iconPath)
	builder.WriteByte('\n')
	builder.WriteString("Terminal=false\n")
	builder.WriteString("StartupNotify=true\n")
	builder.WriteString("Categories=Network;\n")
	return []byte(builder.String())
}

// EncodeWebAppExecURL applies Desktop Entry argument quoting. It does not
// invoke a shell and escapes field-code percent signs so the launcher receives
// one unchanged URL argument.
func EncodeWebAppExecURL(webURL string) string { return encodeDesktopExecArgument(webURL) }

func encodeDesktopExecPath(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\r\n\\\"'`$%") {
		return value
	}
	return encodeDesktopExecArgument(value)
}

func encodeDesktopExecArgument(value string) string {
	var builder strings.Builder
	builder.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\', '"', '`', '$':
			builder.WriteByte('\\')
		case '%':
			// %% is the Desktop Entry spelling for a literal percent.
			builder.WriteString("%%")
			continue
		}
		builder.WriteRune(r)
	}
	builder.WriteByte('"')
	return builder.String()
}

func validateWebApps(entries []WebApp) ([]WebApp, error) {
	validated := make([]WebApp, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		webURL := strings.TrimSpace(entry.URL)
		iconURL := strings.TrimSpace(entry.IconURL)
		if name != entry.Name || webURL != entry.URL || iconURL != entry.IconURL {
			return nil, webAppDefinitionError(index+1, "entry", "surrounding whitespace")
		}
		if name == "" {
			return nil, webAppDefinitionError(index+1, "name", "field is empty")
		}
		if hasControl(entry.Name) || hasControl(entry.URL) || hasControl(entry.IconURL) {
			return nil, webAppDefinitionError(index+1, "entry", "control character")
		}
		basename, err := safeWebAppBasename(name)
		if err != nil {
			return nil, webAppDefinitionError(index+1, "name", "unsafe name")
		}
		if _, exists := seen[basename]; exists {
			return nil, webAppDefinitionError(index+1, "name", "duplicate name")
		}
		if err := validateWebAppURL(webURL, "url"); err != nil {
			return nil, webAppDefinitionError(index+1, "url", "must be an absolute HTTP(S) URL")
		}
		if err := validateWebAppURL(iconURL, "icon_url"); err != nil {
			return nil, webAppDefinitionError(index+1, "icon_url", "must be an absolute HTTP(S) URL")
		}
		seen[basename] = struct{}{}
		validated = append(validated, WebApp{Name: name, URL: webURL, IconURL: iconURL})
	}
	return cloneWebApps(validated), nil
}

func validateWebAppURL(value, _ string) error {
	if value == "" || !utf8.ValidString(value) || hasControl(value) || strings.TrimSpace(value) != value {
		return errors.New("invalid webapp URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return errors.New("invalid webapp URL")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.Opaque != "" || parsed.User != nil {
		return errors.New("webapp URL must be absolute HTTP(S) without credentials")
	}
	return nil
}

func validPNG(content []byte) bool { return validatePNG(content) == nil }

func validatePNG(content []byte) error {
	const pngSignatureLength = 8
	if int64(len(content)) > MaxWebAppIconBytes || len(content) < pngSignatureLength || !bytes.Equal(content[:pngSignatureLength], []byte("\x89PNG\r\n\x1a\n")) {
		return ErrInvalidWebAppPNG
	}
	config, err := png.DecodeConfig(bytes.NewReader(content))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > MaxWebAppIconDimension || config.Height > MaxWebAppIconDimension {
		return ErrInvalidWebAppPNG
	}
	pixels := int64(config.Width) * int64(config.Height)
	if pixels <= 0 || pixels > MaxWebAppIconPixels {
		return ErrInvalidWebAppPNG
	}
	decoded, err := png.Decode(bytes.NewReader(content))
	if err != nil {
		return ErrInvalidWebAppPNG
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return ErrInvalidWebAppPNG
	}
	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func newManagedFile(path string, content []byte, mode uint32, kind string) ManagedFileDescriptor {
	return ManagedFileDescriptor{
		Path:    filepath.ToSlash(filepath.Clean(path)),
		Content: cloneBytes(content),
		Mode:    mode,
		Kind:    kind,
	}
}

func cloneWebApps(values []WebApp) []WebApp {
	if values == nil {
		return nil
	}
	return append([]WebApp(nil), values...)
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
