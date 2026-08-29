package catalog

import (
	"fmt"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
)

type PinValidationError struct {
	Pin, Path, Message string
}

func (e PinValidationError) Error() string {
	return fmt.Sprintf("pin %q%s: %s", e.Pin, e.Path, e.Message)
}

type NpmPin struct{ Name, Version string }

func ValidateNpmPin(name string, pin NpmPin) error {
	if !validNpmName(pin.Name) {
		return pinError(name, "/name", "npm package name is invalid")
	}
	if !exactSemver(pin.Version) {
		return pinError(name, "/version", "npm version must be an exact semver")
	}
	return nil
}
func ValidateNpmSpec(name, spec string) error {
	body := strings.TrimPrefix(spec, "npm:")
	at := strings.LastIndexByte(body, '@')
	if !strings.HasPrefix(spec, "npm:") || at <= 0 || at == len(body)-1 || !validNpmName(body[:at]) || !exactSemver(body[at+1:]) {
		return pinError(name, "/spec", "npm pin must use npm:<name>@<exact-semver>")
	}
	return nil
}

type CheckoutDestination struct {
	Path           string
	ForbiddenRoots []string
}

func ValidateSourceCheckoutPin(name string, pin CheckoutPin, destination CheckoutDestination) error {
	if !canonicalRemote(pin.Remote) {
		return pinError(name, "/remote", "checkout remote must be canonical")
	}
	if pin.Branch != "main" {
		return pinError(name, "/branch", "checkout branch must be main")
	}
	if !isHex(pin.Commit, 40) {
		return pinError(name, "/commit", "checkout commit must be 40 hexadecimal characters")
	}
	return ValidateCheckoutDestination(name, destination)
}
func ValidateCheckoutDestination(name string, destination CheckoutDestination) error {
	if !safeAbsolutePath(destination.Path) {
		return pinError(name, "/destination", "checkout destination must be a clean absolute path")
	}
	for i, root := range destination.ForbiddenRoots {
		if root == "" || !filepath.IsAbs(root) {
			return pinError(name, fmt.Sprintf("/forbiddenRoots/%d", i), "forbidden root must be absolute")
		}
		if pathWithin(filepath.Clean(root), destination.Path) {
			return pinError(name, "/destination", "checkout destination is under a forbidden root")
		}
	}
	return nil
}

type PacmanArtifactSource string

const (
	PacmanArtifactCache      PacmanArtifactSource = "cache"
	PacmanArtifactArchive    PacmanArtifactSource = "archive"
	PacmanArtifactRepository PacmanArtifactSource = "repository"
	PacmanCacheSource                             = PacmanArtifactCache
	PacmanArchiveSource                           = PacmanArtifactArchive
)

type PacmanArtifactPin struct {
	Package string
	Version string
	Source  PacmanArtifactSource
	SHA256  string
}

func ValidatePacmanArtifactPin(name string, pin PacmanArtifactPin) error {
	if !pacmanAtom(pin.Package, false) {
		return pinError(name, "/package", "pacman package name is invalid")
	}
	if !pacmanAtom(pin.Version, true) {
		return pinError(name, "/version", "pacman artifact version must be exact")
	}
	if pin.Source != PacmanArtifactCache && pin.Source != PacmanArtifactArchive {
		return pinError(name, "/source", "pacman artifact requires an approved cache or archive source")
	}
	if !isHex(pin.SHA256, 64) {
		return pinError(name, "/sha256", "SHA-256 must be 64 hexadecimal characters")
	}
	return nil
}

type AURLocalPin struct {
	SourceCommit string
	PatchSHA256  string
}

func ValidateAURLocalPin(name string, pin AURLocalPin) error {
	if !isHex(pin.SourceCommit, 40) {
		return pinError(name, "/sourceCommit", "source commit must be 40 hexadecimal characters")
	}
	if !isHex(pin.PatchSHA256, 64) {
		return pinError(name, "/patchSHA256", "patch SHA-256 must be 64 hexadecimal characters")
	}
	return nil
}

type RemoteArtifactPin struct {
	URL    string
	SHA256 string
}

func ValidateRemoteArtifactPin(name string, pin RemoteArtifactPin) error {
	if !immutableHTTPSURL(pin.URL) {
		return pinError(name, "/url", "remote artifact URL must be an immutable HTTPS URL")
	}
	if !isHex(pin.SHA256, 64) {
		return pinError(name, "/sha256", "SHA-256 must be 64 hexadecimal characters")
	}
	return nil
}

const LocalPiPackagePath = "../../Projects/gentle-pi"

func ValidateLocalPiPackagePin(name, value string) error {
	if value != LocalPiPackagePath {
		return pinError(name, "/path", "local Pi package must be ../../Projects/gentle-pi")
	}
	return nil
}

var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func exactSemver(value string) bool {
	if !semverPattern.MatchString(value) {
		return false
	}
	withoutBuild := strings.SplitN(value, "+", 2)[0]
	parts := strings.SplitN(withoutBuild, "-", 2)
	if len(parts) == 1 {
		return true
	}
	for _, identifier := range strings.Split(parts[1], ".") {
		if len(identifier) > 1 && identifier[0] == '0' && allDigits(identifier) {
			return false
		}
	}
	return true
}
func validNpmName(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 {
		return false
	}
	if strings.HasPrefix(value, "@") {
		parts := strings.Split(value[1:], "/")
		return len(parts) == 2 && npmAtom(parts[0]) && npmAtom(parts[1])
	}
	return npmAtom(value)
}
func npmAtom(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r == '~' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func pacmanAtom(value string, colon bool) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 {
		return false
	}
	for i, r := range value {
		if i == 0 && !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
		if !(r == '.' || r == '_' || r == '+' || r == '-' || colon && r == ':' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func canonicalRemote(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Path == "" || u.Path == "/" || u.RawPath != "" {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "ssh" && u.Scheme != "file" {
		return false
	}
	if u.Scheme == "file" {
		if u.User != nil || u.Host != "" && strings.ToLower(u.Host) != "localhost" {
			return false
		}
	} else if u.Host == "" || u.User != nil {
		return false
	}
	if !filepath.IsAbs(filepath.FromSlash(u.Path)) || pathpkg.Clean(u.Path) != u.Path || strings.HasSuffix(u.Path, "/") {
		return false
	}
	canonical := *u
	canonical.Host = strings.ToLower(u.Host)
	return value == canonical.String()
}
func immutableHTTPSURL(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || u.Path == "" || u.Path == "/" || u.RawPath != "" {
		return false
	}
	if !filepath.IsAbs(filepath.FromSlash(u.Path)) || pathpkg.Clean(u.Path) != u.Path || strings.HasSuffix(u.Path, "/") {
		return false
	}
	canonical := *u
	canonical.Host = strings.ToLower(u.Host)
	return value == canonical.String() && !mutableArtifactPath(u.Path)
}
func mutableArtifactPath(value string) bool {
	for _, segment := range strings.Split(strings.ToLower(value), "/") {
		switch segment {
		case "latest", "current", "nightly", "snapshot", "main", "master", "head", "trunk", "refs", "heads":
			return true
		}
	}
	base := strings.ToLower(pathpkg.Base(value))
	for _, suffix := range []string{".tar.gz", ".tar.zst", ".tar.xz", ".tgz"} {
		base = strings.TrimSuffix(base, suffix)
	}
	switch base {
	case "latest", "current", "nightly", "snapshot", "main", "master", "head", "trunk":
		return true
	}
	return false
}
func safeAbsolutePath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexByte(value, 0) >= 0 || !filepath.IsAbs(value) || value == string(filepath.Separator) || filepath.Clean(value) != value {
		return false
	}
	return true
}
func pathWithin(root, value string) bool {
	return root == string(filepath.Separator) || value == root || strings.HasPrefix(value, root+string(filepath.Separator))
}
func isHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
func allDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func pinError(name, path, message string) error {
	return &PinValidationError{Pin: name, Path: path, Message: message}
}
