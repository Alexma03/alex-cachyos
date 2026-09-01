package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// Repository owns the declared catalog inventory and resolves it without host
// discovery. Its constructor and results clone all mutable catalog values.
type Repository struct {
	global Document
	roles  map[string]Document
	hosts  map[string]Document
	lookup AssetLookup
}

// ResolvedHostPolicy is one immutable-by-copy desired state and its host-owned
// authorization. Callers receive fresh slices and maps on every Resolve call.
type ResolvedHostPolicy struct {
	Name    string
	Roles   []string
	Desired Catalog
	Risks   RiskPolicy
}

// Allows reports host catalog authority. Runtime evidence is deliberately not
// an input, so observations cannot promote a denied capability.
func (policy ResolvedHostPolicy) Allows(capability RiskCapability) bool {
	return policy.Risks.Allows(capability)
}

// NewRepository captures a catalog inventory. It performs no filesystem or
// hardware access; validation that can consult AssetLookup occurs on Resolve.
func NewRepository(global Document, roles, hosts map[string]Document, lookup AssetLookup) (*Repository, error) {
	if _, exists := roles[""]; exists {
		return nil, fmt.Errorf("catalog role name is empty")
	}
	if _, exists := hosts[""]; exists {
		return nil, fmt.Errorf("catalog host name is empty")
	}
	return &Repository{
		global: cloneDocument(global),
		roles:  cloneDocuments(roles),
		hosts:  cloneDocuments(hosts),
		lookup: lookup,
	}, nil
}

// KnownHosts returns the explicit host inventory in stable order.
func (repository *Repository) KnownHosts() []string {
	if repository == nil {
		return nil
	}
	names := make([]string, 0, len(repository.hosts))
	for name := range repository.hosts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Resolve merges global, named roles in host-declared order, then the host.
// Unknown hosts fail before validation or any AssetLookup call.
func (repository *Repository) Resolve(name string) (ResolvedHostPolicy, error) {
	if repository == nil {
		return ResolvedHostPolicy{}, fmt.Errorf("catalog repository is nil")
	}
	host, exists := repository.hosts[name]
	if !exists {
		return ResolvedHostPolicy{}, fmt.Errorf("unknown host %q; known hosts: %s", name, strings.Join(repository.KnownHosts(), ", "))
	}

	if err := validateRepositoryLayer(repository.global, KindGlobal, "global"); err != nil {
		return ResolvedHostPolicy{}, err
	}
	if err := validateRepositoryLayer(host, KindHost, "host "+name); err != nil {
		return ResolvedHostPolicy{}, err
	}

	roles := documentRoles(host)
	documents := make([]Document, 0, len(roles)+2)
	documents = append(documents, cloneDocument(repository.global))
	seen := make(map[string]struct{}, len(roles))
	for i, roleName := range roles {
		if _, duplicate := seen[roleName]; duplicate {
			return ResolvedHostPolicy{}, fmt.Errorf("/roles/%d: duplicate role %q", i, roleName)
		}
		seen[roleName] = struct{}{}
		role, known := repository.roles[roleName]
		if !known {
			return ResolvedHostPolicy{}, fmt.Errorf("/roles/%d: unknown role %q", i, roleName)
		}
		if err := validateRepositoryLayer(role, KindRole, "role "+roleName); err != nil {
			return ResolvedHostPolicy{}, err
		}
		documents = append(documents, cloneDocument(role))
	}
	documents = append(documents, cloneDocument(host))

	desired, err := MergeDocuments(documents)
	if err != nil {
		return ResolvedHostPolicy{}, fmt.Errorf("resolve host %q: %w", name, err)
	}
	if err := Validate(desired, repository.lookup); err != nil {
		return ResolvedHostPolicy{}, fmt.Errorf("resolve host %q: %w", name, err)
	}

	var risks RiskPolicy
	if host.RiskPolicy != nil {
		risks = *host.RiskPolicy
	}
	return ResolvedHostPolicy{
		Name:    name,
		Roles:   cloneStrings(roles),
		Desired: cloneCatalog(desired),
		Risks:   risks,
	}, nil
}

func validateRepositoryLayer(document Document, want Kind, label string) error {
	if document.CatalogVersion == nil {
		return fmt.Errorf("%s /catalogVersion: required catalog field is missing", label)
	}
	if document.Kind == nil {
		return fmt.Errorf("%s /kind: required catalog field is missing", label)
	}
	if *document.Kind != want {
		return fmt.Errorf("%s /kind: got %q, want %q", label, *document.Kind, want)
	}
	if want != KindHost {
		if document.Roles != nil {
			return fmt.Errorf("%s /roles: roles are host-owned", label)
		}
		if document.RiskPolicy != nil {
			return fmt.Errorf("%s /riskPolicy: risk policy is host-owned", label)
		}
	}
	return nil
}

func documentRoles(document Document) []string {
	if document.Roles == nil {
		return nil
	}
	return copyStrings(*document.Roles)
}

func cloneDocuments(source map[string]Document) map[string]Document {
	if source == nil {
		return nil
	}
	cloned := make(map[string]Document, len(source))
	for name, document := range source {
		cloned[name] = cloneDocument(document)
	}
	return cloned
}

func cloneDocument(source Document) Document {
	cloned := source
	cloned.CatalogVersion = copyPointer(source.CatalogVersion)
	cloned.Kind = copyPointer(source.Kind)
	cloned.Roles = copySlicePointer(source.Roles)
	cloned.RiskPolicy = copyPointer(source.RiskPolicy)
	cloned.Modules = copyModuleSetPointer(source.Modules)
	cloned.Templates = copySlicePointer(source.Templates)
	cloned.Overlays = copySlicePointer(source.Overlays)
	cloned.Pins = clonePinsDocument(source.Pins)
	cloned.CheckoutPins = cloneCheckoutPinDocuments(source.CheckoutPins)
	return cloned
}

func cloneCatalog(source Catalog) Catalog {
	cloned := source
	cloned.Roles = cloneStrings(source.Roles)
	cloned.RiskPolicy = copyPointer(source.RiskPolicy)
	cloned.Modules = cloneModuleSet(source.Modules)
	cloned.Templates = cloneStrings(source.Templates)
	cloned.Overlays = cloneStrings(source.Overlays)
	cloned.Pins = clonePins(source.Pins)
	cloned.CheckoutPins = cloneCheckoutPins(source.CheckoutPins)
	return cloned
}

func cloneStrings(source []string) []string {
	if source == nil {
		return nil
	}
	return append([]string{}, source...)
}

func cloneModuleSet(source ModuleSet) ModuleSet {
	if source == nil {
		return nil
	}
	cloned := make(ModuleSet, len(source))
	for name, enabled := range source {
		cloned[name] = enabled
	}
	return cloned
}

func copyModuleSetPointer(source *ModuleSet) *ModuleSet {
	if source == nil {
		return nil
	}
	value := cloneModuleSet(*source)
	return &value
}

func clonePins(source *Pins) *Pins {
	if source == nil {
		return nil
	}
	return &Pins{
		NPM:               cloneMap(source.NPM),
		PacmanArtifacts:   cloneMap(source.PacmanArtifacts),
		AURLocal:          cloneMap(source.AURLocal),
		RemoteArtifacts:   cloneMap(source.RemoteArtifacts),
		LocalPathPackages: cloneMap(source.LocalPathPackages),
	}
}

func clonePinsDocument(source *PinsDocument) *PinsDocument {
	if source == nil {
		return nil
	}
	return &PinsDocument{
		NPM:               copyMapPointer(source.NPM),
		PacmanArtifacts:   clonePacmanArtifactPinDocuments(source.PacmanArtifacts),
		AURLocal:          cloneAURLocalPinDocuments(source.AURLocal),
		RemoteArtifacts:   cloneRemoteArtifactPinDocuments(source.RemoteArtifacts),
		LocalPathPackages: copyMapPointer(source.LocalPathPackages),
	}
}

func clonePacmanArtifactPinDocuments(source *map[string]PacmanArtifactPinDocument) *map[string]PacmanArtifactPinDocument {
	if source == nil {
		return nil
	}
	if *source == nil {
		var nilMap map[string]PacmanArtifactPinDocument
		return &nilMap
	}
	cloned := make(map[string]PacmanArtifactPinDocument, len(*source))
	for name, pin := range *source {
		cloned[name] = PacmanArtifactPinDocument{
			Package: copyPointer(pin.Package),
			Version: copyPointer(pin.Version),
			Source:  copyPointer(pin.Source),
			SHA256:  copyPointer(pin.SHA256),
		}
	}
	return &cloned
}

func cloneAURLocalPinDocuments(source *map[string]AURLocalPinDocument) *map[string]AURLocalPinDocument {
	if source == nil {
		return nil
	}
	if *source == nil {
		var nilMap map[string]AURLocalPinDocument
		return &nilMap
	}
	cloned := make(map[string]AURLocalPinDocument, len(*source))
	for name, pin := range *source {
		cloned[name] = AURLocalPinDocument{
			SourceCommit: copyPointer(pin.SourceCommit),
			PatchSHA256:  copyPointer(pin.PatchSHA256),
		}
	}
	return &cloned
}

func cloneRemoteArtifactPinDocuments(source *map[string]RemoteArtifactPinDocument) *map[string]RemoteArtifactPinDocument {
	if source == nil {
		return nil
	}
	if *source == nil {
		var nilMap map[string]RemoteArtifactPinDocument
		return &nilMap
	}
	cloned := make(map[string]RemoteArtifactPinDocument, len(*source))
	for name, pin := range *source {
		cloned[name] = RemoteArtifactPinDocument{
			URL:    copyPointer(pin.URL),
			SHA256: copyPointer(pin.SHA256),
		}
	}
	return &cloned
}

func cloneCheckoutPinDocuments(source *map[string]CheckoutPinDocument) *map[string]CheckoutPinDocument {
	if source == nil {
		return nil
	}
	if *source == nil {
		var nilMap map[string]CheckoutPinDocument
		return &nilMap
	}
	cloned := make(map[string]CheckoutPinDocument, len(*source))
	for name, pin := range *source {
		cloned[name] = CheckoutPinDocument{
			Remote: copyPointer(pin.Remote),
			Branch: copyPointer(pin.Branch),
			Commit: copyPointer(pin.Commit),
		}
	}
	return &cloned
}

func cloneCheckoutPins(source map[string]CheckoutPin) map[string]CheckoutPin {
	return cloneMap(source)
}

func copyPointer[T any](source *T) *T {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func copySlicePointer[T any](source *[]T) *[]T {
	if source == nil {
		return nil
	}
	value := append([]T(nil), (*source)...)
	if *source != nil && value == nil {
		value = []T{}
	}
	return &value
}

func copyMapPointer[K comparable, V any](source *map[K]V) *map[K]V {
	if source == nil {
		return nil
	}
	value := cloneMap(*source)
	return &value
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return nil
	}
	cloned := make(map[K]V, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
