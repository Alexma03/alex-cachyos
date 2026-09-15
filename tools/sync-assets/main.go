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

type assetMode uint8

const (
	assetCopied assetMode = iota
	assetEmbedded
)

type declaredAsset struct {
	path string
	mode assetMode
}

var declaredAssets = []declaredAsset{
	{path: "bin/alex-cachyos-webapp-launch", mode: assetCopied},
	{path: "catalog/", mode: assetCopied},
	{path: "overlays/galaxy/", mode: assetEmbedded},
	{path: "overlays/generic/", mode: assetEmbedded},
	{path: "packaging/", mode: assetCopied},
	{path: "templates/apps/", mode: assetEmbedded},
	{path: "templates/bootstrap/", mode: assetEmbedded},
	{path: "templates/desktop/", mode: assetEmbedded},
	{path: "templates/devtools/", mode: assetEmbedded},
	{path: "templates/hosts/", mode: assetEmbedded},
	{path: "templates/quickshell-polkit/", mode: assetEmbedded},
	{path: "templates/roles/", mode: assetEmbedded},
	{path: "templates/vicinae/", mode: assetEmbedded},
}

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
	mode assetMode
}
type normalizedDeclaration struct {
	raw  string
	rel  string
	mode assetMode
}
type assetPath struct {
	rel  string
	mode assetMode
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
		switch a.mode {
		case assetCopied:
			if err := writeFile(root, name, a.data); err != nil {
				return fmt.Errorf("write %s: %w", a.rel, err)
			}
		case assetEmbedded:
			if err := removeEmbeddedDestination(dest, name, a.rel); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported asset mode for %s", a.rel)
		}
	}
	return writeFile(root, filepath.Join(dest, manifestName), mb)
}

func collectAssets(root, dest string) ([]asset, error) {
	declarations, err := normalizeDeclarations()
	if err != nil {
		return nil, err
	}
	var paths []assetPath
	for _, declaration := range declarations {
		full := filepath.Join(root, filepath.FromSlash(declaration.rel))
		if !within(root, full) || within(full, dest) || within(dest, full) {
			return nil, fmt.Errorf("declared asset path is outside the source root: %q", declaration.raw)
		}
		if err := safePath(root, full); err != nil {
			return nil, err
		}
		i, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("declared asset path %q is unavailable: %w", declaration.raw, err)
		}
		if strings.HasSuffix(declaration.raw, "/") && !i.IsDir() {
			return nil, fmt.Errorf("declared asset directory %q is not a directory", declaration.raw)
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
				rel, e := filepath.Rel(root, name)
				if e != nil || !within(root, name) {
					return fmt.Errorf("asset path escapes source root: %q", name)
				}
				paths = append(paths, assetPath{rel: filepath.ToSlash(rel), mode: declaration.mode})
				return nil
			})
		} else if i.Mode().IsRegular() {
			paths = append(paths, assetPath{rel: declaration.rel, mode: declaration.mode})
		} else {
			err = fmt.Errorf("declared asset %q is not a regular file or directory", declaration.raw)
		}
		if err != nil {
			return nil, fmt.Errorf("scan declared asset %q: %w", declaration.raw, err)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].rel < paths[j].rel })
	for i := 1; i < len(paths); i++ {
		if paths[i].rel == paths[i-1].rel {
			return nil, fmt.Errorf("declared asset paths overlap at %q", paths[i].rel)
		}
	}
	assets := make([]asset, 0, len(paths))
	for _, path := range paths {
		data, err := readRegular(filepath.Join(root, filepath.FromSlash(path.rel)))
		if err != nil {
			return nil, fmt.Errorf("read asset %s: %w", path.rel, err)
		}
		assets = append(assets, asset{rel: path.rel, data: data, mode: path.mode})
	}
	return assets, nil
}

func normalizeDeclarations() ([]normalizedDeclaration, error) {
	declarations := make([]normalizedDeclaration, 0, len(declaredAssets))
	for _, declaration := range declaredAssets {
		if declaration.mode != assetCopied && declaration.mode != assetEmbedded {
			return nil, fmt.Errorf("unsupported asset mode for declaration %q", declaration.path)
		}
		rel, err := sourceRel(declaration.path)
		if err != nil {
			return nil, err
		}
		declarations = append(declarations, normalizedDeclaration{
			raw: declaration.path, rel: rel, mode: declaration.mode,
		})
	}
	sort.Slice(declarations, func(i, j int) bool { return declarations[i].rel < declarations[j].rel })
	for i := 1; i < len(declarations); i++ {
		previous, current := declarations[i-1], declarations[i]
		if current.rel == previous.rel {
			return nil, fmt.Errorf("duplicate declared asset path %q", current.rel)
		}
		if strings.HasPrefix(current.rel, previous.rel+"/") {
			return nil, fmt.Errorf("declared asset paths overlap at %q and %q", previous.rel, current.rel)
		}
	}
	return declarations, nil
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
		switch a.mode {
		case assetCopied:
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
		case assetEmbedded:
			if err := requireAbsentEmbeddedDestination(dest, name, a.rel); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported asset mode for %s", a.rel)
		}
	}
	return nil
}

func removeEmbeddedDestination(dest, name, rel string) error {
	if err := safePath(dest, name); err != nil {
		return fmt.Errorf("remove embedded destination %s: %w", rel, err)
	}
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect embedded destination %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("embedded destination %s is not a regular file", rel)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("remove embedded destination %s: %w", rel, err)
	}
	return nil
}

func requireAbsentEmbeddedDestination(dest, name, rel string) error {
	if err := safePath(dest, name); err != nil {
		return fmt.Errorf("check embedded destination %s: %w", rel, err)
	}
	if _, err := os.Lstat(name); err == nil {
		return fmt.Errorf("embedded destination must be absent: %s", rel)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check embedded destination %s: %w", rel, err)
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
