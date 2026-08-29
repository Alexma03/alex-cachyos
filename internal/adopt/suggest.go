package adopt

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ProfileDescriptor describes one known host profile for detect-and-adopt
// suggestion. Name is the canonical profile name; Aliases lists additional
// hostnames that select the same profile.
type ProfileDescriptor struct {
	Name    string
	Aliases []string
}

// Suggestion is the result of resolving an explicit hostname against known
// profiles. Exactly one of Matched and Candidates carries the answer: a
// canonical match sets Matched and Profile, while an unmatched or empty
// hostname leaves Matched false and returns the deterministic sorted list of
// known profile names in Candidates.
type Suggestion struct {
	Matched    bool
	Profile    string
	Candidates []string
}

var (
	// ErrEmptyProfileName reports a profile descriptor whose canonical name is
	// blank after normalization.
	ErrEmptyProfileName = errors.New("profile name is empty")
	// ErrDuplicateProfileName reports two profile descriptors that normalize to
	// the same canonical name.
	ErrDuplicateProfileName = errors.New("duplicate profile name")
	// ErrDuplicateHostnameAlias reports an alias that duplicates another alias or
	// a canonical profile name.
	ErrDuplicateHostnameAlias = errors.New("duplicate hostname alias")
)

// SuggestProfile resolves an explicit hostname against known profile
// descriptors without reading DMI, machine-id, machine-info, the environment,
// HOME, the network, or the filesystem. A canonical exact hostname match (or
// alias match) returns that profile; an unmatched or empty hostname returns a
// deterministic sorted suggestion list with no automatic selection.
//
// Normalization trims surrounding whitespace and folds case; a single trailing
// dot is ignored so "Galaxy." and "galaxy" are equivalent. Duplicate or empty
// profile names and duplicate hostname aliases are rejected with typed errors.
func SuggestProfile(hostname string, profiles []ProfileDescriptor) (Suggestion, error) {
	canonical := normalizeHostname(hostname)
	names := make(map[string]string, len(profiles))
	aliases := make(map[string]string, len(profiles))
	candidateNames := make([]string, 0, len(profiles))

	for _, profile := range profiles {
		name := normalizeHostname(profile.Name)
		if name == "" {
			return Suggestion{}, ErrEmptyProfileName
		}
		if _, exists := names[name]; exists {
			return Suggestion{}, fmt.Errorf("%w: %q", ErrDuplicateProfileName, profile.Name)
		}
		if prior, exists := aliases[name]; exists {
			return Suggestion{}, fmt.Errorf("%w: %q (profile %q) collides with %q", ErrDuplicateHostnameAlias, profile.Name, profile.Name, prior)
		}
		names[name] = profile.Name
		candidateNames = append(candidateNames, profile.Name)
		for _, alias := range profile.Aliases {
			normalizedAlias := normalizeHostname(alias)
			if normalizedAlias == "" {
				return Suggestion{}, fmt.Errorf("%w: empty alias on profile %q", ErrDuplicateHostnameAlias, profile.Name)
			}
			if prior, exists := names[normalizedAlias]; exists {
				return Suggestion{}, fmt.Errorf("%w: %q (profile %q) collides with %q", ErrDuplicateHostnameAlias, alias, profile.Name, prior)
			}
			if prior, exists := aliases[normalizedAlias]; exists {
				return Suggestion{}, fmt.Errorf("%w: %q (profile %q) collides with %q", ErrDuplicateHostnameAlias, alias, profile.Name, prior)
			}
			aliases[normalizedAlias] = profile.Name
		}
	}

	sort.Slice(candidateNames, func(i, j int) bool {
		return normalizeHostname(candidateNames[i]) < normalizeHostname(candidateNames[j])
	})

	if canonical != "" {
		if owner, exists := names[canonical]; exists {
			return Suggestion{Matched: true, Profile: owner}, nil
		}
		if owner, exists := aliases[canonical]; exists {
			return Suggestion{Matched: true, Profile: owner}, nil
		}
	}

	candidates := make([]string, len(candidateNames))
	copy(candidates, candidateNames)
	return Suggestion{Candidates: candidates}, nil
}

func normalizeHostname(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.TrimSuffix(value, ".")
	return value
}
