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
	if source.PacmanArtifacts != nil {
		mergePacmanArtifactSource(&merged.Pins.PacmanArtifacts, source.PacmanArtifacts, issues)
	}
	if source.AURLocal != nil {
		mergeAURLocalSource(&merged.Pins.AURLocal, source.AURLocal, issues)
	}
	if source.RemoteArtifacts != nil {
		mergeRemoteArtifactSource(&merged.Pins.RemoteArtifacts, source.RemoteArtifacts, issues)
	}
}

// mergePacmanArtifactSource converts and merges one document's pacmanArtifacts
// map. A present pointer to a nil map fails with the source path, while each entry
// requires every schema-required field and fails with its named field path when one
// is missing. Complete typed values replace same-key inherited values.
func mergePacmanArtifactSource(destination *map[string]PacmanArtifactPin, source *map[string]PacmanArtifactPinDocument, issues *[]ValidationError) {
	if *source == nil {
		*issues = append(*issues, ValidationError{
			Path: "/pins/pacmanArtifacts", Message: "pin definitions are nil",
		})
		return
	}
	if *destination == nil {
		*destination = make(map[string]PacmanArtifactPin)
	}
	for _, name := range sortedPinNames(*source) {
		pin := (*source)[name]
		packageName, packageOK := requiredMergePinField("pacmanArtifacts", name, "package", pin.Package, issues)
		version, versionOK := requiredMergePinField("pacmanArtifacts", name, "version", pin.Version, issues)
		sourceName, sourceOK := requiredMergePinField("pacmanArtifacts", name, "source", pin.Source, issues)
		sha256, sha256OK := requiredMergePinField("pacmanArtifacts", name, "sha256", pin.SHA256, issues)
		if !(packageOK && versionOK && sourceOK && sha256OK) {
			continue
		}
		(*destination)[name] = PacmanArtifactPin{
			Package: packageName,
			Version: version,
			Source:  PacmanArtifactSource(sourceName),
			SHA256:  sha256,
		}
	}
}

// mergeAURLocalSource converts and merges one document's aurLocal map under the
// same presence and complete-value replacement rules as other typed pin sources.
func mergeAURLocalSource(destination *map[string]AURLocalPin, source *map[string]AURLocalPinDocument, issues *[]ValidationError) {
	if *source == nil {
		*issues = append(*issues, ValidationError{
			Path: "/pins/aurLocal", Message: "pin definitions are nil",
		})
		return
	}
	if *destination == nil {
		*destination = make(map[string]AURLocalPin)
	}
	for _, name := range sortedPinNames(*source) {
		pin := (*source)[name]
		sourceCommit, sourceOK := requiredMergePinField("aurLocal", name, "sourceCommit", pin.SourceCommit, issues)
		patchSHA256, patchOK := requiredMergePinField("aurLocal", name, "patchSHA256", pin.PatchSHA256, issues)
		if !(sourceOK && patchOK) {
			continue
		}
		(*destination)[name] = AURLocalPin{SourceCommit: sourceCommit, PatchSHA256: patchSHA256}
	}
}

// mergeRemoteArtifactSource converts and merges one document's remoteArtifacts map
// under the same presence and complete-value replacement rules as other typed pin
// sources.
func mergeRemoteArtifactSource(destination *map[string]RemoteArtifactPin, source *map[string]RemoteArtifactPinDocument, issues *[]ValidationError) {
	if *source == nil {
		*issues = append(*issues, ValidationError{
			Path: "/pins/remoteArtifacts", Message: "pin definitions are nil",
		})
		return
	}
	if *destination == nil {
		*destination = make(map[string]RemoteArtifactPin)
	}
	for _, name := range sortedPinNames(*source) {
		pin := (*source)[name]
		url, urlOK := requiredMergePinField("remoteArtifacts", name, "url", pin.URL, issues)
		sha256, shaOK := requiredMergePinField("remoteArtifacts", name, "sha256", pin.SHA256, issues)
		if !(urlOK && shaOK) {
			continue
		}
		(*destination)[name] = RemoteArtifactPin{URL: url, SHA256: sha256}
	}
}

// requiredMergePinField converts a required field pointer into a value, or appends
// a named-field validation issue and reports false.
func requiredMergePinField(source, name, field string, value *string, issues *[]ValidationError) (string, bool) {
	if value == nil {
		*issues = append(*issues, ValidationError{
			Path:    childPointer(childPointer(childPointer("/pins", source), name), field),
			Message: "required pin field is missing",
		})
		return "", false
	}
	return *value, true
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
