package catalog

import (
	"errors"
	"fmt"
)

// StableID is the string identity shared by future catalog named values.
type StableID = string

// NamedValue associates an opaque complete value with its stable identity.
type NamedValue[T any] struct {
	ID    StableID
	Value T
}

// ListPatch applies non-nil Replace, then Remove, then Add in explicit order.
type ListPatch[T any] struct {
	Replace []NamedValue[T]
	Remove  []StableID
	Add     []NamedValue[T]
}

var (
	ErrDuplicateInputID     = errors.New("duplicate input stable ID")
	ErrDuplicateFinalID     = errors.New("duplicate final stable ID")
	ErrUnsupportedValueKind = errors.New("unsupported catalog value kind")
)

// DuplicateInputIDError identifies an ID repeated in one input sequence.
type DuplicateInputIDError struct{ ID StableID }

func (e *DuplicateInputIDError) Error() string {
	return fmt.Sprintf("%v: %q", ErrDuplicateInputID, e.ID)
}
func (e *DuplicateInputIDError) Unwrap() error { return ErrDuplicateInputID }

// DuplicateFinalIDError identifies an ID repeated by the resulting list.
type DuplicateFinalIDError struct{ ID StableID }

func (e *DuplicateFinalIDError) Error() string {
	return fmt.Sprintf("%v: %q", ErrDuplicateFinalID, e.ID)
}
func (e *DuplicateFinalIDError) Unwrap() error { return ErrDuplicateFinalID }

// ApplyListPatch copies and applies a ListPatch without mutating its inputs.
func ApplyListPatch[T any](base []NamedValue[T], patch ListPatch[T]) ([]NamedValue[T], error) {
	if err := uniqueNamedValues(base); err != nil {
		return nil, err
	}
	if err := uniqueNamedValues(patch.Replace); err != nil {
		return nil, err
	}
	if err := uniqueIDs(patch.Remove); err != nil {
		return nil, err
	}
	if err := uniqueNamedValues(patch.Add); err != nil {
		return nil, err
	}

	var current []NamedValue[T]
	if patch.Replace != nil {
		current = copyNamedValues(patch.Replace)
	} else {
		current = copyNamedValues(base)
	}

	removed := make(map[StableID]struct{}, len(patch.Remove))
	for _, id := range patch.Remove {
		removed[id] = struct{}{}
	}
	result := make([]NamedValue[T], 0, len(current)+len(patch.Add))
	for _, value := range current {
		if _, ok := removed[value.ID]; !ok {
			result = append(result, value)
		}
	}
	result = append(result, patch.Add...)
	if err := uniqueFinalValues(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ReplaceCompleteDefinitions replaces complete values by ID, preserving base order.
func ReplaceCompleteDefinitions[T any](base, replacements []NamedValue[T]) ([]NamedValue[T], error) {
	if err := uniqueNamedValues(base); err != nil {
		return nil, err
	}
	if err := uniqueNamedValues(replacements); err != nil {
		return nil, err
	}

	result := copyNamedValues(base)
	positions := make(map[StableID]int, len(result)+len(replacements))
	for i, value := range result {
		positions[value.ID] = i
	}
	for _, replacement := range replacements {
		if position, ok := positions[replacement.ID]; ok {
			// Assignment is deliberate: the complete definition is not field-merged.
			result[position] = replacement
			continue
		}
		positions[replacement.ID] = len(result)
		result = append(result, replacement)
	}
	if err := uniqueFinalValues(result); err != nil {
		return nil, err
	}
	return result, nil
}

func uniqueNamedValues[T any](values []NamedValue[T]) error {
	seen := make(map[StableID]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value.ID]; ok {
			return &DuplicateInputIDError{ID: value.ID}
		}
		seen[value.ID] = struct{}{}
	}
	return nil
}

func uniqueIDs(ids []StableID) error {
	seen := make(map[StableID]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return &DuplicateInputIDError{ID: id}
		}
		seen[id] = struct{}{}
	}
	return nil
}

func uniqueFinalValues[T any](values []NamedValue[T]) error {
	seen := make(map[StableID]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value.ID]; ok {
			return &DuplicateFinalIDError{ID: value.ID}
		}
		seen[value.ID] = struct{}{}
	}
	return nil
}

func copyNamedValues[T any](values []NamedValue[T]) []NamedValue[T] {
	if values == nil {
		return nil
	}
	copyOfValues := make([]NamedValue[T], len(values))
	copy(copyOfValues, values)
	return copyOfValues
}

// ValueKind identifies a generic merge primitive; future kinds are diagnostic only.
type ValueKind string

const (
	ValueKindListPatch          ValueKind = "listPatch"
	ValueKindCompleteDefinition ValueKind = "completeDefinition"
	ValueKindWebapps            ValueKind = "webapps"
	ValueKindWebApps            ValueKind = ValueKindWebapps
	ValueKindRoutes             ValueKind = "routes"
	ValueKindCommands           ValueKind = "commands"
	ValueKindSteps              ValueKind = "steps"
)

// UnsupportedValueKindError prevents future kinds from silently doing nothing.
type UnsupportedValueKindError struct{ Kind ValueKind }

func (e *UnsupportedValueKindError) Error() string {
	return fmt.Sprintf("%v: %q", ErrUnsupportedValueKind, e.Kind)
}
func (e *UnsupportedValueKindError) Unwrap() error { return ErrUnsupportedValueKind }

// MergeValues dispatches only the generic primitives currently available.
// Schema-specific future kinds intentionally return UnsupportedValueKindError.
func MergeValues[T any](kind ValueKind, base []NamedValue[T], value any) ([]NamedValue[T], error) {
	switch kind {
	case ValueKindListPatch:
		patch, ok := value.(ListPatch[T])
		if !ok {
			if pointer, pointerOK := value.(*ListPatch[T]); pointerOK && pointer != nil {
				patch, ok = *pointer, true
			}
		}
		if !ok {
			return nil, fmt.Errorf("catalog value kind %q expects ListPatch", kind)
		}
		return ApplyListPatch(base, patch)
	case ValueKindCompleteDefinition:
		replacements, ok := value.([]NamedValue[T])
		if !ok {
			return nil, fmt.Errorf("catalog value kind %q expects []NamedValue", kind)
		}
		return ReplaceCompleteDefinitions(base, replacements)
	default:
		return nil, &UnsupportedValueKindError{Kind: kind}
	}
}
