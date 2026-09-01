package adopt

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestAdoptRejectsTargetReplacementBeforeBackupOpen(t *testing.T) {
	for _, tc := range []struct {
		name    string
		replace func(target, held string) error
	}{
		{name: "regular", replace: func(target, held string) error {
			if err := os.Rename(target, held); err != nil {
				return err
			}
			return os.WriteFile(target, []byte("replacement"), 0600)
		}},
		{name: "symlink", replace: func(target, held string) error {
			if err := os.Rename(target, held); err != nil {
				return err
			}
			return os.Symlink(held, target)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := t.TempDir()
			dir := t.TempDir()
			target, held := filepath.Join(dir, "settings"), filepath.Join(dir, "held")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			ops := defaultFileOps()
			replaced := false
			ops.openFile = func(path string, flag int, mode os.FileMode) (*os.File, error) {
				if path == target && !replaced {
					replaced = true
					if err := tc.replace(target, held); err != nil {
						return nil, err
					}
				}
				return os.OpenFile(path, flag, mode)
			}
			if _, err := newStoreWithOps(state, ops).Adopt(target, "receipt-swap"); !errors.Is(err, ErrInvalidTarget) {
				t.Fatalf("Adopt error = %v, want ErrInvalidTarget", err)
			}
			if _, err := os.Lstat(target + BackupSuffix); !os.IsNotExist(err) {
				t.Fatalf("backup exists after rejected swap: %v", err)
			}
		})
	}
}

func TestAdoptPreservesOwnerAndUsesDescriptorForBackupMode(t *testing.T) {
	state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(target, []byte("original"), 0640); err != nil {
		t.Fatal(err)
	}
	ops := defaultFileOps()
	pathChmod := false
	baseChmod := ops.chmod
	ops.chmod = func(path string, mode os.FileMode) error {
		if path == target+BackupSuffix {
			pathChmod = true
		}
		return baseChmod(path, mode)
	}
	record, err := newStoreWithOps(state, ops).Adopt(target, "receipt-owner")
	if err != nil {
		t.Fatal(err)
	}
	if pathChmod {
		t.Fatal("backup mode was changed through a reopenable pathname")
	}
	if record.OriginalUID != uint32(os.Getuid()) || record.OriginalGID != uint32(os.Getgid()) {
		t.Fatalf("record ownership = %d:%d", record.OriginalUID, record.OriginalGID)
	}
	info, err := os.Stat(record.Backup)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, ok := fileOwner(info)
	if !ok || uid != record.OriginalUID || gid != record.OriginalGID || info.Mode().Perm() != record.OriginalMode {
		t.Fatalf("backup metadata = uid:%d gid:%d mode:%o", uid, gid, info.Mode().Perm())
	}
}

func TestLookupUsesDescriptorRelativeBoundedReadsAndValidatesModes(t *testing.T) {
	state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(target, []byte("target"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(state)
	want, err := store.Adopt(target, "receipt-lookup")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Lookup(target)
	if err != nil || !found || got != want {
		t.Fatalf("Lookup = %#v, %v, %v", got, found, err)
	}

	if err := os.Chmod(store.recordPath(target), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Lookup(target); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("unsafe record mode error = %v", err)
	}
	if err := os.Chmod(store.recordPath(target), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.recordPath(target), make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Lookup(target); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("oversized record error = %v", err)
	}
}

func TestLookupRejectsCorruptRecordAndUnsafeBackup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Store, string, Record) error
	}{
		{name: "record trailing data", mutate: func(store *Store, target string, record Record) error {
			data, err := json.Marshal(record)
			if err != nil {
				return err
			}
			return os.WriteFile(store.recordPath(target), append(data, []byte(`{}`)...), 0600)
		}},
		{name: "backup symlink", mutate: func(_ *Store, _ string, record Record) error {
			held := record.Backup + ".held"
			if err := os.Rename(record.Backup, held); err != nil {
				return err
			}
			return os.Symlink(held, record.Backup)
		}},
		{name: "backup changed", mutate: func(_ *Store, _ string, record Record) error {
			return os.WriteFile(record.Backup, []byte("changed"), record.OriginalMode)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewStore(state)
			record, err := store.Adopt(target, "receipt-invalid")
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.mutate(store, target, record); err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.Lookup(target); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("Lookup error = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

func TestLookupIsReadOnlyAndDoesNotUsePathBasedReaders(t *testing.T) {
	state, target := t.TempDir(), filepath.Join(t.TempDir(), "settings")
	if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	want, err := NewStore(state).Adopt(target, "receipt-readonly")
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	ops := defaultFileOps()
	ops.readFile = func(string) ([]byte, error) { return nil, errors.New("unexpected path read") }
	ops.openFile = func(string, int, os.FileMode) (*os.File, error) { writes++; return nil, errors.New("unexpected write") }
	ops.createTemp = func(string, string) (*os.File, error) { writes++; return nil, errors.New("unexpected write") }
	ops.mkdirAll = func(string, os.FileMode) error { writes++; return errors.New("unexpected write") }
	ops.chmod = func(string, os.FileMode) error { writes++; return errors.New("unexpected write") }
	ops.link = func(string, string) error { writes++; return errors.New("unexpected write") }
	record, found, err := newStoreWithOps(state, ops).Lookup(target)
	if err != nil || !found || record != want || writes != 0 {
		t.Fatalf("Lookup = %#v, found=%v, writes=%d, err=%v", record, found, writes, err)
	}
}

func TestWebSearchBoundaryNeverReadsOrBacksUpUnmanagedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-search.json")
	secret := []byte(`{"apiKey":"literal-secret"}`)
	if err := os.WriteFile(path, secret, 0600); err != nil {
		t.Fatal(err)
	}
	reads := 0
	ops := defaultWebSearchOps()
	ops.readOwned = func(string, string, int64) ([]byte, os.FileMode, error) {
		reads++
		return nil, 0, errors.New("unexpected read")
	}
	got, err := observeWebSearchWithOps(path, OwnershipUnmanaged, fmt.Sprintf("%x", sha256.Sum256([]byte("expected"))), ops)
	if !errors.Is(err, ErrUnmanagedWebSearch) || !got.Exists || !got.Blocked || reads != 0 {
		t.Fatalf("observation = %#v, reads=%d, err=%v", got, reads, err)
	}
	if _, err := os.Lstat(path + BackupSuffix); !os.IsNotExist(err) {
		t.Fatalf("unmanaged secret file was backed up: %v", err)
	}
}

func TestWebSearchBoundaryHashesOnlyConfiguratorCreatedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web-search.json")
	content := []byte(`{"providers":{"openai":{"apiKey":"$OPENAI_API_KEY"}}}`)
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	got, err := ObserveWebSearch(path, OwnershipCreated, want)
	if err != nil || !got.Exists || got.Blocked || !got.MatchesExpected || got.Mode != 0600 {
		t.Fatalf("observation = %#v, err=%v", got, err)
	}
}
