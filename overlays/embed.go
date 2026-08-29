// Package overlays exposes source overlays as embedded, read-only assets.
package overlays

import "embed"

// FS contains the galaxy overlay without embedding this adapter.
//
//go:embed galaxy
var FS embed.FS
