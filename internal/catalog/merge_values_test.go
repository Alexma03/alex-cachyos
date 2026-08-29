package catalog

import (
	"errors"
	"reflect"
	"testing"
)

func TestApplyListPatchReplacesThenRemovesThenAddsInExplicitOrder(t *testing.T) {
	base := []NamedValue[string]{{ID: "inherited", Value: "discarded"}}
	patch := ListPatch[string]{
		Replace: []NamedValue[string]{
			{ID: "first", Value: "replacement-first"},
			{ID: "remove-me", Value: "replacement-removed"},
			{ID: "second", Value: "replacement-second"},
		},
		Remove: []string{"remove-me"},
		Add: []NamedValue[string]{
			{ID: "added-first", Value: "add-first"},
			{ID: "added-second", Value: "add-second"},
		},
	}

	got, err := ApplyListPatch(base, patch)
	if err != nil {
		t.Fatalf("apply list patch: %v", err)
	}
	want := []NamedValue[string]{
		{ID: "first", Value: "replacement-first"},
		{ID: "second", Value: "replacement-second"},
		{ID: "added-first", Value: "add-first"},
		{ID: "added-second", Value: "add-second"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patched values = %#v, want %#v", got, want)
	}
}

func TestApplyListPatchWithoutReplaceKeepsBaseOrder(t *testing.T) {
	base := []NamedValue[string]{
		{ID: "first", Value: "base-first"},
		{ID: "second", Value: "base-second"},
	}
	got, err := ApplyListPatch(base, ListPatch[string]{
		Remove: []string{"first"},
		Add:    []NamedValue[string]{{ID: "third", Value: "added-third"}},
	})
	if err != nil {
		t.Fatalf("apply list patch: %v", err)
	}
	want := []NamedValue[string]{
		{ID: "second", Value: "base-second"},
		{ID: "third", Value: "added-third"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patched values = %#v, want %#v", got, want)
	}
}

func TestApplyListPatchAllowsRemoveThenAddAndExplicitEmptyReplace(t *testing.T) {
	base := []NamedValue[string]{{ID: "inherited", Value: "discarded"}}
	got, err := ApplyListPatch(base, ListPatch[string]{
		Replace: []NamedValue[string]{},
		Remove:  []string{"reintroduced"},
		Add:     []NamedValue[string]{{ID: "reintroduced", Value: "new value"}},
	})
	if err != nil {
		t.Fatalf("apply empty list patch: %v", err)
	}
	want := []NamedValue[string]{{ID: "reintroduced", Value: "new value"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patched values = %#v, want %#v", got, want)
	}
}

func TestApplyListPatchRejectsDuplicateInputIDs(t *testing.T) {
	tests := []struct {
		name  string
		patch ListPatch[string]
		base  []NamedValue[string]
	}{
		{
			name: "base",
			base: []NamedValue[string]{{ID: "same"}, {ID: "same"}},
		},
		{
			name:  "replace",
			patch: ListPatch[string]{Replace: []NamedValue[string]{{ID: "same"}, {ID: "same"}}},
		},
		{
			name:  "remove",
			patch: ListPatch[string]{Remove: []string{"same", "same"}},
		},
		{
			name:  "add",
			patch: ListPatch[string]{Add: []NamedValue[string]{{ID: "same"}, {ID: "same"}}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ApplyListPatch(test.base, test.patch)
			if !errors.Is(err, ErrDuplicateInputID) {
				t.Fatalf("error = %v, want ErrDuplicateInputID", err)
			}
			var duplicate *DuplicateInputIDError
			if !errors.As(err, &duplicate) || duplicate.ID != "same" {
				t.Fatalf("error = %v, want named duplicate for same", err)
			}
		})
	}
}

func TestApplyListPatchRejectsDuplicateFinalIDs(t *testing.T) {
	_, err := ApplyListPatch(
		[]NamedValue[string]{{ID: "existing", Value: "old"}},
		ListPatch[string]{Add: []NamedValue[string]{{ID: "existing", Value: "new"}}},
	)
	if !errors.Is(err, ErrDuplicateFinalID) {
		t.Fatalf("error = %v, want ErrDuplicateFinalID", err)
	}
	var duplicate *DuplicateFinalIDError
	if !errors.As(err, &duplicate) || duplicate.ID != "existing" {
		t.Fatalf("error = %v, want named duplicate for existing", err)
	}
}

func TestReplaceCompleteDefinitionsReplacesWholeValuesByID(t *testing.T) {
	type definition struct {
		Text  string
		Retry int
	}
	base := []NamedValue[definition]{
		{ID: "step-a", Value: definition{Text: "old text", Retry: 1}},
		{ID: "step-b", Value: definition{Text: "keep", Retry: 2}},
	}
	replacements := []NamedValue[definition]{
		{ID: "step-a", Value: definition{Text: "new text", Retry: 9}},
		{ID: "step-c", Value: definition{Text: "new step", Retry: 3}},
	}

	got, err := ReplaceCompleteDefinitions(base, replacements)
	if err != nil {
		t.Fatalf("replace complete definitions: %v", err)
	}
	want := []NamedValue[definition]{
		{ID: "step-a", Value: definition{Text: "new text", Retry: 9}},
		{ID: "step-b", Value: definition{Text: "keep", Retry: 2}},
		{ID: "step-c", Value: definition{Text: "new step", Retry: 3}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("definitions = %#v, want %#v", got, want)
	}
}

func TestReplaceCompleteDefinitionsRejectsDuplicateInputAndKeepsInputsUnchanged(t *testing.T) {
	base := []NamedValue[string]{{ID: "step", Value: "old"}}
	replacements := []NamedValue[string]{{ID: "step", Value: "new"}, {ID: "step", Value: "again"}}
	_, err := ReplaceCompleteDefinitions(base, replacements)
	if !errors.Is(err, ErrDuplicateInputID) {
		t.Fatalf("error = %v, want ErrDuplicateInputID", err)
	}

	got, err := ReplaceCompleteDefinitions(base, []NamedValue[string]{{ID: "step", Value: "new"}})
	if err != nil {
		t.Fatalf("replace complete definition: %v", err)
	}
	got[0].Value = "changed result"
	if base[0].Value != "old" {
		t.Fatalf("base was mutated through result: %#v", base)
	}
}

func TestMergeValuesDispatchesPrimitivesAndRejectsAbsentKinds(t *testing.T) {
	base := []NamedValue[string]{{ID: "base", Value: "old"}}
	patch := ListPatch[string]{Add: []NamedValue[string]{{ID: "added", Value: "new"}}}
	got, err := MergeValues(ValueKindListPatch, base, patch)
	if err != nil {
		t.Fatalf("dispatch list patch: %v", err)
	}
	want := []NamedValue[string]{
		{ID: "base", Value: "old"},
		{ID: "added", Value: "new"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dispatched values = %#v, want %#v", got, want)
	}

	_, err = MergeValues(ValueKind("webapps"), base, patch)
	var unsupported *UnsupportedValueKindError
	if !errors.Is(err, ErrUnsupportedValueKind) || !errors.As(err, &unsupported) || unsupported.Kind != ValueKind("webapps") {
		t.Fatalf("error = %v, want typed unsupported webapps error", err)
	}

	_, err = MergeValues(ValueKindCompleteDefinition, base, []NamedValue[string]{{ID: "base", Value: "complete"}})
	if err != nil {
		t.Fatalf("dispatch complete definition: %v", err)
	}
}
