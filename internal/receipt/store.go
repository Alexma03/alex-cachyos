package receipt

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"alex-cachyos/internal/statepath"
)

const (
	receiptStateDir = "alex-cachyos"
	receiptsDirName = "receipts"
	currentName     = "current.json"
	currentSchema   = "alex-cachyos.receipt-index/v1"
)

var ErrReceiptExists = errors.New("receipt already exists")

type fileOps struct {
	mkdirAll   func(string, os.FileMode) error
	chmod      func(string, os.FileMode) error
	createTemp func(string, string) (*os.File, error)
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

func (s *Store) ensureDir(path string) error {
	if err := s.ops.mkdirAll(path, 0700); err != nil {
		return fmt.Errorf("create receipt directory: %w", err)
	}
	if err := s.ops.chmod(path, 0700); err != nil {
		return fmt.Errorf("secure receipt directory: %w", err)
	}
	return nil
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
	temp, err := s.stage(s.root, ".current-", data)
	if err != nil {
		return err
	}
	defer func() { _ = s.ops.remove(temp) }()
	if err := s.ops.rename(temp, filepath.Join(s.root, currentName)); err != nil {
		return fmt.Errorf("publish current receipt index: %w", err)
	}
	if err := s.syncDir(s.root); err != nil {
		return fmt.Errorf("sync receipt index directory: %w", err)
	}
	return nil
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
	if err := s.ops.chmod(temp, 0600); err != nil {
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
