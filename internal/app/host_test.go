package app

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeHostPort is a deterministic HostPort double that records every call so
// tests can prove precedence and single-call behavior.
type fakeHostPort struct {
	hostname string
	err      error
	calls    int
}

func (f *fakeHostPort) Hostname() (string, error) {
	f.calls++
	return f.hostname, f.err
}

func knownHosts() []KnownHost {
	return []KnownHost{{Name: "galaxy"}, {Name: "alpha"}}
}

func TestResolveHostExplicitTakesPrecedenceWithoutCallingPort(t *testing.T) {
	port := &fakeHostPort{hostname: "alpha"}
	got, err := ResolveHost("galaxy", knownHosts(), port)
	if err != nil {
		t.Fatal(err)
	}
	if got != "galaxy" {
		t.Fatalf("resolved host = %q, want galaxy", got)
	}
	if port.calls != 0 {
		t.Fatalf("Hostname called %d times, want 0 (explicit host must not call the port)", port.calls)
	}
}

func TestResolveHostMatchesCanonicalNameExactly(t *testing.T) {
	for name, hostname := range map[string]string{
		"canonical":          "galaxy",
		"case-insensitive":   "Galaxy",
		"trailing dot":       "galaxy.",
		"surrounding spaces": " galaxy ",
	} {
		t.Run(name, func(t *testing.T) {
			port := &fakeHostPort{hostname: hostname}
			got, err := ResolveHost("", knownHosts(), port)
			if err != nil {
				t.Fatal(err)
			}
			if got != "galaxy" {
				t.Fatalf("resolved host = %q, want galaxy", got)
			}
			if port.calls != 1 {
				t.Fatalf("Hostname called %d times, want exactly 1", port.calls)
			}
		})
	}
}

func TestResolveHostRejectsUnknownExplicitHost(t *testing.T) {
	port := &fakeHostPort{hostname: "galaxy"}
	_, err := ResolveHost("unknown", knownHosts(), port)
	if err == nil {
		t.Fatal("ResolveHost accepted an unknown explicit host")
	}
	if !errors.Is(err, ErrUnknownHost) {
		t.Fatalf("error = %v, want ErrUnknownHost", err)
	}
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want UnknownHostError", err)
	}
	if unknown.Requested != "unknown" {
		t.Fatalf("Requested = %q, want %q", unknown.Requested, "unknown")
	}
	if !reflect.DeepEqual(unknown.Known, []string{"alpha", "galaxy"}) {
		t.Fatalf("Known = %v, want sorted [alpha galaxy]", unknown.Known)
	}
	if port.calls != 0 {
		t.Fatalf("Hostname called %d times, want 0", port.calls)
	}
}

func TestResolveHostRejectsUnknownHostname(t *testing.T) {
	port := &fakeHostPort{hostname: "unknown-machine"}
	_, err := ResolveHost("", knownHosts(), port)
	if err == nil {
		t.Fatal("ResolveHost accepted an unknown hostname")
	}
	if !errors.Is(err, ErrUnknownHost) {
		t.Fatalf("error = %v, want ErrUnknownHost", err)
	}
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want UnknownHostError", err)
	}
	if !reflect.DeepEqual(unknown.Known, []string{"alpha", "galaxy"}) {
		t.Fatalf("Known = %v, want sorted [alpha galaxy]", unknown.Known)
	}
	if port.calls != 1 {
		t.Fatalf("Hostname called %d times, want exactly 1", port.calls)
	}
}

func TestResolveHostRejectsEmptyHostname(t *testing.T) {
	for _, hostname := range []string{"", "   "} {
		port := &fakeHostPort{hostname: hostname}
		_, err := ResolveHost("", knownHosts(), port)
		if err == nil {
			t.Fatalf("ResolveHost accepted empty hostname %q", hostname)
		}
		if !errors.Is(err, ErrUnknownHost) {
			t.Fatalf("hostname %q: error = %v, want ErrUnknownHost", hostname, err)
		}
		var unknown *UnknownHostError
		if !errors.As(err, &unknown) {
			t.Fatalf("hostname %q: error = %v, want UnknownHostError", hostname, err)
		}
		if !reflect.DeepEqual(unknown.Known, []string{"alpha", "galaxy"}) {
			t.Fatalf("hostname %q: Known = %v, want sorted [alpha galaxy]", hostname, unknown.Known)
		}
		if port.calls != 1 {
			t.Fatalf("hostname %q: Hostname called %d times, want exactly 1", hostname, port.calls)
		}
	}
}

func TestResolveHostPortErrorIsPrivateAndKeepsKnownHosts(t *testing.T) {
	port := &fakeHostPort{err: errors.New("secret internal port detail")}
	_, err := ResolveHost("", knownHosts(), port)
	if err == nil {
		t.Fatal("ResolveHost accepted a failing port")
	}
	if !errors.Is(err, ErrHostnameUnavailable) {
		t.Fatalf("error = %v, want ErrHostnameUnavailable", err)
	}
	if strings.Contains(err.Error(), "secret internal port detail") {
		t.Fatalf("error %q leaks the port's internal error", err)
	}
	if !strings.Contains(err.Error(), "alpha") || !strings.Contains(err.Error(), "galaxy") {
		t.Fatalf("error %q loses known hosts", err)
	}
	if port.calls != 1 {
		t.Fatalf("Hostname called %d times, want exactly 1", port.calls)
	}
}

func TestResolveHostListsKnownHostsSorted(t *testing.T) {
	known := []KnownHost{
		{Name: "zeta"},
		{Name: "Alpha"},
		{Name: "galaxy"},
	}
	port := &fakeHostPort{hostname: "missing"}
	_, err := ResolveHost("", known, port)
	if err == nil {
		t.Fatal("ResolveHost accepted an unknown hostname")
	}
	var unknown *UnknownHostError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want UnknownHostError", err)
	}
	if want := []string{"Alpha", "galaxy", "zeta"}; !reflect.DeepEqual(unknown.Known, want) {
		t.Fatalf("Known = %v, want %v", unknown.Known, want)
	}
}

func TestResolveHostRejectsEmptyCanonicalName(t *testing.T) {
	port := &fakeHostPort{hostname: "galaxy"}
	_, err := ResolveHost("", []KnownHost{{Name: ""}, {Name: "galaxy"}}, port)
	if !errors.Is(err, ErrInvalidKnownHost) {
		t.Fatalf("error = %v, want ErrInvalidKnownHost", err)
	}
	var invalid *InvalidKnownHostError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want InvalidKnownHostError", err)
	}
	if port.calls != 0 {
		t.Fatalf("Hostname called %d times, want 0 (fail-closed before lookup)", port.calls)
	}
}

func TestResolveHostRejectsDuplicateNormalizedCanonicalDeterministically(t *testing.T) {
	var want string
	for i, known := range [][]KnownHost{
		{{Name: "galaxy"}, {Name: "GALAXY."}},
		{{Name: "GALAXY."}, {Name: "galaxy"}},
	} {
		port := &fakeHostPort{hostname: "galaxy"}
		_, err := ResolveHost("", known, port)
		if !errors.Is(err, ErrInvalidKnownHost) {
			t.Fatalf("order %d: error = %v, want ErrInvalidKnownHost", i, err)
		}
		var invalid *InvalidKnownHostError
		if !errors.As(err, &invalid) {
			t.Fatalf("order %d: error = %v, want InvalidKnownHostError", i, err)
		}
		if port.calls != 0 {
			t.Fatalf("order %d: Hostname called %d times, want 0", i, port.calls)
		}
		if i == 0 {
			want = invalid.Error()
		} else if invalid.Error() != want {
			t.Fatalf("reversed order diagnostic = %q, want %q", invalid.Error(), want)
		}
	}
}

func TestHostPortExposesOnlyHostname(t *testing.T) {
	iface := reflect.TypeOf((*HostPort)(nil)).Elem()
	if iface.NumMethod() != 1 {
		t.Fatalf("HostPort has %d methods, want exactly 1 (hostname only)", iface.NumMethod())
	}
	if name := iface.Method(0).Name; name != "Hostname" {
		t.Fatalf("HostPort method = %q, want %q", name, "Hostname")
	}
}
