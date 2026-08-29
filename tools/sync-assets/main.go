package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	dataRelative   = "internal/assets/data"
	manifestName   = "source-manifest.json"
	manifestSchema = "alex-cachyos.source-manifest/v1"
)

var declaredPaths = []string{"catalog/"}

type manifest struct {
	Schema  string  `json:"schema"`
	Entries []entry `json:"entries"`
}
type entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type asset struct {
	rel  string
	data []byte
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	f := flag.NewFlagSet("sync-assets", flag.ContinueOnError)
	f.SetOutput(os.Stderr)
	check := f.Bool("check", false, "verify generated assets without changing them")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", f.Arg(0))
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	return syncAssets(root, filepath.Join(root, dataRelative), *check)
}
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if i, e := os.Stat(filepath.Join(dir, "go.mod")); e == nil && i.Mode().IsRegular() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("repository root with go.mod not found from %q", dir)
}

func syncAssets(root, dest string, check bool) error {
	var err error
	if root, err = filepath.Abs(root); err != nil {
		return err
	}
	if dest, err = filepath.Abs(dest); err != nil {
		return err
	}
	if !within(root, dest) || filepath.Clean(root) == filepath.Clean(dest) {
		return fmt.Errorf("asset destination must be inside repository root")
	}
	if err := safePath(root, dest); err != nil {
		return err
	}
	assets, err := collectAssets(root, dest)
	if err != nil {
		return err
	}
	mb, err := manifestBytes(assets)
	if err != nil {
		return err
	}
	if check {
		return checkAssets(dest, assets, mb)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, a := range assets {
		name := filepath.Join(dest, filepath.FromSlash(a.rel))
		if err := writeFile(root, name, a.data); err != nil {
			return fmt.Errorf("write %s: %w", a.rel, err)
		}
	}
	return writeFile(root, filepath.Join(dest, manifestName), mb)
}

func collectAssets(root, dest string) ([]asset, error) {
	var names []string
	for _, raw := range declaredPaths {
		rel, err := sourceRel(raw)
		if err != nil {
			return nil, err
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		if !within(root, full) || within(full, dest) || within(dest, full) {
			return nil, fmt.Errorf("declared asset path is outside the source root: %q", raw)
		}
		if err := safePath(root, full); err != nil {
			return nil, err
		}
		i, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("declared asset path %q is unavailable: %w", raw, err)
		}
		if strings.HasSuffix(raw, "/") && !i.IsDir() {
			return nil, fmt.Errorf("declared asset directory %q is not a directory", raw)
		}
		if i.IsDir() {
			err = filepath.WalkDir(full, func(name string, d fs.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("declared asset tree contains symlink %q", name)
				}
				if d.IsDir() {
					return nil
				}
				if !d.Type().IsRegular() {
					return fmt.Errorf("declared asset %q is not a regular file", name)
				}
				r, e := filepath.Rel(root, name)
				if e != nil || !within(root, name) {
					return fmt.Errorf("asset path escapes source root: %q", name)
				}
				names = append(names, filepath.ToSlash(r))
				return nil
			})
		} else if i.Mode().IsRegular() {
			names = append(names, rel)
		} else {
			err = fmt.Errorf("declared asset %q is not a regular file or directory", raw)
		}
		if err != nil {
			return nil, fmt.Errorf("scan declared asset %q: %w", raw, err)
		}
	}
	sort.Strings(names)
	for i := 1; i < len(names); i++ {
		if names[i] == names[i-1] {
			return nil, fmt.Errorf("declared asset paths overlap at %q", names[i])
		}
	}
	assets := make([]asset, 0, len(names))
	for _, rel := range names {
		data, err := readRegular(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", rel, err)
		}
		assets = append(assets, asset{rel: rel, data: data})
	}
	return assets, nil
}
func sourceRel(raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00") {
		return "", fmt.Errorf("declared asset path must be repository-relative: %q", raw)
	}
	rel := strings.TrimRight(raw, "/")
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if rel == "" || clean != rel || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("declared asset path must stay within repository root: %q", raw)
	}
	return clean, nil
}
func manifestBytes(assets []asset) ([]byte, error) {
	m := manifest{Schema: manifestSchema, Entries: make([]entry, 0, len(assets))}
	for _, a := range assets {
		sum := sha256.Sum256(a.data)
		m.Entries = append(m.Entries, entry{a.rel, int64(len(a.data)), hex.EncodeToString(sum[:])})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	return append(b, '\n'), nil
}

func checkAssets(dest string, assets []asset, expected []byte) error {
	name := filepath.Join(dest, manifestName)
	if err := safePath(dest, name); err != nil {
		return err
	}
	got, err := readRegular(name)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestName, err)
	}
	if !bytes.Equal(got, expected) {
		return fmt.Errorf("asset manifest drift detected")
	}
	for _, a := range assets {
		name := filepath.Join(dest, filepath.FromSlash(a.rel))
		if err := safePath(dest, name); err != nil {
			return fmt.Errorf("read generated asset %s: %w", a.rel, err)
		}
		data, err := readRegular(name)
		if err != nil {
			return fmt.Errorf("read generated asset %s: %w", a.rel, err)
		}
		if !bytes.Equal(data, a.data) {
			return fmt.Errorf("asset drift detected: %s", a.rel)
		}
	}
	return nil
}
func readRegular(name string) ([]byte, error) {
	i, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if i.Mode()&os.ModeSymlink != 0 || !i.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	return os.ReadFile(name)
}
func writeFile(root, name string, data []byte) error {
	if err := safePath(root, name); err != nil {
		return err
	}
	if i, err := os.Lstat(name); err == nil && !i.Mode().IsRegular() {
		return fmt.Errorf("destination is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, 0o644)
}
func safePath(root, target string) error {
	if !within(root, target) {
		return fmt.Errorf("asset path escapes root: %s", target)
	}
	for {
		i, err := os.Lstat(target)
		if err == nil {
			if i.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("asset path contains symlink: %s", target)
			}
			real, err := filepath.EvalSymlinks(target)
			if err != nil {
				return err
			}
			if !within(root, real) {
				return fmt.Errorf("asset path resolves outside root: %s", target)
			}
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if target == root {
			return nil
		}
		parent := filepath.Dir(target)
		if parent == target {
			return fmt.Errorf("asset path has no repository root")
		}
		target = parent
	}
}
func within(root, name string) bool {
	rel, err := filepath.Rel(root, name)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
