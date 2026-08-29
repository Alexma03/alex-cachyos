package planner

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Normalize returns the compact canonical JSON representation of a Plan.
//
// The top-level object and every nested object are emitted with stable key
// order matching the plan model (see design §4.1). Selection slices and each
// step's dependency list are treated as unordered sets: they are sorted and
// deduplicated without mutating the input. The ordered step slice preserves
// plan order. Desired, observed, and inverse JSON payloads are decoded and
// re-encoded canonically so object keys are stable regardless of their input
// order. The result ends with exactly one newline.
func Normalize(plan Plan) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	first := true

	if err := writeField(&buffer, &first, "selection", func(value *bytes.Buffer) error {
		return writeSelection(value, plan.Selection)
	}); err != nil {
		return nil, err
	}
	if err := writeField(&buffer, &first, "steps", func(value *bytes.Buffer) error {
		return writeSteps(value, plan.Steps)
	}); err != nil {
		return nil, err
	}

	buffer.WriteByte('}')
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}

// Digest returns the lowercase SHA-256 digest of Normalize(plan).
func Digest(plan Plan) (string, error) {
	normalized, err := Normalize(plan)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}

// writeField writes `"name":` and delegates the value to writeValue, inserting
// a comma separator before every field except the first in the enclosing object.
func writeField(buffer *bytes.Buffer, first *bool, name string, writeValue func(*bytes.Buffer) error) error {
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

func writeSelection(buffer *bytes.Buffer, selection Selection) error {
	buffer.WriteByte('{')
	first := true
	for _, field := range []struct {
		name   string
		values []string
	}{
		{name: "only", values: selection.Only},
		{name: "with", values: selection.With},
		{name: "without", values: selection.Without},
	} {
		values := field.values
		if err := writeField(buffer, &first, field.name, func(value *bytes.Buffer) error {
			return writeStringSet(value, values)
		}); err != nil {
			return err
		}
	}
	buffer.WriteByte('}')
	return nil
}

func writeSteps(buffer *bytes.Buffer, steps []Step) error {
	buffer.WriteByte('[')
	for i, step := range steps {
		if i != 0 {
			buffer.WriteByte(',')
		}
		if err := writeStep(buffer, step); err != nil {
			return err
		}
	}
	buffer.WriteByte(']')
	return nil
}

func writeStep(buffer *bytes.Buffer, step Step) error {
	buffer.WriteByte('{')
	first := true

	if err := writeField(buffer, &first, "id", func(value *bytes.Buffer) error {
		return writeJSONString(value, step.ID)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "module", func(value *bytes.Buffer) error {
		return writeJSONString(value, step.Module)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "description", func(value *bytes.Buffer) error {
		return writeJSONString(value, step.Description)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "dependsOn", func(value *bytes.Buffer) error {
		return writeStringSet(value, step.DependsOn)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "scope", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(step.Scope))
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "network", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(step.Network))
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "operation", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(step.Operation))
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "desired", func(value *bytes.Buffer) error {
		return writeCanonicalJSON(value, step.Desired)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "observed", func(value *bytes.Buffer) error {
		return writeCanonicalJSON(value, step.Observed)
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "disposition", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(step.Disposition))
	}); err != nil {
		return err
	}
	if step.Inverse != nil {
		if err := writeField(buffer, &first, "inverse", func(value *bytes.Buffer) error {
			return writeInverse(value, *step.Inverse)
		}); err != nil {
			return err
		}
	}

	buffer.WriteByte('}')
	return nil
}

func writeInverse(buffer *bytes.Buffer, inverse InverseDescriptor) error {
	buffer.WriteByte('{')
	first := true

	if err := writeField(buffer, &first, "operation", func(value *bytes.Buffer) error {
		return writeJSONString(value, string(inverse.Operation))
	}); err != nil {
		return err
	}
	if err := writeField(buffer, &first, "value", func(value *bytes.Buffer) error {
		return writeCanonicalJSON(value, inverse.Value)
	}); err != nil {
		return err
	}

	buffer.WriteByte('}')
	return nil
}

// writeStringSet writes values as a JSON array treated as an unordered set:
// sorted and deduplicated. It never mutates the input slice.
func writeStringSet(buffer *bytes.Buffer, values []string) error {
	set := normalizedSet(values)
	buffer.WriteByte('[')
	for i, value := range set {
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

func normalizedSet(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// writeCanonicalJSON writes a JSON payload in compact canonical form. A nil
// payload is written as JSON null. Non-nil payloads are decoded and re-encoded
// so object keys are sorted, and malformed or multi-value payloads are
// rejected rather than serialized verbatim.
func writeCanonicalJSON(buffer *bytes.Buffer, raw json.RawMessage) error {
	if raw == nil {
		buffer.WriteString("null")
		return nil
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return err
	}
	buffer.Write(canonical)
	return nil
}

func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid JSON payload: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid JSON payload: multiple top-level values")
		}
		return nil, fmt.Errorf("invalid JSON payload: %w", err)
	}

	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("cannot canonicalize JSON payload: %w", err)
	}
	return canonical, nil
}
