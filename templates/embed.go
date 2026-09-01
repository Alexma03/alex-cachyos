// Package templates exposes source templates as embedded, read-only assets.
package templates

import "embed"

// FS contains only the named template subdirectories, excluding this adapter.
//
//go:embed apps bootstrap desktop devtools hosts quickshell-polkit roles vicinae
var FS embed.FS
