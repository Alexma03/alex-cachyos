package cachyos

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"alex-cachyos/internal/planner"
)

type scriptedIconFetcher struct {
	responses map[string][]byte
	errors    map[string]error
	requests  []IconFetchRequest
	contexts  []context.Context
	onFetch   func(context.Context, IconFetchRequest)
}

func (f *scriptedIconFetcher) Fetch(ctx context.Context, request IconFetchRequest) ([]byte, error) {
	f.contexts = append(f.contexts, ctx)
	f.requests = append(f.requests, request)
	if f.onFetch != nil {
		f.onFetch(ctx, request)
	}
	if err := f.errors[request.URL]; err != nil {
		return nil, err
	}
	return append([]byte(nil), f.responses[request.URL]...), nil
}

func testWebAppRoots(t *testing.T) WebAppRoots {
	t.Helper()
	home := t.TempDir()
	roots, err := NewWebAppRoots(home, filepath.Join(home, ".local"))
	if err != nil {
		t.Fatalf("NewWebAppRoots() error = %v", err)
	}
	return roots
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.SetRGBA(0, 0, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, picture); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return encoded.Bytes()
}

func TestWebAppPlanUsesInjectedAbsoluteRootsAndCanonicalFiles(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "My App", URL: "https://example.test/app", IconURL: "https://icons.test/my.png"}
	declared := testPNG(t, 2, 2)
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{entry.IconURL: declared}}

	plan, err := BuildWebAppPlanFromEntriesWithRoots([]WebApp{entry}, roots, fetcher)
	if err != nil {
		t.Fatalf("BuildWebAppPlanFromEntriesWithRoots() error = %v", err)
	}
	if len(fetcher.requests) != 1 || fetcher.requests[0].URL != entry.IconURL {
		t.Fatalf("icon requests = %#v, want one declared-icon request", fetcher.requests)
	}
	if fetcher.requests[0].MaxBytes != MaxWebAppIconBytes {
		t.Fatalf("icon request max bytes = %d, want %d", fetcher.requests[0].MaxBytes, MaxWebAppIconBytes)
	}
	if len(plan.Installs) != 1 {
		t.Fatalf("install count = %d, want 1", len(plan.Installs))
	}
	install := plan.Installs[0]
	if install.BaseName != "my-app" {
		t.Fatalf("safe basename = %q, want my-app", install.BaseName)
	}

	wantLauncher := filepath.ToSlash(filepath.Join(roots.InstallRoot, "bin", webAppLauncherName))
	wantDesktop := filepath.ToSlash(filepath.Join(roots.InstallRoot, "share", "applications", "my-app.desktop"))
	wantIcon := filepath.ToSlash(filepath.Join(roots.InstallRoot, "share", "applications", "icons", "my-app.png"))
	if plan.Launcher.Path != wantLauncher || install.Desktop.Path != wantDesktop || install.Icon.Path != wantIcon {
		t.Fatalf("paths = launcher %q, desktop %q, icon %q; want %q, %q, %q", plan.Launcher.Path, install.Desktop.Path, install.Icon.Path, wantLauncher, wantDesktop, wantIcon)
	}
	for _, path := range []string{plan.Launcher.Path, install.Desktop.Path, install.Icon.Path} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.Contains(path, "~") {
			t.Fatalf("generated path is not a clean absolute path without tilde: %q", path)
		}
	}
	if install.Icon.Outcome != IconOutcomeDeclared || install.Icon.SourceURL != entry.IconURL {
		t.Fatalf("icon metadata = %#v, want declared source", install.Icon)
	}
	if !bytes.Equal(install.Icon.Content, declared) {
		t.Fatalf("icon content differs from fetched PNG")
	}
	if !strings.Contains(string(plan.Launcher.Content), "--app=\"$url\"") {
		t.Fatalf("launcher content was not loaded from the embedded launcher: %q", plan.Launcher.Content)
	}

	files := plan.Files()
	if len(files) != 3 || files[0].Path != wantLauncher || files[1].Path != wantDesktop || files[2].Path != wantIcon {
		t.Fatalf("files = %#v, want launcher plus desktop and icon in order", files)
	}
	desktop := string(install.Desktop.Content)
	if !strings.Contains(desktop, "Exec="+wantLauncher+" \"https://example.test/app\"") {
		t.Fatalf("desktop Exec does not use the same absolute launcher path: %q", desktop)
	}
	if !strings.Contains(desktop, "Icon="+wantIcon+"\n") {
		t.Fatalf("desktop Icon does not use the same absolute icon path: %q", desktop)
	}
	if strings.Contains(desktop, "~") {
		t.Fatalf("desktop entry contains a tilde path: %q", desktop)
	}
}

func TestWebAppRootsRejectUncleanRelativeAndEscapingInputs(t *testing.T) {
	cases := []WebAppRoots{
		{HomeRoot: "relative/home", InstallRoot: "/tmp/install"},
		{HomeRoot: "/tmp/home/../home", InstallRoot: "/tmp/home/.local"},
		{HomeRoot: "/tmp/home", InstallRoot: "/tmp/other"},
		{HomeRoot: "/tmp/home", InstallRoot: "/tmp/home/.local/../other"},
	}
	for _, roots := range cases {
		if err := roots.Validate(); !errors.Is(err, ErrInvalidWebAppRoots) {
			t.Errorf("roots %#v error = %v, want ErrInvalidWebAppRoots", roots, err)
		}
	}
}

func TestWebAppPlanRetriesDeterministicSameOriginFavicon(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Telegram", URL: "https://web.telegram.test/k/", IconURL: "https://cdn.test/telegram.png"}
	fallback := "https://web.telegram.test/favicon.ico"
	favicon := testPNG(t, 1, 1)
	fetcher := &scriptedIconFetcher{
		responses: map[string][]byte{fallback: favicon},
		errors:    map[string]error{entry.IconURL: errors.New("unreachable")},
	}

	plan, err := BuildWebAppPlanFromEntriesWithContextAndRoots(context.Background(), []WebApp{entry}, roots, fetcher)
	if err != nil {
		t.Fatalf("fallback plan error = %v", err)
	}
	if got := []string{fetcher.requests[0].URL, fetcher.requests[1].URL}; !reflect.DeepEqual(got, []string{entry.IconURL, fallback}) {
		t.Fatalf("icon request order = %#v, want declared then favicon", got)
	}
	if got := plan.Installs[0].Icon.Outcome; got != IconOutcomeFaviconFallback {
		t.Fatalf("fallback outcome = %q, want favicon-fallback", got)
	}
	if got := plan.Installs[0].Icon.SourceURL; got != fallback {
		t.Fatalf("selected icon URL = %q, want %q", got, fallback)
	}
	if !bytes.Equal(plan.Installs[0].Icon.Content, favicon) {
		t.Fatalf("fallback icon content differs from fetched PNG")
	}
}

func TestWebAppPlanBothIconAttemptsFailWithoutLeakingInputs(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Private App", URL: "https://private.test/", IconURL: "https://icons.test/icon.png?token=super-secret-value"}
	fetcher := &scriptedIconFetcher{errors: map[string]error{
		entry.IconURL:                      errors.New("response token=super-secret-value"),
		"https://private.test/favicon.ico": errors.New("second failure"),
	}}

	_, err := BuildWebAppPlanFromEntries([]WebApp{entry}, fetcher, roots)
	if err == nil {
		t.Fatal("both failed icon attempts unexpectedly succeeded")
	}
	var iconErr *IconFetchError
	if !errors.As(err, &iconErr) || !errors.Is(err, ErrWebAppIconFetch) {
		t.Fatalf("error = %T %v, want typed icon-fetch error", err, err)
	}
	message := err.Error()
	for _, forbidden := range []string{"super-secret-value", entry.IconURL, "response token", "favicon.ico", entry.Name} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("error leaked %q: %q", forbidden, message)
		}
	}
	if iconErr.AppIndex != 0 || iconErr.Attempts != 2 || iconErr.Reason != IconFailureUnavailableOrInvalid {
		t.Fatalf("typed error metadata = %#v, want sanitized metadata", iconErr)
	}
	if len(fetcher.requests) != 2 {
		t.Fatalf("requests = %#v, want primary and fallback only", fetcher.requests)
	}
	for _, fieldName := range []string{"Declaration", "Name", "URL", "Bytes"} {
		if _, ok := reflect.TypeOf(IconFetchError{}).FieldByName(fieldName); ok {
			t.Fatalf("IconFetchError unexpectedly exposes %s", fieldName)
		}
	}
}

func TestWebAppDefinitionsRejectUnsafeInputBeforeAnyFetch(t *testing.T) {
	roots := testWebAppRoots(t)
	cases := []struct {
		name string
		list string
	}{
		{name: "missing field", list: "App|https://example.test/icon-only\n"},
		{name: "extra field", list: "App|https://example.test/|https://icons.test/a.png|extra\n"},
		{name: "case-fold collision", list: "A|https://a.test/|https://icons.test/a.png\na|https://b.test/|https://icons.test/b.png\n"},
		{name: "unsafe name", list: "../../escape|https://example.test/|https://icons.test/a.png\n"},
		{name: "non-http web URL", list: "App|file:///tmp/app|https://icons.test/a.png\n"},
		{name: "non-http icon URL", list: "App|https://example.test/|ftp://icons.test/a.png\n"},
		{name: "control", list: "App|https://example.test/|https://icons.test/a.png\x00\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &scriptedIconFetcher{}
			if _, err := BuildWebAppPlanFromList([]byte(tc.list), fetcher, roots); err == nil {
				t.Fatal("unsafe webapp list unexpectedly accepted")
			}
			if len(fetcher.requests) != 0 {
				t.Fatalf("fetcher received requests before validation: %#v", fetcher.requests)
			}
		})
	}
}

func TestWebAppPlanRejectsMalformedTruncatedOversizedAndOversizedDimensionPNGs(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Bounded", URL: "https://bounded.test/", IconURL: "https://icons.test/bounded.png"}
	fallback := "https://bounded.test/favicon.ico"
	valid := testPNG(t, 2, 2)
	truncated := valid[:len(valid)-1]
	oversized := append(append([]byte(nil), valid...), make([]byte, MaxWebAppIconBytes-int64(len(valid))+1)...)
	dimensionLimited := testPNG(t, MaxWebAppIconDimension+1, 1)
	pixelLimited := testPNG(t, MaxWebAppIconDimension, int(MaxWebAppIconPixels/int64(MaxWebAppIconDimension))+1)

	cases := []struct {
		name string
		bad  []byte
	}{
		{name: "malformed", bad: []byte("not a png")},
		{name: "truncated", bad: truncated},
		{name: "oversized bytes", bad: oversized},
		{name: "oversized dimension", bad: dimensionLimited},
		{name: "oversized pixels", bad: pixelLimited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &scriptedIconFetcher{responses: map[string][]byte{
				entry.IconURL: tc.bad,
				fallback:      tc.bad,
			}}
			if _, err := BuildWebAppPlanFromEntriesWithRoots([]WebApp{entry}, roots, fetcher); !errors.Is(err, ErrWebAppIconFetch) {
				t.Fatalf("error = %v, want ErrWebAppIconFetch", err)
			}
			if len(fetcher.requests) != 2 {
				t.Fatalf("requests = %#v, want both bounded attempts", fetcher.requests)
			}
		})
	}
	if !validPNG(valid) {
		t.Fatal("real encoded PNG was rejected")
	}
}

func TestWebAppPlanContextCancellationStopsBeforeFaviconFallback(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Cancelled", URL: "https://cancelled.test/", IconURL: "https://icons.test/cancelled.png"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher := &scriptedIconFetcher{
		onFetch: func(_ context.Context, request IconFetchRequest) {
			if request.URL == entry.IconURL {
				cancel()
			}
		},
	}
	_, err := BuildWebAppPlanFromEntriesWithContextAndRoots(ctx, []WebApp{entry}, roots, fetcher)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if len(fetcher.requests) != 1 {
		t.Fatalf("requests = %#v, want primary only after cancellation", fetcher.requests)
	}
}

func TestWebAppPlanDeadlineStopsBeforeAnyFetch(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Deadline", URL: "https://deadline.test/", IconURL: "https://icons.test/deadline.png"}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	fetcher := &scriptedIconFetcher{}
	_, err := BuildWebAppPlanFromEntriesWithContextAndRoots(ctx, []WebApp{entry}, roots, fetcher)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if len(fetcher.requests) != 0 {
		t.Fatalf("requests = %#v, want none for expired context", fetcher.requests)
	}
}

func TestNilContextIsNormalizedToBackground(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Background", URL: "https://background.test/", IconURL: "https://icons.test/background.png"}
	var seen context.Context
	fetcher := IconFetcherFunc(func(ctx context.Context, request IconFetchRequest) ([]byte, error) {
		seen = ctx
		return testPNG(t, 1, 1), nil
	})
	if _, err := BuildWebAppPlanFromEntriesWithContextAndRoots(nil, []WebApp{entry}, roots, fetcher); err != nil {
		t.Fatalf("nil-context plan error = %v", err)
	}
	if seen == nil {
		t.Fatal("fetcher received nil context")
	}
}

func TestWebAppPlanPreservesOrderAndCloneIsolation(t *testing.T) {
	roots := testWebAppRoots(t)
	entries := []WebApp{
		{Name: "First", URL: "https://first.test/", IconURL: "https://icons.test/first.png"},
		{Name: "Second", URL: "https://second.test/", IconURL: "https://icons.test/second.png"},
	}
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{
		entries[0].IconURL: testPNG(t, 1, 1),
		entries[1].IconURL: testPNG(t, 2, 1),
	}}
	plan, err := BuildWebAppPlanFromEntriesWithRoots(entries, roots, fetcher)
	if err != nil {
		t.Fatalf("BuildWebAppPlanFromEntriesWithRoots() error = %v", err)
	}
	clone := plan.Clone()
	if !reflect.DeepEqual(plan, clone) {
		t.Fatalf("clone differs before mutation:\nplan=%#v\nclone=%#v", plan, clone)
	}
	clone.Launcher.Content[0] ^= 0xff
	clone.Installs[0].Desktop.Content[0] ^= 0xff
	clone.Installs[0].Icon.Content[0] ^= 0xff
	if bytes.Equal(plan.Launcher.Content, clone.Launcher.Content) ||
		bytes.Equal(plan.Installs[0].Desktop.Content, clone.Installs[0].Desktop.Content) ||
		bytes.Equal(plan.Installs[0].Icon.Content, clone.Installs[0].Icon.Content) {
		t.Fatal("plan Clone retained mutable file aliases")
	}
	files := plan.Files()
	files[0].Content[0] ^= 0xff
	files[1].Content[0] ^= 0xff
	files[2].Content[0] ^= 0xff
	if bytes.Equal(plan.Launcher.Content, files[0].Content) || bytes.Equal(plan.Installs[0].Desktop.Content, files[1].Content) || bytes.Equal(plan.Installs[0].Icon.Content, files[2].Content) {
		t.Fatal("Files() exposed mutable plan storage")
	}
	if got := []string{plan.Installs[0].WebApp.Name, plan.Installs[1].WebApp.Name}; !reflect.DeepEqual(got, []string{"First", "Second"}) {
		t.Fatalf("declaration order = %#v", got)
	}
}

func TestWebAppBasenameUsesUTF8ByteLimitIncludingExtensions(t *testing.T) {
	accepted := strings.Repeat("é", 123) // 246 UTF-8 bytes; .desktop remains 254 bytes.
	if got, err := SafeWebAppBasename(accepted); err != nil || len([]byte(got+webAppDesktopExtension)) > MaxWebAppFilenameComponentBytes {
		t.Fatalf("accepted basename = %q, error = %v", got, err)
	}
	rejected := strings.Repeat("é", 124) // 248 bytes; .desktop would be 256 bytes.
	if _, err := SafeWebAppBasename(rejected); err == nil {
		t.Fatal("basename exceeding byte limit unexpectedly accepted")
	}
}

func TestLongWebAppLineHasNoArtificialLineLimit(t *testing.T) {
	longURL := "https://long.test/?q=" + strings.Repeat("a", 4096)
	apps, err := ParseWebApps([]byte("Long|" + longURL + "|https://icons.test/long.png\n"))
	if err != nil {
		t.Fatalf("long valid declaration rejected: %v", err)
	}
	if len(apps) != 1 || apps[0].URL != longURL {
		t.Fatalf("long declaration = %#v", apps)
	}
}

func TestWebAppInverseBindsOnlyFromTypedOwnershipObservation(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Inverse", URL: "https://inverse.test/", IconURL: "https://icons.test/inverse.png"}
	plan, err := BuildWebAppPlanFromEntriesWithRoots([]WebApp{entry}, roots, &scriptedIconFetcher{responses: map[string][]byte{entry.IconURL: testPNG(t, 1, 1)}})
	if err != nil {
		t.Fatalf("plan error = %v", err)
	}
	file := plan.Installs[0].Icon
	created, err := file.BindInverse(FileOwnershipObservation{Kind: OwnershipCreated})
	if err != nil {
		t.Fatalf("created inverse error = %v", err)
	}
	if created.Source != OwnershipCreated || created.Operation != plannerRemoveFile {
		t.Fatalf("created inverse = %#v", created)
	}
	if got := string(created.Value); got != `{"path":"`+file.Path+`"}` {
		t.Fatalf("created inverse value = %s", got)
	}

	backup := file.Path + ".bak.alex-cachyos"
	adopted, err := file.BindInverse(FileOwnershipObservation{Kind: OwnershipAdopted, BackupPath: backup})
	if err != nil {
		t.Fatalf("adopted inverse error = %v", err)
	}
	if adopted.Source != OwnershipAdopted || adopted.Operation != plannerRestoreBackup || !strings.Contains(string(adopted.Value), `"backup":"`+backup+`"`) {
		t.Fatalf("adopted inverse = %#v", adopted)
	}
	if _, err := file.BindInverse(FileOwnershipObservation{Kind: OwnershipAdopted, BackupPath: file.Path + ".other"}); !errors.Is(err, ErrInvalidFileOwnership) {
		t.Fatalf("wrong adoption backup error = %v, want ErrInvalidFileOwnership", err)
	}
	if _, err := file.BindInverse(FileOwnershipObservation{Kind: FileOwnershipKind("unknown")}); !errors.Is(err, ErrInvalidFileOwnership) {
		t.Fatalf("unknown ownership error = %v, want ErrInvalidFileOwnership", err)
	}
}

func TestWebAppExecURLEscapesDesktopFieldCodesAndShellSyntax(t *testing.T) {
	roots := testWebAppRoots(t)
	entry := WebApp{Name: "Encoded", URL: "https://encoded.test/path?q=one%20two&x=$HOME`echo nope`", IconURL: "https://icons.test/encoded.png"}
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{entry.IconURL: testPNG(t, 1, 1)}}
	plan, err := BuildWebAppPlanFromEntriesWithRoots([]WebApp{entry}, roots, fetcher)
	if err != nil {
		t.Fatalf("encoded URL plan error = %v", err)
	}
	desktop := string(plan.Installs[0].Desktop.Content)
	for _, forbidden := range []string{"$HOME`", "$(", "\nhttps://encoded.test/path?q=one%20two&x=$HOME"} {
		if strings.Contains(desktop, forbidden) {
			t.Fatalf("desktop entry contains unsafe sequence %q: %q", forbidden, desktop)
		}
	}
	if !strings.Contains(desktop, `Exec=`+plan.Launcher.Path+` "https://encoded.test/path?q=one%%20two&x=`) || !strings.Contains(desktop, `\$HOME`) {
		t.Fatalf("desktop Exec was not canonically encoded: %q", desktop)
	}
}

func TestParseWebAppsReturnsCopiesAndRootInjectionIsMandatory(t *testing.T) {
	list := []byte("Copied|https://copied.test/|https://icons.test/copied.png\n")
	apps, err := ParseWebApps(list)
	if err != nil {
		t.Fatalf("ParseWebApps() error = %v", err)
	}
	list[0] = 'X'
	if apps[0].Name != "Copied" {
		t.Fatalf("parsed definition aliased input: %#v", apps)
	}
	fetcher := &scriptedIconFetcher{responses: map[string][]byte{apps[0].IconURL: testPNG(t, 1, 1)}}
	if _, err := BuildWebAppPlanFromEntries(apps, fetcher); !errors.Is(err, ErrWebAppRootsRequired) {
		t.Fatalf("missing roots error = %v, want ErrWebAppRootsRequired", err)
	}
}

const (
	plannerRemoveFile    planner.Operation = "remove-file"
	plannerRestoreBackup planner.Operation = "restore-backup"
)
