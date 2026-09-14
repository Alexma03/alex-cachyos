package receipt
import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
)
const SchemaV1 = "alex-cachyos.receipt/v1"
const Schema = SchemaV1
var (
	ErrInvalid           = errors.New("invalid receipt")
	ErrSensitiveContent  = errors.New("receipt contains prohibited sensitive content")
)
type Receipt struct {
	Schema                     string                       `json:"schema"`
	RunID                      string                       `json:"runId"`
	Command                    string                       `json:"command"`
	StartedAt                  string                       `json:"startedAt"`
	FinishedAt                 string                       `json:"finishedAt"`
	Status                     string                       `json:"status"`
	NoChange                   bool                         `json:"noChange"`
	Host                       Host                         `json:"host"`
	Catalog                    Catalog                      `json:"catalog"`
	Selection                  Selection                    `json:"selection"`
	Plan                       Plan                         `json:"plan"`
	Steps                      []Step                       `json:"steps"`
	ManagedFiles               []ManagedFile                 `json:"managedFiles"`
	Mutations                  []Mutation                    `json:"mutations"`
	DesiredPackages            DesiredPackages               `json:"desiredPackages"`
	DesiredExactPins           DesiredExactPins              `json:"desiredExactPins"`
	ResolvedInstalledVersions  ResolvedInstalledVersions     `json:"resolvedInstalledVersions"`
	SystemTransactions         []SystemTransaction           `json:"systemTransactions"`
	Checkouts                  []Checkout                    `json:"checkouts"`
	Credentials                Credentials                   `json:"credentials"`
	Warnings                   []string                      `json:"warnings"`
	Errors                     []string                      `json:"errors"`
	RollbackOf                 string                        `json:"rollbackOf"`
	ReappliedCatalogTag        string                        `json:"reappliedCatalogTag"`
}
type Host struct {
	Requested string `json:"requested"`
	Resolved  string `json:"resolved"`
	Hostname  string `json:"hostname"`
}
type Catalog struct {
	CatalogVersion string `json:"catalogVersion"`
	Release        string `json:"release"`
	Tag            string `json:"tag"`
	Digest         string `json:"digest"`
	Source         string `json:"source"`
}
type Selection struct {
	Only []string `json:"only"`; With []string `json:"with"`; Without []string `json:"without"`; Remove []string `json:"remove"`
	Update bool `json:"update"`; DryRun bool `json:"dryRun"`
}
type Plan struct { Digest string `json:"digest"`; NetworkRequired []string `json:"networkRequired"` }
type Timing struct { StartedAt string `json:"startedAt"`; FinishedAt string `json:"finishedAt"` }
type Step struct {
	ID string `json:"id"`; Module string `json:"module"`; Scope string `json:"scope"`; Network string `json:"network"`
	Disposition string `json:"disposition"`; Outcome string `json:"outcome"`; Timing Timing `json:"timing"`; ErrorCode string `json:"errorCode"`
}
type ManagedFile struct {
	Path string `json:"path"`; BeforeHash string `json:"beforeHash"`; AfterHash string `json:"afterHash"`; Mode string `json:"mode"`; Backup string `json:"backup"`; Ownership string `json:"ownership"`
}
type Mutation struct {
	Kind string `json:"kind"`; Target string `json:"target"`
	Before json.RawMessage `json:"before"`; After json.RawMessage `json:"after"`; Inverse json.RawMessage `json:"inverse"`; RollbackPrecondition json.RawMessage `json:"rollbackPrecondition"`
}
type DesiredPackages struct {
	PacmanNames []string `json:"pacmanNames"`; PacmanRepositoryPolicy string `json:"pacmanRepositoryPolicy"`; PacmanTransactionPolicy string `json:"pacmanTransactionPolicy"`
}
type DesiredExactPins struct {
	NPM json.RawMessage `json:"npm"`; SourceCheckouts json.RawMessage `json:"sourceCheckouts"`; AURLocalSources json.RawMessage `json:"aurLocalSources"`; Patches json.RawMessage `json:"patches"`; RemoteArtifacts json.RawMessage `json:"remoteArtifacts"`; OptionalPacmanArtifacts json.RawMessage `json:"optionalPacmanArtifacts"`
}
type ResolvedInstalledVersions struct { Pacman json.RawMessage `json:"pacman"`; AUR json.RawMessage `json:"aur"`; NPM json.RawMessage `json:"npm"` }
type SystemTransaction struct {
	Manager string `json:"manager"`; Policy string `json:"policy"`; RequestedNames []json.RawMessage `json:"requestedNames"`; VersionChanges []json.RawMessage `json:"versionChanges"`
}
type Checkout struct {
	Repo string `json:"repo"`; Branch string `json:"branch"`; DesiredCommit string `json:"desiredCommit"`; BeforeCommit string `json:"beforeCommit"`; ResolvedCommit string `json:"resolvedCommit"`
	Dirty bool `json:"dirty"`; DirtyCounts json.RawMessage `json:"dirtyCounts"`; Adopted bool `json:"adopted"`; Cloned bool `json:"cloned"`; Skipped bool `json:"skipped"`
}
type Credentials struct { ReferencedNames []string `json:"referencedNames"` }
func Decode(data []byte) (Receipt, error) {
	if err := Validate(data); err != nil { return Receipt{}, err }
	var r Receipt
	if json.Unmarshal(data, &r) != nil { return Receipt{}, ErrInvalid }
	return r, nil
}
func Parse(data []byte) (Receipt, error) { return Decode(data) }
func ValidateJSON(data []byte) error { return Validate(data) }
func (r Receipt) Validate() error {
	data, err := json.Marshal(r)
	if err != nil { return ErrInvalid }
	return Validate(data)
}
func CanonicalJSON(r Receipt) ([]byte, error) {
	data, err := json.Marshal(r)
	if err != nil { return nil, ErrInvalid }
	if err := Validate(data); err != nil { return nil, err }
	return data, nil
}
func Validate(data []byte) error {
	if err := scanJSON(data); err != nil { return err }
	var top map[string]json.RawMessage
	if json.Unmarshal(data, &top) != nil || top == nil { return ErrInvalid }
	if !validKeys(top,
		[]string{"schema", "runId", "command", "startedAt", "finishedAt", "status", "noChange", "host", "catalog", "selection", "plan", "steps", "managedFiles", "mutations", "desiredPackages", "desiredExactPins", "resolvedInstalledVersions", "systemTransactions", "checkouts", "credentials", "warnings", "errors"},
		[]string{"rollbackOf", "reappliedCatalogTag"}) { return ErrInvalid }
	for _, k := range []string{"runId", "command", "startedAt", "finishedAt", "status"} {
		if !nonemptyString(top, k) { return ErrInvalid }
	}
	if s, ok := stringField(top, "schema"); !ok || s != SchemaV1 || !boolField(top, "noChange") { return ErrInvalid }
	for _, k := range []string{"rollbackOf", "reappliedCatalogTag"} {
		if raw, ok := top[k]; ok && !isString(raw) { return ErrInvalid }
	}
	host, ok := section(top, "host", []string{"requested", "resolved", "hostname"}, nil)
	if !ok || !allStrings(host, "requested", "resolved", "hostname") { return ErrInvalid }
	catalog, ok := section(top, "catalog", []string{"catalogVersion", "release", "tag", "digest", "source"}, nil)
	if !ok || !allStrings(catalog, "catalogVersion", "release", "tag", "digest", "source") { return ErrInvalid }
	selection, ok := section(top, "selection", []string{"only", "with", "without", "remove", "update", "dryRun"}, nil)
	if !ok || !allStringArrays(selection, "only", "with", "without", "remove") || !allBools(selection, "update", "dryRun") { return ErrInvalid }
	plan, ok := section(top, "plan", []string{"digest", "networkRequired"}, nil)
	if !ok || !nonemptyString(plan, "digest") || !stringArray(plan, "networkRequired") { return ErrInvalid }

	if !objectsField(top, "steps", []string{"id", "module", "scope", "network", "disposition", "outcome", "timing", "errorCode"}, nil, []string{"id", "module", "scope", "network", "disposition", "outcome", "errorCode"}, nil, nil, []string{"timing"}, nil) { return ErrInvalid }
	if !objectsField(top, "managedFiles", []string{"path", "beforeHash", "afterHash", "mode", "backup", "ownership"}, nil, []string{"path", "beforeHash", "afterHash", "mode", "backup", "ownership"}, nil, nil, nil, nil) { return ErrInvalid }
	if !objectsField(top, "mutations", []string{"kind", "target", "before", "after", "inverse", "rollbackPrecondition"}, nil, []string{"kind", "target"}, nil, nil, nil, []string{"before", "after", "inverse", "rollbackPrecondition"}) { return ErrInvalid }
	desired, ok := section(top, "desiredPackages", []string{"pacmanNames", "pacmanRepositoryPolicy", "pacmanTransactionPolicy"}, nil)
	if !ok || !stringArray(desired, "pacmanNames") || !allStrings(desired, "pacmanRepositoryPolicy", "pacmanTransactionPolicy") { return ErrInvalid }
	exact, ok := section(top, "desiredExactPins", []string{"npm", "sourceCheckouts", "aurLocalSources", "patches", "remoteArtifacts", "optionalPacmanArtifacts"}, nil)
	if !ok || !allObjectsOrArrays(exact, "npm", "sourceCheckouts", "aurLocalSources", "patches", "remoteArtifacts", "optionalPacmanArtifacts") { return ErrInvalid }
	resolved, ok := section(top, "resolvedInstalledVersions", []string{"pacman", "aur", "npm"}, nil)
	if !ok || !allObjectsOrArrays(resolved, "pacman", "aur", "npm") { return ErrInvalid }
	if !objectsField(top, "systemTransactions", []string{"manager", "policy", "requestedNames", "versionChanges"}, nil, []string{"manager", "policy"}, nil, []string{"requestedNames", "versionChanges"}, nil, nil) { return ErrInvalid }
	if !objectsField(top, "checkouts", []string{"repo", "branch", "desiredCommit", "beforeCommit", "resolvedCommit", "dirty", "dirtyCounts", "adopted", "cloned", "skipped"}, nil, []string{"repo", "branch", "desiredCommit", "beforeCommit", "resolvedCommit"}, []string{"dirty", "adopted", "cloned", "skipped"}, nil, []string{"dirtyCounts"}, nil) { return ErrInvalid }

	credentials, ok := section(top, "credentials", []string{"referencedNames"}, nil)
	if !ok || !credentialNames(credentials) || !stringArray(credentials, "referencedNames") { return ErrInvalid }
	if !stringArrayField(top, "warnings") || !stringArrayField(top, "errors") { return ErrInvalid }
	return nil
}
func scanJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := scanValue(dec, ""); err != nil { return err }
	if _, err := dec.Token(); err != io.EOF { return ErrInvalid }
	return nil
}
func scanValue(dec *json.Decoder, field string) error {
	tok, err := dec.Token()
	if err != nil { return ErrInvalid }
	switch v := tok.(type) {
	case string:
		if prohibitedString(v, field) { return ErrSensitiveContent }
	case json.Delim:
		switch v {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				key, err := dec.Token(); if err != nil { return ErrInvalid }
				name, ok := key.(string); if !ok || seen[name] { return ErrInvalid }
				seen[name] = true
				if sensitiveKey(name, field) { return ErrSensitiveContent }
				if err := scanValue(dec, name); err != nil { return err }
			}
			if end, err := dec.Token(); err != nil || end != json.Delim('}') { return ErrInvalid }
		case '[':
			for dec.More() { if err := scanValue(dec, field); err != nil { return err } }
			if end, err := dec.Token(); err != nil || end != json.Delim(']') { return ErrInvalid }
		}
	}
	return nil
}
var receiptIdentifierPattern = regexp.MustCompile(`^/?[A-Za-z0-9_@~+.-]+(?:[/=:][A-Za-z0-9_@~+.-]+)*$`)
var receiptSecretMarkers = []string{"apikey", "secret", "token", "password", "credential", "authorization", "bearer"}
func normalizeSensitiveMarkers(s string) string { return strings.Map(func(r rune) rune { if unicode.IsLetter(r) || unicode.IsNumber(r) { return unicode.ToLower(r) }; return -1 }, s) }
func prohibitedString(s, field string) bool {
	l := strings.ToLower(s)
	normalized := normalizeSensitiveMarkers(s)
	if (strings.Contains(normalized, "fingerprint") || strings.Contains(normalized, "biometric") || strings.Contains(normalized, "fprint")) && !isBenignBiometricIdentifier(s, field) { return true }
	if strings.Contains(l, "dirty diff") || strings.Contains(l, "diff --git") || strings.Contains(l, "@@ -") { return true }
	trimmed := strings.TrimSpace(l)
	if strings.HasPrefix(trimmed, "--- ") && strings.Contains(l, "\n+++ ") { return true }
	for _, marker := range []string{"api_key", "api-key", "apikey", "token", "password", "secret", "credential", "authorization", "bearer"} {
		for rest := l; ; {
			i := strings.Index(rest, marker); if i < 0 { break }
			rest = rest[i+len(marker):]
			rest = strings.TrimLeft(rest, " \t")
			if strings.HasPrefix(rest, "=") || strings.HasPrefix(rest, ":") { return true }
		}
	}
	return false
}
func isBenignBiometricIdentifier(s, field string) bool {
	if field == "warnings" || field == "errors" {
		switch strings.ToLower(strings.TrimSpace(s)) { case "fingerprint", "fprintd": return true; default: return false }
	}
	normalized := normalizeSensitiveMarkers(s)
	if !receiptIdentifierPattern.MatchString(s) || (!strings.Contains(normalized, "fingerprint") && !strings.Contains(normalized, "biometric") && !strings.Contains(normalized, "fprint")) { return false }
	for _, marker := range receiptSecretMarkers { if strings.Contains(normalized, marker) { return false } }
	return true
}
func sensitiveKey(name, parent string) bool {
	if name == "credentials" && parent == "" || name == "referencedNames" && parent == "credentials" { return false }
	normalized := normalizeSensitiveMarkers(name)
	if strings.Contains(normalized, "referencednames") { return true }
	for _, part := range []string{"argv", "diff", "keyring"} { if strings.Contains(normalized, part) { return true } }
	for _, marker := range receiptSecretMarkers { if strings.Contains(normalized, marker) { return true } }
	if normalized == "auth" || strings.Contains(normalized, "authstate") { return true }
	if strings.Contains(normalized, "biometric") || strings.Contains(normalized, "fingerprint") || strings.Contains(normalized, "fprint") { return !isBenignBiometricIdentifier(name, "") }
	return false
}

func section(parent map[string]json.RawMessage, name string, required, optional []string) (map[string]json.RawMessage, bool) {
	raw, ok := parent[name]; if !ok { return nil, false }
	obj, ok := object(raw); return obj, ok && validKeys(obj, required, optional)
}
func object(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	var obj map[string]json.RawMessage
	if len(bytes.TrimSpace(raw)) == 0 || json.Unmarshal(raw, &obj) != nil || obj == nil { return nil, false }
	return obj, true
}
func validKeys(obj map[string]json.RawMessage, required, optional []string) bool {
	for _, k := range required { if _, ok := obj[k]; !ok { return false } }
	for k := range obj { if !contains(required, k) && !contains(optional, k) { return false } }
	return true
}
func contains(values []string, want string) bool { for _, value := range values { if value == want { return true } }; return false }
func stringField(obj map[string]json.RawMessage, name string) (string, bool) { var value string; raw, ok := obj[name]; return value, ok && json.Unmarshal(raw, &value) == nil }
func nonemptyString(obj map[string]json.RawMessage, name string) bool { value, ok := stringField(obj, name); return ok && value != "" }
func isString(raw json.RawMessage) bool { var value string; return json.Unmarshal(raw, &value) == nil }
func boolField(obj map[string]json.RawMessage, name string) bool { var value bool; raw, ok := obj[name]; return ok && json.Unmarshal(raw, &value) == nil }
func allStrings(obj map[string]json.RawMessage, names ...string) bool { for _, name := range names { if !isString(obj[name]) { return false } }; return true }
func allBools(obj map[string]json.RawMessage, names ...string) bool { for _, name := range names { if !boolField(obj, name) { return false } }; return true }
func arrayField(obj map[string]json.RawMessage, name string) bool { raw, ok := obj[name]; return ok && len(bytes.TrimSpace(raw)) > 0 && bytes.TrimSpace(raw)[0] == '[' }
func stringArray(obj map[string]json.RawMessage, name string) bool { return stringArrayField(obj, name) }
func stringArrayField(obj map[string]json.RawMessage, name string) bool {
	var values []string; raw, ok := obj[name]
	if !ok || !arrayField(obj, name) || json.Unmarshal(raw, &values) != nil { return false }
	for _, value := range values { if value == "" { return false } }
	return true
}
func allStringArrays(obj map[string]json.RawMessage, names ...string) bool { for _, name := range names { if !stringArray(obj, name) { return false } }; return true }
func objectOrArray(raw json.RawMessage) bool { trimmed := bytes.TrimSpace(raw); return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') }
func allObjectsOrArrays(obj map[string]json.RawMessage, names ...string) bool { for _, name := range names { raw, ok := obj[name]; if !ok || !objectOrArray(raw) { return false } }; return true }
func credentialNames(obj map[string]json.RawMessage) bool {
	var names []string; raw, ok := obj["referencedNames"]
	if !ok || json.Unmarshal(raw, &names) != nil { return false }
	for _, name := range names {
		if name == "" || (name[0] < 'A' || name[0] > 'Z') && (name[0] < 'a' || name[0] > 'z') && name[0] != '_' { return false }
		for _, c := range name[1:] { if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' { return false } }
	}
	return true
}
func jsonField(obj map[string]json.RawMessage, name string) bool { raw, ok := obj[name]; return ok && json.Valid(raw) }
func objectsField(parent map[string]json.RawMessage, name string, required, optional, strings_, bools, arrays, objects, raws []string) bool {
	raw, ok := parent[name]; if !ok || !arrayField(parent, name) { return false }
	var items []json.RawMessage; if json.Unmarshal(raw, &items) != nil { return false }
	for _, item := range items {
		obj, ok := object(item); if !ok || !validKeys(obj, required, optional) || !allStrings(obj, strings_...) || !allBools(obj, bools...) { return false }
		for _, field := range arrays { if !arrayField(obj, field) { return false } }
		for _, field := range objects { if _, ok := object(obj[field]); !ok { return false } }
		for _, field := range raws { if !jsonField(obj, field) { return false } }
	}
	return true
}
