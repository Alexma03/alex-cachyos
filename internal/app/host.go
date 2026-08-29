package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// HostPort is the minimal host observation boundary for host resolution. It
// exposes only the machine hostname. DMI, /etc/machine-id, /etc/machine-info,
// and every other hardware or machine identifier are intentionally absent from
// this interface and cannot be read through it.
type HostPort interface {
	// Hostname returns the machine hostname. A non-empty value must be returned
	// with a nil error; an empty value or a non-nil error means the hostname is
	// unavailable and resolution must fail closed.
	Hostname() (string, error)
}

// KnownHost names one concrete host profile. Name is the canonical host name;
// there is no alias authority in the catalog.
type KnownHost struct {
	Name string
}

var (
	// ErrUnknownHost reports that a host name did not match any known host.
	ErrUnknownHost = errors.New("unknown host")
	// ErrHostnameUnavailable reports that the machine hostname could not be
	// obtained. It never carries the port's underlying error.
	ErrHostnameUnavailable = errors.New("hostname unavailable")
	// ErrInvalidKnownHost reports a malformed known-host declaration.
	ErrInvalidKnownHost = errors.New("invalid known host")
)

// UnknownHostError identifies the host name that failed resolution and the
// deterministic sorted list of known canonical host names. It never includes
// machine identifiers or the underlying port error.
type UnknownHostError struct {
	Requested string
	Known     []string
}

func (e *UnknownHostError) Error() string {
	if e == nil {
		return ErrUnknownHost.Error()
	}
	return fmt.Sprintf("unknown host %q (known hosts: %s)", e.Requested, strings.Join(e.Known, ", "))
}

func (e *UnknownHostError) Unwrap() error { return ErrUnknownHost }

// InvalidKnownHostError reports a malformed known-host declaration. Its
// diagnostic is deterministic regardless of the input order: declarations are
// sorted by normalized name before validation, and the first offending
// declaration in that sorted order is reported.
type InvalidKnownHostError struct {
	Name   string
	Reason string
}

func (e *InvalidKnownHostError) Error() string {
	if e == nil {
		return ErrInvalidKnownHost.Error()
	}
	return fmt.Sprintf("%s: %q (%s)", ErrInvalidKnownHost, e.Name, e.Reason)
}

func (e *InvalidKnownHostError) Unwrap() error { return ErrInvalidKnownHost }

// ResolveHost resolves the active host profile. An explicit non-empty host
// takes precedence and is resolved without calling the HostPort; it is the
// caller's responsibility to preserve case on the requested name. When no
// explicit host is supplied, Hostname is called exactly once and the result is
// matched against known canonical names.
//
// Matching is deterministic and documented: canonical names are compared after
// trimming surrounding whitespace, folding case, and ignoring a single
// trailing dot. No aliases or implicit fuzzy matching are performed. An
// unmatched or empty hostname, an unknown explicit host, or an unavailable
// hostname fails closed before any mutation with a typed error listing the
// sorted known canonical host names. The returned value is always the
// canonical host name.
func ResolveHost(explicit string, known []KnownHost, port HostPort) (string, error) {
	index, err := newHostIndex(known)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(explicit) != "" {
		if canonical, ok := index.lookup(explicit); ok {
			return canonical, nil
		}
		return "", newUnknownHostError(explicit, index.names())
	}

	hostname, err := port.Hostname()
	if err != nil {
		return "", fmt.Errorf("%w (known hosts: %s)", ErrHostnameUnavailable, strings.Join(index.names(), ", "))
	}
	if strings.TrimSpace(hostname) == "" {
		return "", newUnknownHostError("", index.names())
	}
	if canonical, ok := index.lookup(hostname); ok {
		return canonical, nil
	}
	return "", newUnknownHostError(hostname, index.names())
}

// hostIndex maps every normalized canonical name to its declared canonical name
// and preserves the deterministic sorted list of canonical names.
type hostIndex struct {
	owners map[string]string
	order  []string
}

// newHostIndex validates and indexes known hosts deterministically. Declarations
// are sorted by normalized name first so empty and duplicate diagnostics do not
// depend on input permutation. Empty canonical names and duplicate normalized
// canonical names are rejected fail-closed with a typed, privacy-safe error.
func newHostIndex(known []KnownHost) (*hostIndex, error) {
	declarations := make([]KnownHost, len(known))
	copy(declarations, known)
	sort.SliceStable(declarations, func(i, j int) bool {
		ni := normalizeHostName(declarations[i].Name)
		nj := normalizeHostName(declarations[j].Name)
		if ni != nj {
			return ni < nj
		}
		return declarations[i].Name < declarations[j].Name
	})

	index := &hostIndex{
		owners: make(map[string]string, len(declarations)),
		order:  make([]string, 0, len(declarations)),
	}
	for _, host := range declarations {
		normalized := normalizeHostName(host.Name)
		if normalized == "" {
			return nil, &InvalidKnownHostError{Name: host.Name, Reason: "empty canonical name"}
		}
		if _, exists := index.owners[normalized]; exists {
			return nil, &InvalidKnownHostError{Name: host.Name, Reason: "duplicate canonical name"}
		}
		index.owners[normalized] = host.Name
		index.order = append(index.order, host.Name)
	}
	return index, nil
}

func (i *hostIndex) lookup(value string) (string, bool) {
	canonical, ok := i.owners[normalizeHostName(value)]
	return canonical, ok
}

func (i *hostIndex) names() []string {
	out := make([]string, len(i.order))
	copy(out, i.order)
	return out
}

func newUnknownHostError(requested string, known []string) error {
	return &UnknownHostError{Requested: requested, Known: known}
}

func normalizeHostName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.TrimSuffix(value, ".")
	return value
}
