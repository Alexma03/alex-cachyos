package receipt

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentReadsVerifiedReceiptThroughTrustedDescriptors(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	want := validStoreReceipt(t, "run-current")
	wantPath, err := store.Publish(want)
	if err != nil {
		t.Fatal(err)
	}
	got, gotPath, err := store.Current()
	if err != nil || gotPath != wantPath || got.RunID != want.RunID {
		t.Fatalf("Current = run %q path %q err %v", got.RunID, gotPath, err)
	}
}

func TestCurrentRejectsUnsafeModesAndSymlinks(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(root, receiptPath string) error
	}{
		{name: "state directory mode", mutate: func(root, _ string) error { return os.Chmod(root, 0755) }},
		{name: "receipts directory mode", mutate: func(root, _ string) error { return os.Chmod(filepath.Join(root, receiptsDirName), 0755) }},
		{name: "current mode", mutate: func(root, _ string) error { return os.Chmod(filepath.Join(root, currentName), 0644) }},
		{name: "receipt mode", mutate: func(_ string, receiptPath string) error { return os.Chmod(receiptPath, 0644) }},
		{name: "current symlink", mutate: func(root, _ string) error {
			path := filepath.Join(root, currentName)
			if err := os.Rename(path, path+".real"); err != nil {
				return err
			}
			return os.Symlink(path+".real", path)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "alex-cachyos")
			store := NewStore(root)
			path, err := store.Publish(validStoreReceipt(t, "run-secure"))
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.mutate(root, path); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Current(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Current error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCurrentRejectsOversizedIndexWithoutParsing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	if _, err := store.Publish(validStoreReceipt(t, "run-large")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, currentName), make([]byte, maxReceiptRead+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Current(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Current error = %v, want ErrInvalid", err)
	}
}

func TestCurrentRejectsEscapingPathDigestAndRunID(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*currentIndex)
	}{
		{name: "traversal", mutate: func(index *currentIndex) { index.ReceiptPath = "receipts/../outside.json" }},
		{name: "nested receipt", mutate: func(index *currentIndex) { index.ReceiptPath = "receipts/nested/receipt.json" }},
		{name: "digest mismatch", mutate: func(index *currentIndex) { index.ReceiptSHA256 = string(make([]byte, sha256.Size*2)) }},
		{name: "run ID mismatch", mutate: func(index *currentIndex) { index.RunID = "other-run" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "alex-cachyos")
			store := NewStore(root)
			if _, err := store.Publish(validStoreReceipt(t, "run-bound")); err != nil {
				t.Fatal(err)
			}
			indexPath := filepath.Join(root, currentName)
			data, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			var index currentIndex
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&index)
			data, err = json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(indexPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Current(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Current error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestCurrentIsReadOnlyAndBypassesPathOperationHooks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	want := validStoreReceipt(t, "run-readonly")
	if _, err := store.Publish(want); err != nil {
		t.Fatal(err)
	}
	writes := 0
	ops := defaultFileOps()
	ops.mkdirAll = func(string, os.FileMode) error { writes++; return errors.New("unexpected write") }
	ops.chmod = func(string, os.FileMode) error { writes++; return errors.New("unexpected write") }
	ops.createTemp = func(string, string) (*os.File, error) { writes++; return nil, errors.New("unexpected write") }
	ops.open = func(string) (*os.File, error) { writes++; return nil, errors.New("unexpected path open") }
	ops.sync = func(*os.File) error { writes++; return errors.New("unexpected write") }
	ops.remove = func(string) error { writes++; return errors.New("unexpected write") }
	ops.link = func(string, string) error { writes++; return errors.New("unexpected write") }
	ops.rename = func(string, string) error { writes++; return errors.New("unexpected write") }
	got, _, err := newStoreWithOps(root, ops).Current()
	if err != nil || got.RunID != want.RunID || writes != 0 {
		t.Fatalf("Current = run %q writes=%d err=%v", got.RunID, writes, err)
	}
}
