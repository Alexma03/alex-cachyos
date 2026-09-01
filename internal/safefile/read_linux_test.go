package safefile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadRegularFileUsesTrustedDescriptorAndReturnsBoundedMetadata(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(rootPath, 0700); err != nil {
		t.Fatal(err)
	}
	want := []byte("trusted bytes\x00\n")
	if err := os.WriteFile(filepath.Join(rootPath, "value"), want, 0600); err != nil {
		t.Fatal(err)
	}

	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	result, err := root.ReadRegularFileWithMetadata("value", int64(len(want)))
	if err != nil {
		t.Fatalf("read regular file: %v", err)
	}
	if !bytes.Equal(result.Data, want) || result.Metadata.Mode.Perm() != 0600 || result.Metadata.UID != uint32(os.Getuid()) || result.Metadata.GID != uint32(os.Getgid()) {
		t.Fatalf("result = %#v, want current-owned 0600 bytes", result)
	}
	if _, err := root.ReadRegularFile("value", int64(len(want)-1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("bounded read error = %v, want ErrTooLarge", err)
	}
}

func TestReadRegularFileRejectsUnsafeResolution(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "state")
	if err := os.Mkdir(rootPath, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(rootPath, "link")); err != nil {
		t.Fatal(err)
	}

	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	for _, path := range []string{"", "/etc/passwd", "../outside", "nested/../value", "link"} {
		if _, err := root.ReadRegularFile(path, 1024); err == nil {
			t.Fatalf("read %q unexpectedly succeeded", path)
		}
	}
}

func TestTrustedRootSurvivesPathReplacement(t *testing.T) {
	parent := t.TempDir()
	rootPath := filepath.Join(parent, "state")
	movedPath := filepath.Join(parent, "state-original")
	if err := os.Mkdir(rootPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "value"), []byte("trusted"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(rootPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, rootPath); err != nil {
		t.Fatal(err)
	}
	got, err := root.ReadRegularFile("value", 1024)
	if err != nil || !bytes.Equal(got, []byte("trusted")) {
		t.Fatalf("descriptor-relative read = %q, %v", got, err)
	}
}
