// Package assets exposes the immutable source tree embedded in the configurator.
package assets

import (
	"embed"
	"errors"
	"io/fs"
	"strings"

	"alex-cachyos/overlays"
	"alex-cachyos/templates"
)

//go:generate go run ../../tools/sync-assets

//go:embed data
var dataFS embed.FS

// FS is the read-only composite asset filesystem. It preserves the data/
// namespace and mounts source templates and overlays at repository-relative
// paths.
var FS = compositeFS{mounts: []mount{
	{prefix: "data", fsys: mustSub(dataFS, "data")},
	{prefix: "templates", fsys: templates.FS},
	{prefix: "overlays", fsys: overlays.FS},
}}

type mount struct {
	prefix string
	fsys   fs.FS
}

type compositeFS struct{ mounts []mount }

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func (c compositeFS) Open(name string) (fs.File, error) {
	mounted, rel, err := c.resolve(name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	file, err := mounted.Open(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return file, err
}

func (c compositeFS) ReadFile(name string) ([]byte, error) {
	mounted, rel, err := c.resolve(name)
	if err != nil {
		return nil, &fs.PathError{Op: "read", Path: name, Err: err}
	}
	data, err := fs.ReadFile(mounted, rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}
	return data, err
}

func (c compositeFS) resolve(name string) (fs.FS, string, error) {
	if !fs.ValidPath(name) {
		return nil, "", fs.ErrInvalid
	}
	for _, mounted := range c.mounts {
		switch {
		case name == mounted.prefix:
			return mounted.fsys, ".", nil
		case strings.HasPrefix(name, mounted.prefix+"/"):
			return mounted.fsys, name[len(mounted.prefix)+1:], nil
		}
	}
	return nil, "", fs.ErrNotExist
}
