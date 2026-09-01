package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"

	"alex-cachyos/internal/runner"
)

func TestHiddenSelfHelperModeRequiresRootAndForwardsOnlyTypedInput(t *testing.T) {
	payload := []byte(`{"schema":"alex-cachyos.privileged-helper/v1","operation":"publish-files","files":[{"source":"/stage/file","destination":"/etc/managed","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mode":420}]}`)
	called := 0
	runtime := selfHelperRuntime{
		effectiveUID: func() int { return 0 },
		policy: func() (runner.SelfHelperPolicy, error) {
			return runner.SelfHelperPolicy{SourceUID: 1000, Destinations: map[string]fs.FileMode{}}, nil
		},
		execute: func(input io.Reader, _ runner.SelfHelperPolicy) error {
			called++
			got, _ := io.ReadAll(input)
			if !bytes.Equal(got, payload) {
				t.Fatalf("helper input = %q", got)
			}
			return nil
		},
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(payload), &stdout, &stderr, runtime); code != 0 || called != 1 || stdout.Len() != 0 {
		t.Fatalf("helper run = code %d called %d stdout %q stderr %q", code, called, stdout.String(), stderr.String())
	}
	runtime.effectiveUID = func() int { return 1000 }
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(payload), &stdout, &stderr, runtime); code == 0 || called != 1 {
		t.Fatalf("non-root helper run = code %d called %d", code, called)
	}
	if code := run([]string{runner.SelfHelperArgument, "unexpected"}, bytes.NewReader(payload), &stdout, &stderr, runtime); code == 0 || called != 1 {
		t.Fatalf("helper accepted extra argv = code %d called %d", code, called)
	}
}

func TestHiddenSelfHelperModeDoesNotExposeExecutionErrors(t *testing.T) {
	runtime := selfHelperRuntime{
		effectiveUID: func() int { return 0 },
		policy:       func() (runner.SelfHelperPolicy, error) { return runner.SelfHelperPolicy{}, nil },
		execute:      func(io.Reader, runner.SelfHelperPolicy) error { return errors.New("secret-content") },
	}
	var stderr bytes.Buffer
	if code := run([]string{runner.SelfHelperArgument}, bytes.NewReader(nil), io.Discard, &stderr, runtime); code == 0 || stderr.String() != "privileged helper failed\n" {
		t.Fatalf("helper error = code %d stderr %q", code, stderr.String())
	}
}
