package runner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelfHelperStagesElevatesAndPublishesAllowlistedFile(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	destination := filepath.Join(root, "etc", "managed.conf")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}

	file, err := StageSelfHelperFile(stage, []byte("managed\n"), destination, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	request := SelfHelperRequest{Schema: SelfHelperSchema, Operation: SelfHelperPublishFiles, Files: []SelfHelperFile{file}}
	command, err := BuildSelfHelperCommand(os.Args[0], request)
	if err != nil {
		t.Fatal(err)
	}
	if command.Scope != ScopeSystem || command.Network != NetworkNone || command.Executable != os.Args[0] ||
		!reflect.DeepEqual(command.Argv, []string{SelfHelperArgument}) || command.Shell {
		t.Fatalf("self-helper command = %#v", command)
	}

	delegate := NewFakeRunner(Expectation{Operation: SelfHelperCommandOperation, Argv: []string{os.Args[0], SelfHelperArgument}})
	if _, err := NewElevationRunner(delegate).Run(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	gotRequests := delegate.Requests()
	if len(gotRequests) != 1 || gotRequests[0].Executable != pkexecExecutable || !bytes.Equal(gotRequests[0].Stdin, command.Stdin) {
		t.Fatalf("elevated helper request = %#v", gotRequests)
	}

	policy := SelfHelperPolicy{SourceUID: uint32(os.Getuid()), Destinations: map[string]os.FileMode{destination: 0o640}}
	if err := ExecuteSelfHelper(request, policy); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "managed\n" || info.Mode().Perm() != 0o640 {
		t.Fatalf("published content/mode = %q %04o", got, info.Mode().Perm())
	}
}

func TestSelfHelperRejectsUntrustedInputsBeforeReplacingTargets(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	destination := filepath.Join(root, "etc", "managed.conf")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid, err := StageSelfHelperFile(stage, []byte("after\n"), destination, 0o640)
	if err != nil {
		t.Fatal(err)
	}
	policy := SelfHelperPolicy{SourceUID: uint32(os.Getuid()), Destinations: map[string]os.FileMode{destination: 0o640}}

	tests := []struct {
		name string
		edit func(*SelfHelperRequest) error
	}{
		{"unknown operation", func(r *SelfHelperRequest) error { r.Operation = "shell"; return nil }},
		{"destination outside allowlist", func(r *SelfHelperRequest) error { r.Files[0].Destination = filepath.Join(root, "other"); return nil }},
		{"mode mismatch", func(r *SelfHelperRequest) error { r.Files[0].Mode = 0o666; return nil }},
		{"hash mismatch", func(r *SelfHelperRequest) error {
			r.Files[0].SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
			return nil
		}},
		{"symlink source", func(r *SelfHelperRequest) error {
			link := filepath.Join(stage, "link")
			if err := os.Symlink(r.Files[0].Source, link); err != nil {
				return err
			}
			r.Files[0].Source = link
			return nil
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := SelfHelperRequest{Schema: SelfHelperSchema, Operation: SelfHelperPublishFiles, Files: []SelfHelperFile{valid}}
			if err := tc.edit(&request); err != nil {
				t.Fatal(err)
			}
			if err := ExecuteSelfHelper(request, policy); err == nil {
				t.Fatal("unsafe helper request accepted")
			}
			got, err := os.ReadFile(destination)
			if err != nil || string(got) != "before\n" {
				t.Fatalf("target changed after rejection: %q, %v", got, err)
			}
		})
	}
}

func TestSelfHelperValidatesWholeBatchBeforeAnyAtomicRename(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	parent := filepath.Join(root, "etc")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	destinationA := filepath.Join(parent, "a")
	destinationB := filepath.Join(parent, "b")
	if err := os.WriteFile(destinationA, []byte("old-a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationB, []byte("old-b"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := StageSelfHelperFile(stage, []byte("new-a"), destinationA, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	b, err := StageSelfHelperFile(stage, []byte("new-b"), destinationB, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	b.SHA256 = hex.EncodeToString(make([]byte, sha256.Size))
	policy := SelfHelperPolicy{SourceUID: uint32(os.Getuid()), Destinations: map[string]os.FileMode{destinationA: 0o600, destinationB: 0o600}}
	request := SelfHelperRequest{Schema: SelfHelperSchema, Operation: SelfHelperPublishFiles, Files: []SelfHelperFile{a, b}}
	if err := ExecuteSelfHelper(request, policy); err == nil {
		t.Fatal("invalid batch accepted")
	}
	for path, want := range map[string]string{destinationA: "old-a", destinationB: "old-b"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, err)
		}
	}
}

func TestSelfHelperUsesOpenedSourceWhenPathIsSwapped(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	parent := filepath.Join(root, "etc")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "managed")
	file, err := StageSelfHelperFile(stage, []byte("trusted"), destination, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(stage, "replacement")
	if err := os.WriteFile(replacement, []byte("attacker"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := SelfHelperRequest{Schema: SelfHelperSchema, Operation: SelfHelperPublishFiles, Files: []SelfHelperFile{file}}
	policy := SelfHelperPolicy{SourceUID: uint32(os.Getuid()), Destinations: map[string]os.FileMode{destination: 0o600}}
	err = executeSelfHelper(request, policy, selfHelperHooks{afterSourcesOpened: func() {
		if err := os.Rename(replacement, file.Source); err != nil {
			t.Fatal(err)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "trusted" {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

func TestDecodeSelfHelperRequestRejectsTrailingOrOversizedInput(t *testing.T) {
	valid := []byte(`{"schema":"alex-cachyos.privileged-helper/v1","operation":"publish-files","files":[]}`)
	if _, err := DecodeSelfHelperRequest(bytes.NewReader(append(valid, []byte("{}")...))); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if _, err := DecodeSelfHelperRequest(bytes.NewReader(make([]byte, MaxSelfHelperRequestBytes+1))); !errors.Is(err, ErrInvalidSelfHelperRequest) {
		t.Fatalf("oversized request error = %v", err)
	}
}
