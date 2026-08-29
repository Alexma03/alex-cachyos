package adopt

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAdoptCreatesOneTimeBackupAndDurableRecord(t *testing.T) {
	state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
	content, mode := []byte("user settings\x00\n"), os.FileMode(0640)
	if err := os.WriteFile(target, content, mode); err != nil { t.Fatal(err) }
	syncs := 0
	ops := defaultFileOps()
	ops.sync = func(f *os.File) error { syncs++; return f.Sync() }
	store := newStoreWithOps(state, ops)

	got, err := store.Adopt(target, "receipt-1")
	if err != nil {
		t.Fatal(err)
	}
	wantHash := fmt.Sprintf("%x", sha256.Sum256(content))
	if got.Schema != SchemaV1 || got.Target != target || got.Backup != target+BackupSuffix || got.OriginalSHA256 != wantHash || got.OriginalMode != mode || got.FirstReceiptID != "receipt-1" {
		t.Fatalf("record = %#v", got)
	}
	backup, err := os.ReadFile(got.Backup)
	if err != nil || !bytes.Equal(backup, content) {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	info, err := os.Stat(got.Backup)
	if err != nil || info.Mode().Perm() != mode {
		t.Fatalf("backup mode = %v, %v", info.Mode(), err)
	}
	decoded, err := store.Load(target)
	if err != nil || decoded != got {
		t.Fatalf("loaded record = %#v, %v", decoded, err)
	}
	if syncs < 3 {
		t.Fatalf("sync calls = %d, want backup, parent, and record durability", syncs)
	}

	again, err := store.Adopt(target, "receipt-2")
	if err != nil || again != got {
		t.Fatalf("second adoption = %#v, %v", again, err)
	}
}

func TestAdoptBlocksUnknownBackupWithoutChangingIt(t *testing.T) {
	state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
	unknown := []byte("not ours")
	if err := os.WriteFile(target, []byte("target"), 0600); err != nil { t.Fatal(err) }
	backup := target + BackupSuffix
	if err := os.WriteFile(backup, unknown, 0600); err != nil { t.Fatal(err) }
	if _, err := NewStore(state).Adopt(target, "receipt-1"); !errors.Is(err, ErrBackupConflict) {
		t.Fatalf("adoption error = %v, want backup conflict", err)
	}
	if got, err := os.ReadFile(backup); err != nil || !bytes.Equal(got, unknown) {
		t.Fatalf("unknown backup changed: %q, %v", got, err)
	}
}

func TestAdoptRejectsUnsafeAndNonRegularTargets(t *testing.T) {
	state := t.TempDir()
	if _, err := NewStore("relative-state").Adopt(filepath.Join(t.TempDir(), "target"), "r"); !errors.Is(err, ErrUnsafeStateRoot) {
		t.Fatalf("relative state error = %v", err)
	}
	dir := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(dir, 0700); err != nil { t.Fatal(err) }
	if _, err := NewStore(state).Adopt(dir, "r"); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("directory target error = %v", err)
	}
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("target"), 0600); err != nil { t.Fatal(err) }
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil { t.Fatal(err) }
	if _, err := NewStore(state).Adopt(link, "r"); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("symlink target error = %v", err)
	}
}
