package catalog

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"alex-cachyos/internal/assets"
)

// KnownModules is the catalog-owned module registry. Keep it independent from
// the CLI so catalog validation can be used without importing command code.
var KnownModules = []string{"bootstrap", "fingerprint", "devtools", "apps", "vicinae", "desktop", "verify"}

func knownModule(name string) bool {
	for _, known := range KnownModules {
		if name == known {
			return true
		}
	}
	return false
}

// AssetLookup is the read-only filesystem used to resolve catalog asset paths.
type AssetLookup interface {
	Open(name string) (fs.File, error)
}

// CatalogValidationError aggregates typed validation issues in stable path order.
type CatalogValidationError struct{ Issues []ValidationError }

func (e *CatalogValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "catalog validation failed"
	}
	parts := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		parts[i] = issue.Error()
	}
	return "catalog validation: " + strings.Join(parts, "; ")
}

func (e *CatalogValidationError) Errors() []ValidationError {
	return append([]ValidationError(nil), e.Issues...)
}

// Validate checks the typed catalog without accessing the host filesystem.
func Validate(catalog Catalog, lookup AssetLookup) error {
	var issues []ValidationError
	if catalog.CatalogVersion != 1 {
		issues = append(issues, ValidationError{Path: "/catalogVersion", Message: "unsupported catalog version"})
	}
	if catalog.Kind != KindGlobal && catalog.Kind != KindRole && catalog.Kind != KindHost {
		issues = append(issues, ValidationError{Path: "/kind", Message: "unsupported catalog kind"})
	}
	if catalog.Kind != KindHost {
		if catalog.Roles != nil {
			issues = append(issues, ValidationError{Path: "/roles", Message: "roles are host-owned"})
		}
		if catalog.RiskPolicy != nil {
			issues = append(issues, ValidationError{Path: "/riskPolicy", Message: "risk policy is host-owned"})
		}
	}
	seenRoles := make(map[string]struct{}, len(catalog.Roles))
	for i, role := range catalog.Roles {
		path := fmt.Sprintf("/roles/%d", i)
		if role == "" {
			issues = append(issues, ValidationError{Path: path, Message: "role name is empty"})
			continue
		}
		if _, exists := seenRoles[role]; exists {
			issues = append(issues, ValidationError{Path: path, Message: fmt.Sprintf("duplicate role %q", role)})
		}
		seenRoles[role] = struct{}{}
	}

	modules := make([]string, 0, len(catalog.Modules))
	for name := range catalog.Modules {
		modules = append(modules, name)
	}
	sort.Strings(modules)
	for _, name := range modules {
		if !knownModule(name) {
			issues = append(issues, ValidationError{
				Path: childPointer("/modules", name), Message: fmt.Sprintf("unknown module %q", name),
			})
		}
	}
	issues = append(issues, validateAssetRefs("templates", catalog.Templates, lookup)...)
	issues = append(issues, validateAssetRefs("overlays", catalog.Overlays, lookup)...)

	pins := make([]string, 0, len(catalog.CheckoutPins))
	for name := range catalog.CheckoutPins {
		pins = append(pins, name)
	}
	sort.Strings(pins)
	for _, name := range pins {
		pin := catalog.CheckoutPins[name]
		for _, field := range []struct {
			name, value string
		}{{"remote", pin.Remote}, {"branch", pin.Branch}, {"commit", pin.Commit}} {
			if field.value == "" {
				issues = append(issues, ValidationError{
					Path:    childPointer(childPointer("/checkoutPins", name), field.name),
					Message: "required checkout-pin field is empty",
				})
			}
		}
	}
	if len(issues) == 0 {
		return nil
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Message < issues[j].Message
		}
		return issues[i].Path < issues[j].Path
	})
	return &CatalogValidationError{Issues: issues}
}

func validateAssetRefs(field string, refs []string, lookup AssetLookup) []ValidationError {
	var issues []ValidationError
	for i, ref := range refs {
		path := fmt.Sprintf("/%s/%d", field, i)
		if lookup == nil {
			issues = append(issues, ValidationError{Path: path, Message: fmt.Sprintf("asset %q cannot be resolved: asset lookup is unavailable", ref)})
		} else if _, err := fs.Stat(lookup, ref); err != nil {
			issues = append(issues, ValidationError{Path: path, Message: fmt.Sprintf("asset %q is missing", ref)})
		}
	}
	return issues
}

// Validate checks the catalog with the receiver as the typed value.
func (catalog Catalog) Validate(lookup AssetLookup) error { return Validate(catalog, lookup) }

// Load performs strict YAML decoding, JSON Schema validation, then typed
// validation against the supplied read-only asset lookup.
func Load(data []byte, lookup AssetLookup) (Catalog, error) {
	catalog, err := Decode(data)
	if err != nil {
		return Catalog{}, err
	}
	if err := Validate(catalog, lookup); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// LoadEmbedded validates a catalog using only the immutable embedded asset tree.
func LoadEmbedded(data []byte) (Catalog, error) { return Load(data, assets.FS) }
