package catalog

import (
	"errors"
	"sort"
)

// MergeDocuments applies documents in their supplied precedence order. Callers
// should supply the layers as global, each role in declared order, then host.
// Omitted fields inherit; present fields, including explicit empty values,
// participate in the field-specific merge rule below. The merge is entirely
// in-memory and does not perform asset validation or host I/O.
func MergeDocuments(documents []Document) (Catalog, error) {
	if len(documents) == 0 {
		return Catalog{}, errors.New("catalog merge requires at least one document")
	}

	var merged Catalog
	var issues []ValidationError
	for _, document := range documents {
		if document.CatalogVersion == nil {
			issues = append(issues, ValidationError{
				Path: "/catalogVersion", Message: "required catalog field is missing",
			})
		} else {
			merged.CatalogVersion = *document.CatalogVersion
			if merged.CatalogVersion != 1 {
				issues = append(issues, ValidationError{
					Path: "/catalogVersion", Message: "unsupported catalog version",
				})
			}
		}
		if document.Kind == nil {
			issues = append(issues, ValidationError{
				Path: "/kind", Message: "required catalog field is missing",
			})
		} else {
			merged.Kind = *document.Kind
			if merged.Kind != KindGlobal && merged.Kind != KindRole && merged.Kind != KindHost {
				issues = append(issues, ValidationError{
					Path: "/kind", Message: "unsupported catalog kind",
				})
			}
		}

		if document.Modules != nil {
			if merged.Modules == nil {
				merged.Modules = make(ModuleSet)
			}
			for _, name := range sortedModuleNames(*document.Modules) {
				merged.Modules[name] = (*document.Modules)[name]
			}
		}
		if document.Templates != nil {
			merged.Templates = copyStrings(*document.Templates)
		}
		if document.Overlays != nil {
			merged.Overlays = copyStrings(*document.Overlays)
		}
		if document.CheckoutPins != nil {
			mergeCheckoutPins(&merged, document.CheckoutPins, &issues)
		}
		if document.Pins != nil {
			mergePins(&merged, document.Pins, &issues)
		}
	}

	if len(issues) != 0 {
		sortValidationIssues(issues)
		return Catalog{}, &CatalogValidationError{Issues: issues}
	}
	return merged, nil
}

// mergePins applies one document's source-specific pins. A present top-level pins
// object establishes presence on the merged catalog; an omitted top-level object
// inherits. Each present source map merges recursively by stable key with later
// same-key complete value replacement, while an omitted source map inherits.
// Explicitly empty source maps remain present but never delete inherited keys.
// pacmanArtifacts, aurLocal, and remoteArtifacts have no merge support until
// WU-5b3b, so any present pointer must fail closed instead of dropping data.
func mergePins(merged *Catalog, source *PinsDocument, issues *[]ValidationError) {
	if source == nil {
		*issues = append(*issues, ValidationError{Path: "/pins", Message: "pin definitions are nil"})
		return
	}
	if merged.Pins == nil {
		merged.Pins = &Pins{}
	}
	if source.NPM != nil {
		mergeStringPinSource(&merged.Pins.NPM, source.NPM, "npm", issues)
	}
	if source.LocalPathPackages != nil {
		mergeStringPinSource(&merged.Pins.LocalPathPackages, source.LocalPathPackages, "localPathPackages", issues)
	}
	appendUnsupportedPinSource(issues, "pacmanArtifacts", source.PacmanArtifacts)
	appendUnsupportedPinSource(issues, "aurLocal", source.AURLocal)
	appendUnsupportedPinSource(issues, "remoteArtifacts", source.RemoteArtifacts)
}

// appendUnsupportedPinSource fails closed for a pin source whose merge rule is
// not yet implemented: even an explicit empty map is present data that cannot be
// carried into the merged catalog without silently dropping it.
func appendUnsupportedPinSource[T any](issues *[]ValidationError, name string, source *map[string]T) {
	if source == nil {
		return
	}
	*issues = append(*issues, ValidationError{
		Path:    childPointer("/pins", name),
		Message: "pin source merge is not yet supported",
	})
}

func mergeStringPinSource(destination *map[string]string, source *map[string]string, name string, issues *[]ValidationError) {
	if *source == nil {
		*issues = append(*issues, ValidationError{
			Path: childPointer("/pins", name), Message: "pin definitions are nil",
		})
		return
	}
	if *destination == nil {
		*destination = make(map[string]string)
	}
	for _, key := range sortedPinNames(*source) {
		(*destination)[key] = (*source)[key]
	}
}

func mergeCheckoutPins(merged *Catalog, source *map[string]CheckoutPinDocument, issues *[]ValidationError) {
	if *source == nil {
		*issues = append(*issues, ValidationError{
			Path: "/checkoutPins", Message: "checkout-pin definitions are nil",
		})
		return
	}
	if merged.CheckoutPins == nil {
		merged.CheckoutPins = make(map[string]CheckoutPin)
	}

	// A YAML map cannot contain two entries with the same key: DecodeDocument
	// rejects such input before a Document reaches this function. Consequently,
	// same-layer duplicate checkout-pin definitions are not representable here;
	// this map merge only replaces a key across successive layers.
	for _, name := range sortedCheckoutPinNames(*source) {
		pin := (*source)[name]
		valid := true
		for _, field := range []struct {
			name  string
			value *string
		}{
			{name: "remote", value: pin.Remote},
			{name: "branch", value: pin.Branch},
			{name: "commit", value: pin.Commit},
		} {
			path := childPointer(childPointer("/checkoutPins", name), field.name)
			switch {
			case field.value == nil:
				*issues = append(*issues, ValidationError{
					Path: path, Message: "required checkout-pin field is missing",
				})
				valid = false
			case *field.value == "":
				*issues = append(*issues, ValidationError{
					Path: path, Message: "required checkout-pin field is empty",
				})
				valid = false
			}
		}
		if valid {
			merged.CheckoutPins[name] = CheckoutPin{
				Remote: *pin.Remote,
				Branch: *pin.Branch,
				Commit: *pin.Commit,
			}
		}
	}
}

func sortedModuleNames(modules ModuleSet) []string {
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedCheckoutPinNames(pins map[string]CheckoutPinDocument) []string {
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func copyStrings(values []string) []string {
	copyOfValues := make([]string, len(values))
	copy(copyOfValues, values)
	return copyOfValues
}

func sortValidationIssues(issues []ValidationError) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Path == issues[j].Path {
			return issues[i].Message < issues[j].Message
		}
		return issues[i].Path < issues[j].Path
	})
}
