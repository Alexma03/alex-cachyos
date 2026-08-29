package receipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alex-cachyos/internal/statepath"
)

var errInjected = errors.New("injected receipt store failure")

func TestPublishFileSyncFailureLeavesNoReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	ops := defaultFileOps()
	ops.sync = func(file *os.File) error {
		if strings.HasPrefix(filepath.Base(file.Name()), ".receipt-") {
			return errInjected
		}
		return file.Sync()
	}

	path, err := newStoreWithOps(root, ops).Publish(validStoreReceipt(t, "run-sync-failure"))
	if !errors.Is(err, errInjected) {
		t.Fatalf("publish error = %v, want injected error", err)
	}
	if path != "" {
		t.Fatalf("published path = %q on failure", path)
	}
	assertNoPublishedReceipt(t, root)
}

func TestPublishLinkFailureDoesNotOverwriteExistingReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	receipt := validStoreReceipt(t, "run-link-failure")
	oldPath, err := NewStore(root).Publish(receipt)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	ops := defaultFileOps()
	ops.link = func(from, to string) error {
		if to != oldPath {
			t.Errorf("link destination = %q, want %q", to, oldPath)
		}
		return &os.LinkError{Op: "link", Old: from, New: to, Err: os.ErrExist}
	}
	path, err := newStoreWithOps(root, ops).Publish(receipt)
	if !errors.Is(err, ErrReceiptExists) {
		t.Fatalf("publish error = %v, want ErrReceiptExists", err)
	}
	if path != "" {
		t.Fatalf("published path = %q on failure", path)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("existing receipt was overwritten")
	}
	assertReceiptsReadable(t, root)
}

func TestPublishRenameFailureLeavesCurrentUnchanged(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	oldPath, err := NewStore(root).Publish(validStoreReceipt(t, "run-old"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCurrent := readCurrent(t, root)
	beforeReceipt, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	ops := defaultFileOps()
	ops.rename = func(from, to string) error {
		if filepath.Base(to) == currentName {
			return errInjected
		}
		return os.Rename(from, to)
	}
	path, err := newStoreWithOps(root, ops).Publish(validStoreReceipt(t, "run-new"))
	if !errors.Is(err, errInjected) {
		t.Fatalf("publish error = %v, want injected error", err)
	}
	if path != "" {
		t.Fatalf("published path = %q on failure", path)
	}
	if got := readCurrent(t, root); !bytes.Equal(got, beforeCurrent) {
		t.Fatal("current index claimed a receipt after rename failure")
	}
	afterReceipt, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterReceipt, beforeReceipt) {
		t.Fatal("existing receipt changed after rename failure")
	}
	assertReceiptsReadable(t, root)
}

func TestPublishDirectorySyncFailureDoesNotClaimReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	oldPath, err := NewStore(root).Publish(validStoreReceipt(t, "run-old"))
	if err != nil {
		t.Fatal(err)
	}
	beforeCurrent := readCurrent(t, root)
	beforeReceipt, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}

	ops := defaultFileOps()
	ops.sync = func(file *os.File) error {
		if filepath.Clean(file.Name()) == filepath.Clean(root) {
			return errInjected
		}
		return file.Sync()
	}
	path, err := newStoreWithOps(root, ops).Publish(validStoreReceipt(t, "run-new"))
	if !errors.Is(err, errInjected) {
		t.Fatalf("publish error = %v, want injected error", err)
	}
	if path != "" {
		t.Fatalf("published path = %q on failure", path)
	}
	if got := readCurrent(t, root); !bytes.Equal(got, beforeCurrent) {
		t.Fatal("current index claimed a receipt after directory sync failure")
	}
	afterReceipt, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterReceipt, beforeReceipt) {
		t.Fatal("existing receipt changed after directory sync failure")
	}
	assertReceiptsReadable(t, root)
}

func TestPublishCrashWindowAfterFsyncBeforeLinkHasNoFinalReceipt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	started, release := make(chan struct{}), make(chan struct{})
	ops := defaultFileOps()
	ops.sync = func(file *os.File) error {
		if strings.HasPrefix(filepath.Base(file.Name()), ".receipt-") {
			if err := file.Sync(); err != nil {
				return err
			}
			entries, err := os.ReadDir(filepath.Dir(file.Name()))
			if err != nil {
				t.Errorf("read receipt directory during crash window: %v", err)
			} else {
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".json") {
						t.Errorf("final receipt visible before link: %s", entry.Name())
					}
				}
			}
			close(started)
			<-release
		}
		return nil
	}
	store := newStoreWithOps(root, ops)
	done := make(chan struct{})
	var path string
	var publishErr error
	go func() {
		path, publishErr = store.Publish(validStoreReceipt(t, "run-crash-window"))
		close(done)
	}()
	<-started
	if _, err := os.Stat(filepath.Join(root, currentName)); !os.IsNotExist(err) {
		t.Fatalf("current receipt appeared during crash window: %v", err)
	}
	close(release)
	<-done
	if publishErr != nil {
		t.Fatalf("publish receipt: %v", publishErr)
	}
	if path == "" {
		t.Fatal("publish returned an empty path")
	}
	assertReceiptsReadable(t, root)
}

func TestFailedCommandReceiptRemainsPublishable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	receipt := validStoreReceipt(t, "run-failed")
	receipt.Status = "failed"
	path, err := NewStore(root).Publish(receipt)
	if err != nil {
		t.Fatalf("publish failed command receipt: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("decode failed command receipt: %v", err)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestNewStoreFromPathsUsesDefaultAndOverriddenStateRoots(t *testing.T) {
	home := t.TempDir()
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	for _, tc := range []struct {
		name, state, want string
	}{
		{"default", "", filepath.Join(home, ".local", "state")},
		{"override", filepath.Join(home, "custom-state"), filepath.Join(home, "custom-state")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", tc.state)
			paths, err := statepath.Resolve()
			if err != nil {
				t.Fatal(err)
			}
			path, err := NewStoreFromPaths(paths).Publish(validStoreReceipt(t, "run-"+tc.name))
			if err != nil {
				t.Fatal(err)
			}
			wantDir := filepath.Join(tc.want, receiptStateDir, receiptsDirName)
			if filepath.Dir(path) != wantDir {
				t.Fatalf("receipt directory = %q, want %q", filepath.Dir(path), wantDir)
			}
			if _, err := os.Stat(filepath.Join(home, ".pi")); !os.IsNotExist(err) {
				t.Fatalf("test touched Pi state: %v", err)
			}
		})
	}
}

func readCurrent(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, currentName))
	if err != nil {
		t.Fatal(err)
	}
	var index currentIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("current index is not complete JSON: %v", err)
	}
	return data
}

func assertNoPublishedReceipt(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, receiptsDirName))
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("readable receipt left after failure: %s", entry.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(root, currentName)); !os.IsNotExist(err) {
		t.Fatalf("current index left after failure: %v", err)
	}
}

func assertReceiptsReadable(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, receiptsDirName))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			if strings.HasPrefix(entry.Name(), ".receipt-") || strings.HasPrefix(entry.Name(), ".current-") {
				t.Fatalf("temporary receipt file left behind: %s", entry.Name())
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, receiptsDirName, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Decode(data); err != nil {
			t.Fatalf("receipt %s is not readable: %v", entry.Name(), err)
		}
	}
}
