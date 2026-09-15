// Package overlays exposes source overlays as embedded, read-only assets.
package overlays

import "embed"

// FS contains production host overlays without embedding this adapter.
//
//go:embed galaxy generic
var FS embed.FS
