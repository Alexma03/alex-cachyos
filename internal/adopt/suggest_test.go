package adopt

import (
	"errors"
	"reflect"
	"testing"
)

func galaxyDescriptor() ProfileDescriptor {
	return ProfileDescriptor{Name: "galaxy", Aliases: []string{"samsung-galaxy-book", "galaxybook"}}
}

func TestSuggestProfileMatchesCanonicalName(t *testing.T) {
	got, err := SuggestProfile("galaxy", []ProfileDescriptor{galaxyDescriptor()})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Matched || got.Profile != "galaxy" {
		t.Fatalf("suggestion = %#v, want matched galaxy", got)
	}
	if got.Candidates != nil {
		t.Fatalf("candidates = %v, want nil on exact match", got.Candidates)
	}
}

func TestSuggestProfileMatchesCaseAndTrailingDotInsensitively(t *testing.T) {
	for _, hostname := range []string{"Galaxy", "GALAXY", "galaxy.", " galaxy ", "GALAXY."} {
		got, err := SuggestProfile(hostname, []ProfileDescriptor{galaxyDescriptor()})
		if err != nil {
			t.Fatalf("hostname %q: %v", hostname, err)
		}
		if !got.Matched || got.Profile != "galaxy" {
			t.Fatalf("hostname %q: suggestion = %#v, want matched galaxy", hostname, got)
		}
	}
}

func TestSuggestProfileMatchesAlias(t *testing.T) {
	for _, hostname := range []string{"samsung-galaxy-book", "Samsung-Galaxy-Book", "galaxybook."} {
		got, err := SuggestProfile(hostname, []ProfileDescriptor{galaxyDescriptor()})
		if err != nil {
			t.Fatalf("hostname %q: %v", hostname, err)
		}
		if !got.Matched || got.Profile != "galaxy" {
			t.Fatalf("hostname %q: suggestion = %#v, want matched galaxy via alias", hostname, got)
		}
	}
}

func TestSuggestProfileUnmatchedReturnsSortedCandidatesWithoutSelection(t *testing.T) {
	profiles := []ProfileDescriptor{
		{Name: "zeta"},
		{Name: "Alpha"},
		{Name: "galaxy", Aliases: []string{"host-a"}},
	}
	got, err := SuggestProfile("unknown", profiles)
	if err != nil {
		t.Fatal(err)
	}
	if got.Matched || got.Profile != "" {
		t.Fatalf("unmatched suggestion = %#v, want no selection", got)
	}
	// Original declared names are preserved; only the ordering is normalized.
	want := []string{"Alpha", "galaxy", "zeta"}
	if !reflect.DeepEqual(got.Candidates, want) {
		t.Fatalf("candidates = %v, want %v", got.Candidates, want)
	}
}

func TestSuggestProfileEmptyHostnameReturnsCandidatesWithoutSelection(t *testing.T) {
	for _, hostname := range []string{"", "   "} {
		got, err := SuggestProfile(hostname, []ProfileDescriptor{{Name: "galaxy"}, {Name: "alpha"}})
		if err != nil {
			t.Fatalf("hostname %q: %v", hostname, err)
		}
		if got.Matched || got.Profile != "" {
			t.Fatalf("hostname %q: suggestion = %#v, want no selection", hostname, got)
		}
		if !reflect.DeepEqual(got.Candidates, []string{"alpha", "galaxy"}) {
			t.Fatalf("hostname %q: candidates = %v", hostname, got.Candidates)
		}
	}
}

func TestSuggestProfileRejectsEmptyProfileName(t *testing.T) {
	for _, name := range []string{"", "   ", "."} {
		_, err := SuggestProfile("galaxy", []ProfileDescriptor{{Name: name}})
		if !errors.Is(err, ErrEmptyProfileName) {
			t.Fatalf("name %q: error = %v, want ErrEmptyProfileName", name, err)
		}
	}
}

func TestSuggestProfileRejectsDuplicateProfileName(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{{Name: "galaxy"}, {Name: "Galaxy."}})
	if !errors.Is(err, ErrDuplicateProfileName) {
		t.Fatalf("error = %v, want ErrDuplicateProfileName", err)
	}
}

func TestSuggestProfileRejectsDuplicateAlias(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{
		{Name: "galaxy", Aliases: []string{"host-a"}},
		{Name: "other", Aliases: []string{"Host-A."}},
	})
	if !errors.Is(err, ErrDuplicateHostnameAlias) {
		t.Fatalf("error = %v, want ErrDuplicateHostnameAlias", err)
	}
}

func TestSuggestProfileRejectsAliasCollidingWithProfileName(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{
		{Name: "galaxy"},
		{Name: "other", Aliases: []string{"galaxy"}},
	})
	if !errors.Is(err, ErrDuplicateHostnameAlias) {
		t.Fatalf("error = %v, want ErrDuplicateHostnameAlias", err)
	}
}

func TestSuggestProfileRejectsCanonicalNameAfterEarlierAlias(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{
		{Name: "galaxy", Aliases: []string{"other"}},
		{Name: "other"},
	})
	if !errors.Is(err, ErrDuplicateHostnameAlias) {
		t.Fatalf("error = %v, want ErrDuplicateHostnameAlias", err)
	}
}

func TestSuggestProfileRejectsDuplicateAliasWithinOneProfile(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{
		{Name: "galaxy", Aliases: []string{"host-a", "HOST-A."}},
	})
	if !errors.Is(err, ErrDuplicateHostnameAlias) {
		t.Fatalf("error = %v, want ErrDuplicateHostnameAlias", err)
	}
}

func TestSuggestProfileRejectsAliasEqualToOwnName(t *testing.T) {
	_, err := SuggestProfile("galaxy", []ProfileDescriptor{
		{Name: "galaxy", Aliases: []string{"Galaxy."}},
	})
	if !errors.Is(err, ErrDuplicateHostnameAlias) {
		t.Fatalf("error = %v, want ErrDuplicateHostnameAlias", err)
	}
}

func TestSuggestProfileReturnsDefensiveCopies(t *testing.T) {
	profiles := []ProfileDescriptor{{Name: "galaxy"}, {Name: "alpha"}}
	got, err := SuggestProfile("unknown", profiles)
	if err != nil {
		t.Fatal(err)
	}
	got.Candidates[0] = "corrupted"

	again, err := SuggestProfile("unknown", profiles)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Candidates, []string{"alpha", "galaxy"}) {
		t.Fatalf("candidates = %v, want a fresh sorted copy", again.Candidates)
	}
	if again.Matched || again.Profile != "" {
		t.Fatalf("suggestion = %#v, want no selection", again)
	}
}
