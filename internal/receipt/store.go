package receipt

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"alex-cachyos/internal/safefile"
	"alex-cachyos/internal/statepath"
)

const (
	receiptStateDir = "alex-cachyos"
	receiptsDirName = "receipts"
	currentName     = "current.json"
	currentSchema   = "alex-cachyos.receipt-index/v1"
	maxReceiptRead  = int64(16 << 20)
)

var (
	ErrReceiptExists    = errors.New("receipt already exists")
	ErrReceiptNotFound  = errors.New("receipt not found")
	ErrReceiptAmbiguous = errors.New("receipt ID is ambiguous")
)

var storedRunIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:+@~-]{0,255}$`)

type fileOps struct {
	mkdirAll   func(string, os.FileMode) error
	chmod      func(string, os.FileMode) error
	createTemp func(string, string) (*os.File, error)
	lstat      func(string) (os.FileInfo, error)
	open       func(string) (*os.File, error)
	sync       func(*os.File) error
	close      func(*os.File) error
	remove     func(string) error
	link       func(string, string) error
	rename     func(string, string) error
}

type Store struct {
	root string
	ops  fileOps
}

type currentIndex struct {
	Schema        string `json:"schema"`
	RunID         string `json:"runId"`
	ReceiptPath   string `json:"receiptPath"`
	ReceiptSHA256 string `json:"receiptSha256"`
}

func defaultFileOps() fileOps {
	return fileOps{
		mkdirAll:   os.MkdirAll,
		chmod:      os.Chmod,
		createTemp: os.CreateTemp,
		lstat:      os.Lstat,
		open:       os.Open,
		sync:       func(f *os.File) error { return f.Sync() },
		close:      func(f *os.File) error { return f.Close() },
		remove:     os.Remove,
		link:       os.Link,
		rename:     os.Rename,
	}
}

func newStoreWithOps(root string, overrides fileOps) *Store {
	ops := defaultFileOps()
	if overrides.mkdirAll != nil {
		ops.mkdirAll = overrides.mkdirAll
	}
	if overrides.chmod != nil {
		ops.chmod = overrides.chmod
	}
	if overrides.createTemp != nil {
		ops.createTemp = overrides.createTemp
	}
	if overrides.lstat != nil {
		ops.lstat = overrides.lstat
	}
	if overrides.open != nil {
		ops.open = overrides.open
	}
	if overrides.sync != nil {
		ops.sync = overrides.sync
	}
	if overrides.close != nil {
		ops.close = overrides.close
	}
	if overrides.remove != nil {
		ops.remove = overrides.remove
	}
	if overrides.link != nil {
		ops.link = overrides.link
	}
	if overrides.rename != nil {
		ops.rename = overrides.rename
	}
	return &Store{root: filepath.Clean(root), ops: ops}
}

func NewStore(root string) *Store { return newStoreWithOps(root, fileOps{}) }

func NewStoreAtStateHome(stateHome string) *Store {
	return NewStore(filepath.Join(stateHome, receiptStateDir))
}

func NewStoreFromPaths(paths statepath.Paths) *Store {
	return NewStoreAtStateHome(paths.StateHome)
}

func (s *Store) Publish(r Receipt) (string, error) {
	if s == nil {
		return "", errors.New("nil receipt store")
	}
	data, err := CanonicalJSON(r)
	if err != nil {
		return "", err
	}
	if err := s.ensureDir(s.root); err != nil {
		return "", err
	}
	receiptsDir := filepath.Join(s.root, receiptsDirName)
	if err := s.ensureDir(receiptsDir); err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	name := fmt.Sprintf("%s-%x.json", receiptTimestamp(r.FinishedAt), digest[:6])
	path := filepath.Join(receiptsDir, name)
	if err := s.publishReceipt(receiptsDir, path, data); err != nil {
		return "", err
	}
	if err := s.updateCurrent(path, r.RunID, digest); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Store) Write(r Receipt) error { _, err := s.Publish(r); return err }

// Current reads the atomically selected immutable receipt without reopening an
// inspected pathname. Both state directories and both files must retain their
// configurator-owned modes and current-user ownership.
func (s *Store) Current() (Receipt, string, error) {
	if s == nil {
		return Receipt{}, "", errors.New("nil receipt store")
	}
	root, err := safefile.OpenRoot(s.root)
	if err != nil {
		return Receipt{}, "", currentReadError("open receipt state root", err)
	}
	defer root.Close()
	if metadata, err := root.Stat(); err != nil || !ownedMode(metadata, 0700) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe receipt state directory", ErrInvalid)
	}

	indexResult, err := root.ReadRegularFileWithMetadata(currentName, maxReceiptRead)
	if err != nil {
		return Receipt{}, "", currentReadError("read current receipt index", err)
	}
	if !ownedMode(indexResult.Metadata, 0600) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe current receipt index metadata", ErrInvalid)
	}
	index, err := decodeCurrentIndex(indexResult.Data)
	if err != nil {
		return Receipt{}, "", err
	}
	name, receiptPath, err := s.currentReceiptPath(index.ReceiptPath)
	if err != nil {
		return Receipt{}, "", err
	}

	receipts, err := root.OpenDir(receiptsDirName)
	if err != nil {
		return Receipt{}, "", currentReadError("open receipts directory", err)
	}
	defer receipts.Close()
	if metadata, err := receipts.Stat(); err != nil || !ownedMode(metadata, 0700) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe receipts directory", ErrInvalid)
	}
	receiptResult, err := receipts.ReadRegularFileWithMetadata(name, maxReceiptRead)
	if err != nil {
		return Receipt{}, "", currentReadError("read current receipt", err)
	}
	if !ownedMode(receiptResult.Metadata, 0600) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe receipt metadata", ErrInvalid)
	}
	if !matchesSHA256(index.ReceiptSHA256, receiptResult.Data) {
		return Receipt{}, "", fmt.Errorf("%w: current receipt digest mismatch", ErrInvalid)
	}
	receipt, err := Parse(receiptResult.Data)
	if err != nil {
		return Receipt{}, "", fmt.Errorf("parse current receipt: %w", err)
	}
	if receipt.RunID != index.RunID {
		return Receipt{}, "", fmt.Errorf("%w: current receipt run ID mismatch", ErrInvalid)
	}
	return receipt, receiptPath, nil
}

// Read resolves an immutable receipt by its recorded run ID. Receipt file
// names are content-addressed rather than run-ID-addressed, so lookup scans the
// descriptor-open receipts directory and validates every candidate before it
// compares the ID. Duplicate IDs fail closed instead of selecting by filename.
func (s *Store) Read(runID string) (Receipt, string, error) {
	if s == nil {
		return Receipt{}, "", errors.New("nil receipt store")
	}
	if !storedRunIDPattern.MatchString(runID) {
		return Receipt{}, "", fmt.Errorf("%w: invalid receipt ID", ErrInvalid)
	}
	root, err := safefile.OpenRoot(s.root)
	if err != nil {
		return Receipt{}, "", currentReadError("open receipt state root", err)
	}
	defer root.Close()
	if metadata, err := root.Stat(); err != nil || !ownedMode(metadata, 0700) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe receipt state directory", ErrInvalid)
	}
	receipts, err := root.OpenDir(receiptsDirName)
	if err != nil {
		return Receipt{}, "", currentReadError("open receipts directory", err)
	}
	defer receipts.Close()
	if metadata, err := receipts.Stat(); err != nil || !ownedMode(metadata, 0700) {
		return Receipt{}, "", fmt.Errorf("%w: unsafe receipts directory", ErrInvalid)
	}
	names, err := receipts.ReadDirNames()
	if err != nil {
		return Receipt{}, "", currentReadError("list receipts", err)
	}
	var found Receipt
	var foundPath string
	for _, name := range names {
		if filepath.Ext(name) != ".json" || name == currentName {
			continue
		}
		result, err := receipts.ReadRegularFileWithMetadata(name, maxReceiptRead)
		if err != nil {
			return Receipt{}, "", currentReadError("read stored receipt", err)
		}
		if !ownedMode(result.Metadata, 0600) {
			return Receipt{}, "", fmt.Errorf("%w: unsafe receipt metadata", ErrInvalid)
		}
		value, err := Parse(result.Data)
		if err != nil {
			return Receipt{}, "", fmt.Errorf("parse stored receipt: %w", err)
		}
		if value.RunID != runID {
			continue
		}
		if foundPath != "" {
			return Receipt{}, "", ErrReceiptAmbiguous
		}
		found, foundPath = value, filepath.Join(s.root, receiptsDirName, name)
	}
	if foundPath == "" {
		return Receipt{}, "", ErrReceiptNotFound
	}
	return found, foundPath, nil
}

func ownedMode(metadata safefile.Metadata, mode os.FileMode) bool {
	return metadata.Mode.Perm() == mode && metadata.UID == uint32(os.Getuid())
}

func fileOwner(info os.FileInfo) (uint32, uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return stat.Uid, stat.Gid, true
}

func currentReadError(operation string, err error) error {
	for _, class := range []error{safefile.ErrUnsafeRoot, safefile.ErrInvalidPath, safefile.ErrUnsafePath, safefile.ErrNotRegular, safefile.ErrNotDirectory, safefile.ErrTooLarge, safefile.ErrChanged} {
		if errors.Is(err, class) {
			return fmt.Errorf("%w: %s: %v", ErrInvalid, operation, err)
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func decodeCurrentIndex(data []byte) (currentIndex, error) {
	var index currentIndex
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return currentIndex{}, fmt.Errorf("%w: decode current receipt index", ErrInvalid)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return currentIndex{}, fmt.Errorf("%w: trailing current receipt index data", ErrInvalid)
	}
	if index.Schema != currentSchema || index.RunID == "" || index.ReceiptPath == "" || index.ReceiptSHA256 == "" {
		return currentIndex{}, fmt.Errorf("%w: incomplete current receipt index", ErrInvalid)
	}
	return index, nil
}

func (s *Store) currentReceiptPath(relative string) (string, string, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || filepath.Clean(relative) != relative {
		return "", "", fmt.Errorf("%w: current receipt path must be strict and relative", ErrInvalid)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 2 || parts[0] != receiptsDirName || parts[1] == "" || parts[1] == "." || parts[1] == ".." {
		return "", "", fmt.Errorf("%w: current receipt path must name one receipt", ErrInvalid)
	}
	return parts[1], filepath.Join(s.root, receiptsDirName, parts[1]), nil
}

func matchesSHA256(expected string, data []byte) bool {
	encoded, err := hex.DecodeString(expected)
	if err != nil || len(encoded) != sha256.Size {
		return false
	}
	digest := sha256.Sum256(data)
	return subtle.ConstantTimeCompare(encoded, digest[:]) == 1
}

func (s *Store) ensureDir(path string) error {
	if err := s.ops.mkdirAll(path, 0700); err != nil {
		return fmt.Errorf("create receipt directory: %w", err)
	}
	info, err := s.ops.lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("unsafe receipt directory")
	}
	uid, _, ok := fileOwner(info)
	if !ok || uid != uint32(os.Getuid()) {
		return fmt.Errorf("unsafe receipt directory owner")
	}
	dir, err := s.ops.open(path)
	if err != nil {
		return fmt.Errorf("open receipt directory: %w", err)
	}
	openedInfo, statErr := dir.Stat()
	if statErr != nil || !os.SameFile(info, openedInfo) {
		_ = s.ops.close(dir)
		return fmt.Errorf("receipt directory changed while opening")
	}
	if err := dir.Chmod(0700); err != nil {
		_ = s.ops.close(dir)
		return fmt.Errorf("secure receipt directory: %w", err)
	}
	return s.ops.close(dir)
}

func (s *Store) publishReceipt(dir, path string, data []byte) error {
	temp, err := s.stage(dir, ".receipt-", data)
	if err != nil {
		return err
	}
	if err := s.ops.link(temp, path); err != nil {
		_ = s.ops.remove(temp)
		if os.IsExist(err) {
			return fmt.Errorf("%w: %s", ErrReceiptExists, path)
		}
		return fmt.Errorf("publish receipt: %w", err)
	}
	if err := s.ops.remove(temp); err != nil {
		return fmt.Errorf("remove receipt temporary file: %w", err)
	}
	if err := s.syncDir(dir); err != nil {
		return fmt.Errorf("sync receipt directory: %w", err)
	}
	return nil
}

func (s *Store) updateCurrent(receiptPath, runID string, digest [32]byte) error {
	index := currentIndex{
		Schema:        currentSchema,
		RunID:         runID,
		ReceiptPath:   filepath.Join(receiptsDirName, filepath.Base(receiptPath)),
		ReceiptSHA256: fmt.Sprintf("%x", digest[:]),
	}
	data, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("encode current receipt index: %w", err)
	}
	currentPath := filepath.Join(s.root, currentName)
	previous, readErr := s.readCurrentBytes()
	hasPrevious := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read current receipt index: %w", readErr)
	}
	temp, err := s.stage(s.root, ".current-", data)
	if err != nil {
		return err
	}
	defer func() { _ = s.ops.remove(temp) }()
	if err := s.ops.rename(temp, currentPath); err != nil {
		return fmt.Errorf("publish current receipt index: %w", err)
	}
	if err := s.syncDir(s.root); err != nil {
		s.rollbackCurrent(currentPath, previous, hasPrevious)
		return fmt.Errorf("sync receipt index directory: %w", err)
	}
	return nil
}

func (s *Store) readCurrentBytes() ([]byte, error) {
	root, err := safefile.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	result, err := root.ReadRegularFileWithMetadata(currentName, maxReceiptRead)
	if err != nil {
		return nil, err
	}
	if !ownedMode(result.Metadata, 0600) {
		return nil, fmt.Errorf("%w: unsafe current receipt index metadata", ErrInvalid)
	}
	return result.Data, nil
}

func (s *Store) rollbackCurrent(currentPath string, previous []byte, hasPrevious bool) {
	if hasPrevious {
		if temp, err := s.stage(s.root, ".current-rollback-", previous); err == nil {
			if err := s.ops.rename(temp, currentPath); err == nil {
				_ = s.ops.remove(temp)
				_ = s.syncDir(s.root)
				return
			}
			_ = s.ops.remove(temp)
		}
	}
	_ = s.ops.remove(currentPath)
}

func (s *Store) stage(dir, pattern string, data []byte) (string, error) {
	file, err := s.ops.createTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create receipt temporary file: %w", err)
	}
	if file == nil {
		return "", errors.New("create receipt temporary file: nil file")
	}
	temp := file.Name()
	closed := false
	closeFile := func() error {
		if closed {
			return nil
		}
		closed = true
		return s.ops.close(file)
	}
	cleanup := func() {
		_ = closeFile()
		_ = s.ops.remove(temp)
	}
	defer func() {
		if !closed {
			cleanup()
		}
	}()
	if err := file.Chmod(0600); err != nil {
		cleanup()
		return "", fmt.Errorf("secure receipt temporary file: %w", err)
	}
	if err := writeAll(file, data); err != nil {
		cleanup()
		return "", fmt.Errorf("write receipt temporary file: %w", err)
	}
	if err := s.ops.sync(file); err != nil {
		cleanup()
		return "", fmt.Errorf("sync receipt temporary file: %w", err)
	}
	if err := closeFile(); err != nil {
		_ = s.ops.remove(temp)
		return "", fmt.Errorf("close receipt temporary file: %w", err)
	}
	return temp, nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (s *Store) syncDir(path string) error {
	dir, err := s.ops.open(path)
	if err != nil {
		return err
	}
	if err := s.ops.sync(dir); err != nil {
		_ = s.ops.close(dir)
		return err
	}
	return s.ops.close(dir)
}

func receiptTimestamp(value string) string {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t = time.Now().UTC()
	}
	return t.UTC().Format("20060102T150405Z")
}
