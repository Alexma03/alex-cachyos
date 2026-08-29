// Package adopt owns one-time backups and their immutable ownership records.
package adopt

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SchemaV1    = "alex-cachyos.adoption/v1"
	Schema      = SchemaV1
	BackupSuffix = ".bak.alex-cachyos"
)

var (
	ErrInvalidTarget   = errors.New("invalid adoption target")
	ErrTargetMissing   = errors.New("adoption target does not exist")
	ErrUnsafeStateRoot = errors.New("unsafe adoption state root")
	ErrBackupConflict  = errors.New("backup exists without a matching adoption record")
	ErrInvalidRecord   = errors.New("invalid adoption record")
	ErrRecordNotFound  = errors.New("adoption record not found")
)

// Record is the immutable relationship between an adopted target and its backup.
type Record struct {
	Schema         string      `json:"schema"`
	Target         string      `json:"target"`
	Backup         string      `json:"backup"`
	OriginalSHA256 string      `json:"originalSha256"`
	OriginalMode   os.FileMode `json:"originalMode"`
	FirstReceiptID string      `json:"firstReceiptId"`
}

type fileOps struct {
	lstat      func(string) (os.FileInfo, error)
	openFile   func(string, int, os.FileMode) (*os.File, error)
	readFile   func(string) ([]byte, error)
	createTemp func(string, string) (*os.File, error)
	mkdirAll   func(string, os.FileMode) error
	chmod      func(string, os.FileMode) error
	open       func(string) (*os.File, error)
	sync       func(*os.File) error
	close      func(*os.File) error
	remove     func(string) error
	link       func(string, string) error
}

type Store struct {
	root string
	ops  fileOps
}

func defaultFileOps() fileOps {
	return fileOps{
		lstat: os.Lstat, openFile: os.OpenFile, readFile: os.ReadFile,
		createTemp: os.CreateTemp, mkdirAll: os.MkdirAll, chmod: os.Chmod,
		open: os.Open, sync: func(f *os.File) error { return f.Sync() },
		close: func(f *os.File) error { return f.Close() }, remove: os.Remove, link: os.Link,
	}
}

func newStoreWithOps(root string, overrides fileOps) *Store {
	ops := defaultFileOps()
	if overrides.lstat != nil { ops.lstat = overrides.lstat }
	if overrides.openFile != nil { ops.openFile = overrides.openFile }
	if overrides.readFile != nil { ops.readFile = overrides.readFile }
	if overrides.createTemp != nil { ops.createTemp = overrides.createTemp }
	if overrides.mkdirAll != nil { ops.mkdirAll = overrides.mkdirAll }
	if overrides.chmod != nil { ops.chmod = overrides.chmod }
	if overrides.open != nil { ops.open = overrides.open }
	if overrides.sync != nil { ops.sync = overrides.sync }
	if overrides.close != nil { ops.close = overrides.close }
	if overrides.remove != nil { ops.remove = overrides.remove }
	if overrides.link != nil { ops.link = overrides.link }
	return &Store{root: filepath.Clean(root), ops: ops}
}

func NewStore(root string) *Store { return newStoreWithOps(root, fileOps{}) }

func Adopt(target, stateRoot, firstReceiptID string) (Record, error) {
	return NewStore(stateRoot).Adopt(target, firstReceiptID)
}

func Load(stateRoot, target string) (Record, error) { return NewStore(stateRoot).Load(target) }

func (s *Store) Adopt(target, firstReceiptID string) (Record, error) {
	if s == nil { return Record{}, errors.New("nil adoption store") }
	if err := s.validateStateRoot(); err != nil { return Record{}, err }
	target, err := cleanTarget(target)
	if err != nil { return Record{}, err }
	info, err := s.targetInfo(target)
	if err != nil { return Record{}, err }
	if record, found, err := s.loadRecord(target); err != nil || found { return record, err }
	backup := target + BackupSuffix
	if _, err := s.ops.lstat(backup); err == nil {
		return Record{}, fmt.Errorf("%w: %s", ErrBackupConflict, backup)
	} else if !os.IsNotExist(err) {
		return Record{}, fmt.Errorf("inspect backup %q: %w", backup, err)
	}
	if firstReceiptID == "" { return Record{}, errors.New("empty first receipt ID") }
	hash, err := s.createBackup(target, backup, info.Mode().Perm())
	if errors.Is(err, errBackupExists) {
		if record, found, loadErr := s.loadRecord(target); loadErr != nil || found { return record, loadErr }
		return Record{}, fmt.Errorf("%w: %s", ErrBackupConflict, backup)
	}
	if err != nil { return Record{}, err }
	record := Record{Schema: SchemaV1, Target: target, Backup: backup, OriginalSHA256: hash, OriginalMode: info.Mode().Perm(), FirstReceiptID: firstReceiptID}
	return s.publishRecord(record)
}

func (s *Store) Load(target string) (Record, error) {
	if s == nil { return Record{}, errors.New("nil adoption store") }
	if err := s.validateStateRoot(); err != nil { return Record{}, err }
	target, err := cleanTarget(target)
	if err != nil { return Record{}, err }
	if _, err := s.targetInfo(target); err != nil { return Record{}, err }
	record, found, err := s.loadRecord(target)
	if err != nil { return Record{}, err }
	if !found { return Record{}, fmt.Errorf("%w: %s", ErrRecordNotFound, target) }
	return record, nil
}

func cleanTarget(target string) (string, error) {
	if target == "" || !filepath.IsAbs(target) { return "", fmt.Errorf("%w: target must be absolute", ErrInvalidTarget) }
	return filepath.Clean(target), nil
}

func (s *Store) validateStateRoot() error {
	if s.root == "" || !filepath.IsAbs(s.root) { return fmt.Errorf("%w: state root must be absolute", ErrUnsafeStateRoot) }
	root := filepath.Clean(s.root)
	volume := filepath.VolumeName(root)
	base := volume + string(filepath.Separator)
	if root == base { return fmt.Errorf("%w: filesystem root is not a state root", ErrUnsafeStateRoot) }
	rel, err := filepath.Rel(base, root)
	if err != nil { return fmt.Errorf("%w: %v", ErrUnsafeStateRoot, err) }
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." { continue }
		current = filepath.Join(current, part)
		info, err := s.ops.lstat(current)
		if os.IsNotExist(err) { return nil }
		if err != nil { return fmt.Errorf("%w: inspect %q: %v", ErrUnsafeStateRoot, current, err) }
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("%w: %q is not a directory", ErrUnsafeStateRoot, current)
		}
		if current == root && info.Mode().Perm()&0002 != 0 { return fmt.Errorf("%w: state root is world-writable", ErrUnsafeStateRoot) }
	}
	return nil
}

func (s *Store) targetInfo(target string) (os.FileInfo, error) {
	info, err := s.ops.lstat(target)
	if os.IsNotExist(err) { return nil, fmt.Errorf("%w: %s", ErrTargetMissing, target) }
	if err != nil { return nil, fmt.Errorf("%w: inspect %q: %v", ErrInvalidTarget, target, err) }
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %q is not a regular non-symlink file", ErrInvalidTarget, target)
	}
	return info, nil
}

func (s *Store) recordPath(target string) string {
	digest := sha256.Sum256([]byte(target))
	return filepath.Join(s.root, "adoptions", fmt.Sprintf("%x.json", digest))
}

func (s *Store) loadRecord(target string) (Record, bool, error) {
	path := s.recordPath(target)
	dirInfo, err := s.ops.lstat(filepath.Dir(path))
	if os.IsNotExist(err) { return Record{}, false, nil }
	if err != nil { return Record{}, false, fmt.Errorf("%w: inspect adoption directory: %v", ErrInvalidRecord, err) }
	if dirInfo.Mode()&os.ModeSymlink != 0 || !dirInfo.IsDir() || dirInfo.Mode().Perm() != 0700 {
		return Record{}, false, fmt.Errorf("%w: unsafe adoption directory", ErrInvalidRecord)
	}
	info, err := s.ops.lstat(path)
	if os.IsNotExist(err) { return Record{}, false, nil }
	if err != nil { return Record{}, false, fmt.Errorf("%w: inspect %q: %v", ErrInvalidRecord, path, err) }
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return Record{}, false, fmt.Errorf("%w: unsafe record %q", ErrInvalidRecord, path)
	}
	data, err := s.ops.readFile(path)
	if err != nil { return Record{}, false, fmt.Errorf("%w: read %q: %v", ErrInvalidRecord, path, err) }
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil { return Record{}, false, fmt.Errorf("%w: decode %q: %v", ErrInvalidRecord, path, err) }
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF { return Record{}, false, fmt.Errorf("%w: trailing data in %q", ErrInvalidRecord, path) }
	if err := s.validateRecord(record, target); err != nil { return Record{}, false, err }
	return record, true, nil
}

func (s *Store) validateRecord(record Record, target string) error {
	if record.Schema != SchemaV1 || record.Target != target || record.Backup != target+BackupSuffix || record.FirstReceiptID == "" || record.OriginalMode&^os.FileMode(0777) != 0 || len(record.OriginalSHA256) != sha256.Size*2 {
		return fmt.Errorf("%w: record does not bind %q", ErrInvalidRecord, target)
	}
	info, err := s.ops.lstat(record.Backup)
	if err != nil { return fmt.Errorf("%w: inspect backup: %v", ErrInvalidRecord, err) }
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != record.OriginalMode {
		return fmt.Errorf("%w: backup metadata mismatch", ErrInvalidRecord)
	}
	data, err := s.ops.readFile(record.Backup)
	if err != nil { return fmt.Errorf("%w: read backup: %v", ErrInvalidRecord, err) }
	hash := sha256.Sum256(data)
	if fmt.Sprintf("%x", hash) != record.OriginalSHA256 {
		return fmt.Errorf("%w: backup hash mismatch", ErrInvalidRecord)
	}
	return nil
}

var errBackupExists = errors.New("backup was created elsewhere")

func (s *Store) createBackup(target, backup string, mode os.FileMode) (string, error) {
	source, err := s.ops.openFile(target, os.O_RDONLY, 0)
	if err != nil { return "", fmt.Errorf("open target %q: %w", target, err) }
	if source == nil { return "", errors.New("open target: nil file") }
	closedSource := false
	defer func() { if !closedSource { _ = s.ops.close(source) } }()
	openedInfo, err := source.Stat()
	if err != nil { return "", fmt.Errorf("stat target %q: %w", target, err) }
	if !openedInfo.Mode().IsRegular() || openedInfo.Mode().Perm() != mode {
		return "", fmt.Errorf("%w: target changed while opening", ErrInvalidTarget)
	}
	dest, err := s.ops.openFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if os.IsExist(err) { return "", errBackupExists }
		return "", fmt.Errorf("create backup %q: %w", backup, err)
	}
	if dest == nil { return "", errors.New("create backup: nil file") }
	closedDest := false
	closeDest := func() error { if closedDest { return nil }; closedDest = true; return s.ops.close(dest) }
	defer func() { if !closedDest { _ = s.ops.close(dest) } }()
	if err := s.ops.chmod(backup, mode); err != nil { _ = closeDest(); return "", fmt.Errorf("preserve backup mode: %w", err) }
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dest, hash), source); err != nil { _ = closeDest(); return "", fmt.Errorf("copy backup: %w", err) }
	if err := s.ops.sync(dest); err != nil { _ = closeDest(); return "", fmt.Errorf("sync backup: %w", err) }
	if err := closeDest(); err != nil { return "", fmt.Errorf("close backup: %w", err) }
	if err := s.ops.close(source); err != nil { closedSource = true; return "", fmt.Errorf("close target: %w", err) }
	closedSource = true
	if err := syncDir(s.ops, filepath.Dir(backup)); err != nil { return "", fmt.Errorf("sync backup directory: %w", err) }
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (s *Store) ensureStateDirs() error {
	if err := s.ensureDir(s.root); err != nil { return err }
	return s.ensureDir(filepath.Join(s.root, "adoptions"))
}

func (s *Store) ensureDir(path string) error {
	info, err := s.ops.lstat(path)
	if os.IsNotExist(err) {
		if err := s.ops.mkdirAll(path, 0700); err != nil { return fmt.Errorf("create state directory: %w", err) }
		info, err = s.ops.lstat(path)
	}
	if err != nil { return fmt.Errorf("inspect state directory: %w", err) }
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() { return fmt.Errorf("%w: unsafe state directory %q", ErrUnsafeStateRoot, path) }
	if err := s.ops.chmod(path, 0700); err != nil { return fmt.Errorf("secure state directory: %w", err) }
	return nil
}

func (s *Store) publishRecord(record Record) (Record, error) {
	if err := s.ensureStateDirs(); err != nil { return Record{}, err }
	data, err := json.Marshal(record)
	if err != nil { return Record{}, fmt.Errorf("encode adoption record: %w", err) }
	dir, path := filepath.Dir(s.recordPath(record.Target)), s.recordPath(record.Target)
	temp, err := s.ops.createTemp(dir, ".adoption-")
	if err != nil { return Record{}, fmt.Errorf("stage adoption record: %w", err) }
	if temp == nil { return Record{}, errors.New("stage adoption record: nil file") }
	tempPath, closed := temp.Name(), false
	cleanup := func() { if !closed { _ = s.ops.close(temp) }; _ = s.ops.remove(tempPath) }
	if err := s.ops.chmod(tempPath, 0600); err != nil { cleanup(); return Record{}, fmt.Errorf("secure adoption record: %w", err) }
	if err := writeAll(temp, data); err != nil { cleanup(); return Record{}, fmt.Errorf("write adoption record: %w", err) }
	if err := s.ops.sync(temp); err != nil { cleanup(); return Record{}, fmt.Errorf("sync adoption record: %w", err) }
	if err := s.ops.close(temp); err != nil { closed = true; _ = s.ops.remove(tempPath); return Record{}, fmt.Errorf("close adoption record: %w", err) }
	closed = true
	if err := s.ops.link(tempPath, path); err != nil {
		_ = s.ops.remove(tempPath)
		if os.IsExist(err) {
			existing, found, loadErr := s.loadRecord(record.Target)
			if loadErr != nil { return Record{}, loadErr }
			if found { return existing, nil }
		}
		return Record{}, fmt.Errorf("publish adoption record: %w", err)
	}
	if err := s.ops.remove(tempPath); err != nil { return Record{}, fmt.Errorf("remove staged adoption record: %w", err) }
	if err := syncDir(s.ops, dir); err != nil { return Record{}, fmt.Errorf("sync adoption directory: %w", err) }
	return record, nil
}

func syncDir(ops fileOps, path string) error {
	dir, err := ops.open(path)
	if err != nil { return err }
	if dir == nil { return errors.New("open directory: nil file") }
	if err := ops.sync(dir); err != nil { _ = ops.close(dir); return err }
	return ops.close(dir)
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil { return err }
		if n == 0 { return io.ErrShortWrite }
		data = data[n:]
	}
	return nil
}
