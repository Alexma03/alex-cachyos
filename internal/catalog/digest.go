package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

// Normalize returns the compact canonical JSON representation of a Catalog.
// Catalog fields are emitted in schema order, map entries are sorted by key, and
// ordered slices retain their supplied order. A nil optional collection is
// omitted while a non-nil empty collection is emitted. The result has one
// trailing newline.
func Normalize(catalog Catalog) ([]byte, error) {
	var normalized bytes.Buffer
	normalized.WriteByte('{')
	first := true

	if err := writeCatalogField(&normalized, &first, "catalogVersion", func(value *bytes.Buffer) error {
		value.WriteString(strconv.Itoa(catalog.CatalogVersion))
		return nil
	}); err != nil {
		return nil, err
	}
	if err := writeCatalogField(&normalized, &first, "kind", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(catalog.Kind))
	}); err != nil {
		return nil, err
	}
	if catalog.Modules != nil {
		if err := writeCatalogField(&normalized, &first, "modules", func(value *bytes.Buffer) error {
			return writeModuleSet(value, catalog.Modules)
		}); err != nil {
			return nil, err
		}
	}
	if catalog.Templates != nil {
		if err := writeCatalogField(&normalized, &first, "templates", func(value *bytes.Buffer) error {
			return writeStringSlice(value, catalog.Templates)
		}); err != nil {
			return nil, err
		}
	}
	if catalog.Overlays != nil {
		if err := writeCatalogField(&normalized, &first, "overlays", func(value *bytes.Buffer) error {
			return writeStringSlice(value, catalog.Overlays)
		}); err != nil {
			return nil, err
		}
	}
	if catalog.CheckoutPins != nil {
		if err := writeCatalogField(&normalized, &first, "checkoutPins", func(value *bytes.Buffer) error {
			return writeCheckoutPins(value, catalog.CheckoutPins)
		}); err != nil {
			return nil, err
		}
	}

	normalized.WriteByte('}')
	normalized.WriteByte('\n')
	return normalized.Bytes(), nil
}

// Digest returns the lowercase SHA-256 digest of Normalize(catalog).
func Digest(catalog Catalog) (string, error) {
	normalized, err := Normalize(catalog)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(normalized)
	return hex.EncodeToString(digest[:]), nil
}

func writeCatalogField(buffer *bytes.Buffer, first *bool, name string, writeValue func(*bytes.Buffer) error) error {
	if !*first {
		buffer.WriteByte(',')
	}
	*first = false
	if err := writeJSONString(buffer, name); err != nil {
		return err
	}
	buffer.WriteByte(':')
	return writeValue(buffer)
}

func writeJSONString(buffer *bytes.Buffer, value string) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	buffer.Write(encoded)
	return nil
}

func writeModuleSet(buffer *bytes.Buffer, modules ModuleSet) error {
	buffer.WriteByte('{')
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if i != 0 {
			buffer.WriteByte(',')
		}
		if err := writeJSONString(buffer, name); err != nil {
			return err
		}
		buffer.WriteByte(':')
		buffer.WriteString(strconv.FormatBool(modules[name]))
	}
	buffer.WriteByte('}')
	return nil
}

func writeStringSlice(buffer *bytes.Buffer, values []string) error {
	buffer.WriteByte('[')
	for i, value := range values {
		if i != 0 {
			buffer.WriteByte(',')
		}
		if err := writeJSONString(buffer, value); err != nil {
			return err
		}
	}
	buffer.WriteByte(']')
	return nil
}

func writeCheckoutPins(buffer *bytes.Buffer, pins map[string]CheckoutPin) error {
	buffer.WriteByte('{')
	names := make([]string, 0, len(pins))
	for name := range pins {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if i != 0 {
			buffer.WriteByte(',')
		}
		if err := writeJSONString(buffer, name); err != nil {
			return err
		}
		buffer.WriteByte(':')
		if err := writeCheckoutPin(buffer, pins[name]); err != nil {
			return err
		}
	}
	buffer.WriteByte('}')
	return nil
}

func writeCheckoutPin(buffer *bytes.Buffer, pin CheckoutPin) error {
	buffer.WriteByte('{')
	first := true
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "remote", value: pin.Remote},
		{name: "branch", value: pin.Branch},
		{name: "commit", value: pin.Commit},
	} {
		if !first {
			buffer.WriteByte(',')
		}
		first = false
		if err := writeJSONString(buffer, field.name); err != nil {
			return err
		}
		buffer.WriteByte(':')
		if err := writeJSONString(buffer, field.value); err != nil {
			return err
		}
	}
	buffer.WriteByte('}')
	return nil
}
