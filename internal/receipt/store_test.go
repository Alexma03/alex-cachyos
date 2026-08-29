package receipt

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishCreatesReceiptAndCurrentIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	receipt := validStoreReceipt(t, "run-publish")

	path, err := store.Publish(receipt)
	if err != nil {
		t.Fatalf("publish receipt: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(root, "receipts") {
		t.Fatalf("receipt path = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("receipt mode = %o, want 600", info.Mode().Perm())
	}
	if info.Size() == 0 {
		t.Fatal("published receipt is empty")
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0700 {
		t.Fatalf("receipt directory mode = %o, want 700", info.Mode().Perm())
	}

	current, err := os.ReadFile(filepath.Join(root, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		RunID         string `json:"runId"`
		ReceiptPath   string `json:"receiptPath"`
		ReceiptSHA256 string `json:"receiptSha256"`
	}
	if err := json.Unmarshal(current, &index); err != nil {
		t.Fatalf("current index is not JSON: %v", err)
	}
	if index.RunID != receipt.RunID || index.ReceiptPath != filepath.Join("receipts", filepath.Base(path)) || index.ReceiptSHA256 == "" {
		t.Fatalf("current index = %#v", index)
	}
}

func validStoreReceipt(t *testing.T, id string) Receipt {
	t.Helper()
	r, err := Decode(goldenReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	r.RunID = id
	return r
}

func TestPublishRefusesExistingReceiptName(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	path, err := store.Publish(validStoreReceipt(t, "run-existing"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(validStoreReceipt(t, "run-existing")); !errors.Is(err, ErrReceiptExists) {
		t.Fatalf("second publish error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("existing receipt was changed")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("receipt entries = %d, %v", len(entries), err)
	}
}

func TestCurrentIndexRenameIsAtomic(t *testing.T) {
	root := filepath.Join(t.TempDir(), "alex-cachyos")
	store := NewStore(root)
	oldPath, err := store.Publish(validStoreReceipt(t, "run-old"))
	if err != nil {
		t.Fatal(err)
	}
	ops := defaultFileOps()
	started, release := make(chan struct{}), make(chan struct{})
	ops.rename = func(from, to string) error {
		if filepath.Base(to) == currentName {
			close(started)
			<-release
		}
		return os.Rename(from, to)
	}
	store = newStoreWithOps(root, ops)
	newReceipt := validStoreReceipt(t, "run-new")
	done := make(chan error, 1)
	go func() { _, err := store.Publish(newReceipt); done <- err }()
	<-started
	for i := 0; i < 100; i++ {
		data, err := os.ReadFile(filepath.Join(root, currentName))
		if err != nil {
			t.Fatal(err)
		}
		var index currentIndex
		if err := json.Unmarshal(data, &index); err != nil {
			t.Fatalf("partial current index: %v", err)
		}
		old := filepath.Join(receiptsDirName, filepath.Base(oldPath))
		if index.ReceiptPath != old {
			t.Fatalf("index changed before rename: %q", index.ReceiptPath)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
