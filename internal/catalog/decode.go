package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

// Decode parses one YAML catalog document, validates its JSON-compatible form
// with the WU-3a schema validator, and returns typed values.
func Decode(data []byte) (Catalog, error) {
	document, err := DecodeDocument(data)
	if err != nil {
		return Catalog{}, err
	}
	return catalogFromDocument(document)
}

// DecodeCatalog is the descriptive alias for Decode.
func DecodeCatalog(data []byte) (Catalog, error) { return Decode(data) }

// DecodeDocument parses and schema-validates one YAML document while retaining
// field presence for the later merge layer.
func DecodeDocument(data []byte) (Document, error) {
	document, err := parseDocument(data)
	if err != nil {
		return Document{}, err
	}
	jsonData, err := jsonFromYAML(data)
	if err != nil {
		return Document{}, fmt.Errorf("convert catalog YAML to JSON: %w", err)
	}
	if err := ValidateJSON(jsonData); err != nil {
		return Document{}, err
	}
	return document, nil
}

func parseDocument(data []byte) (Document, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var document Document
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return Document{}, errors.New("catalog YAML is empty")
		}
		return Document{}, fmt.Errorf("decode catalog YAML: %w", err)
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Document{}, errors.New("catalog YAML contains multiple documents")
		}
		return Document{}, fmt.Errorf("decode catalog YAML: %w", err)
	}
	return document, nil
}

func jsonFromYAML(data []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("catalog YAML is empty")
		}
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("catalog YAML contains multiple documents")
		}
		return nil, err
	}
	value, err := jsonValue(document)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func jsonValue(node yaml.Node) (any, error) {
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return nil, errors.New("catalog YAML has no document content")
		}
		return jsonValue(*node.Content[0])
	}
	switch node.Kind {
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return nil, errors.New("catalog YAML mapping has no value")
		}
		value := make(map[string]any, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, fmt.Errorf("catalog YAML key %q is not a string", key.Value)
			}
			if _, exists := value[key.Value]; exists {
				return nil, fmt.Errorf("catalog YAML contains duplicate key %q", key.Value)
			}
			item, err := jsonValue(*node.Content[i+1])
			if err != nil {
				return nil, err
			}
			value[key.Value] = item
		}
		return value, nil
	case yaml.SequenceNode:
		value := make([]any, len(node.Content))
		for i, item := range node.Content {
			converted, err := jsonValue(*item)
			if err != nil {
				return nil, err
			}
			value[i] = converted
		}
		return value, nil
	case yaml.AliasNode:
		if node.Alias == nil {
			return nil, errors.New("catalog YAML alias has no target")
		}
		return jsonValue(*node.Alias)
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!null":
			return nil, nil
		case "!!bool":
			var value bool
			if err := node.Decode(&value); err != nil {
				return nil, err
			}
			return value, nil
		case "!!int":
			var value int64
			if err := node.Decode(&value); err == nil {
				return value, nil
			}
			var unsigned uint64
			if err := node.Decode(&unsigned); err != nil {
				return nil, err
			}
			return unsigned, nil
		case "!!float":
			var value float64
			if err := node.Decode(&value); err != nil {
				return nil, err
			}
			return value, nil
		case "!!str":
			return node.Value, nil
		default:
			return nil, fmt.Errorf("catalog YAML scalar %q has unsupported tag %q", node.Value, node.Tag)
		}
	default:
		return nil, fmt.Errorf("catalog YAML has unsupported node kind %d", node.Kind)
	}
}

func catalogFromDocument(document Document) (Catalog, error) {
	if document.CatalogVersion == nil || document.Kind == nil {
		return Catalog{}, errors.New("catalog document is missing required fields")
	}
	catalog := Catalog{CatalogVersion: *document.CatalogVersion, Kind: *document.Kind}
	if document.Modules != nil {
		catalog.Modules = make(ModuleSet, len(*document.Modules))
		for name, enabled := range *document.Modules {
			catalog.Modules[name] = enabled
		}
	}
	if document.Templates != nil {
		catalog.Templates = make([]string, len(*document.Templates))
		copy(catalog.Templates, *document.Templates)
	}
	if document.Overlays != nil {
		catalog.Overlays = make([]string, len(*document.Overlays))
		copy(catalog.Overlays, *document.Overlays)
	}
	if document.Pins != nil {
		pins, err := pinsFromDocument(document.Pins)
		if err != nil {
			return Catalog{}, err
		}
		catalog.Pins = pins
	}
	if document.CheckoutPins != nil {
		catalog.CheckoutPins = make(map[string]CheckoutPin, len(*document.CheckoutPins))
		for name, pin := range *document.CheckoutPins {
			if pin.Remote == nil || pin.Branch == nil || pin.Commit == nil {
				return Catalog{}, fmt.Errorf("checkout pin %q is missing a required field", name)
			}
			catalog.CheckoutPins[name] = CheckoutPin{Remote: *pin.Remote, Branch: *pin.Branch, Commit: *pin.Commit}
		}
	}
	return catalog, nil
}

func pinsFromDocument(document *PinsDocument) (*Pins, error) {
	pins := &Pins{}
	if document.NPM != nil {
		pins.NPM = copyStringMap(*document.NPM)
	}
	if document.PacmanArtifacts != nil {
		pins.PacmanArtifacts = make(map[string]PacmanArtifactPin, len(*document.PacmanArtifacts))
		names := sortedPinNames(*document.PacmanArtifacts)
		for _, name := range names {
			pin := (*document.PacmanArtifacts)[name]
			packageName, err := requiredPinField("pacmanArtifacts", name, "package", pin.Package)
			if err != nil {
				return nil, err
			}
			version, err := requiredPinField("pacmanArtifacts", name, "version", pin.Version)
			if err != nil {
				return nil, err
			}
			source, err := requiredPinField("pacmanArtifacts", name, "source", pin.Source)
			if err != nil {
				return nil, err
			}
			sha256, err := requiredPinField("pacmanArtifacts", name, "sha256", pin.SHA256)
			if err != nil {
				return nil, err
			}
			pins.PacmanArtifacts[name] = PacmanArtifactPin{
				Package: packageName,
				Version: version,
				Source:  PacmanArtifactSource(source),
				SHA256:  sha256,
			}
		}
	}
	if document.AURLocal != nil {
		pins.AURLocal = make(map[string]AURLocalPin, len(*document.AURLocal))
		names := sortedPinNames(*document.AURLocal)
		for _, name := range names {
			pin := (*document.AURLocal)[name]
			sourceCommit, err := requiredPinField("aurLocal", name, "sourceCommit", pin.SourceCommit)
			if err != nil {
				return nil, err
			}
			patchSHA256, err := requiredPinField("aurLocal", name, "patchSHA256", pin.PatchSHA256)
			if err != nil {
				return nil, err
			}
			pins.AURLocal[name] = AURLocalPin{SourceCommit: sourceCommit, PatchSHA256: patchSHA256}
		}
	}
	if document.RemoteArtifacts != nil {
		pins.RemoteArtifacts = make(map[string]RemoteArtifactPin, len(*document.RemoteArtifacts))
		names := sortedPinNames(*document.RemoteArtifacts)
		for _, name := range names {
			pin := (*document.RemoteArtifacts)[name]
			url, err := requiredPinField("remoteArtifacts", name, "url", pin.URL)
			if err != nil {
				return nil, err
			}
			sha256, err := requiredPinField("remoteArtifacts", name, "sha256", pin.SHA256)
			if err != nil {
				return nil, err
			}
			pins.RemoteArtifacts[name] = RemoteArtifactPin{URL: url, SHA256: sha256}
		}
	}
	if document.LocalPathPackages != nil {
		pins.LocalPathPackages = copyStringMap(*document.LocalPathPackages)
	}
	return pins, nil
}

func copyStringMap(values map[string]string) map[string]string {
	copyOfValues := make(map[string]string, len(values))
	for name, value := range values {
		copyOfValues[name] = value
	}
	return copyOfValues
}

func sortedPinNames[T any](pins map[string]T) []string {
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func requiredPinField(source, name, field string, value *string) (string, error) {
	if value == nil {
		path := childPointer(childPointer(childPointer("/pins", source), name), field)
		return "", fmt.Errorf("%s: required pin field is missing", path)
	}
	return *value, nil
}
