package adopt

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"alex-cachyos/internal/safefile"
)

const maxWebSearchRead = int64(1 << 20)

var ErrUnmanagedWebSearch = errors.New("unmanaged web-search configuration requires manual credential-reference migration")

type Ownership string

const (
	OwnershipUnmanaged Ownership = "unmanaged"
	OwnershipCreated   Ownership = "created"
)

// WebSearchObservation deliberately contains metadata and a comparison result,
// never live configuration bytes or parsed credential values.
type WebSearchObservation struct {
	Exists          bool
	Blocked         bool
	Ownership       Ownership
	Mode            os.FileMode
	MatchesExpected bool
}

type webSearchOps struct {
	lstat     func(string) (os.FileInfo, error)
	readOwned func(string, string, int64) ([]byte, os.FileMode, error)
}

func defaultWebSearchOps() webSearchOps {
	return webSearchOps{
		lstat: os.Lstat,
		readOwned: func(root, name string, limit int64) ([]byte, os.FileMode, error) {
			trusted, err := safefile.OpenRoot(root)
			if err != nil {
				return nil, 0, err
			}
			defer trusted.Close()
			result, err := trusted.ReadRegularFileWithMetadata(name, limit)
			if err != nil {
				return nil, 0, err
			}
			if result.Metadata.UID != uint32(os.Getuid()) {
				return nil, 0, safefile.ErrUnsafePath
			}
			return result.Data, result.Metadata.Mode.Perm(), nil
		},
	}
}

func ObserveWebSearch(path string, ownership Ownership, expectedSHA256 string) (WebSearchObservation, error) {
	return observeWebSearchWithOps(path, ownership, expectedSHA256, defaultWebSearchOps())
}

func observeWebSearchWithOps(path string, ownership Ownership, expectedSHA256 string, ops webSearchOps) (WebSearchObservation, error) {
	observation := WebSearchObservation{Ownership: ownership}
	if path == "" || !filepath.IsAbs(path) || filepath.Base(path) != "web-search.json" {
		return observation, ErrInvalidTarget
	}
	info, err := ops.lstat(filepath.Clean(path))
	if os.IsNotExist(err) {
		return observation, nil
	}
	if err != nil {
		return observation, fmt.Errorf("inspect web-search metadata: %w", err)
	}
	observation.Exists, observation.Mode = true, info.Mode().Perm()
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		observation.Blocked = true
		return observation, fmt.Errorf("%w: unsafe web-search path", ErrInvalidTarget)
	}
	if ownership != OwnershipCreated {
		observation.Blocked = true
		return observation, ErrUnmanagedWebSearch
	}
	if info.Mode().Perm() != 0600 {
		observation.Blocked = true
		return observation, fmt.Errorf("%w: owned web-search mode must be 0600", ErrInvalidTarget)
	}
	expected, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(expected) != sha256.Size {
		return observation, fmt.Errorf("%w: invalid expected web-search digest", ErrInvalidTarget)
	}
	data, mode, err := ops.readOwned(filepath.Dir(path), filepath.Base(path), maxWebSearchRead)
	if err != nil {
		return observation, fmt.Errorf("observe owned web-search file: %w", err)
	}
	if mode != 0600 {
		return observation, fmt.Errorf("%w: owned web-search mode changed", ErrInvalidTarget)
	}
	digest := sha256.Sum256(data)
	observation.MatchesExpected = subtle.ConstantTimeCompare(expected, digest[:]) == 1
	return observation, nil
}
