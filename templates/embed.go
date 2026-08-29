// Package templates exposes source templates as embedded, read-only assets.
package templates

import "embed"

// FS contains only the named template subdirectories, excluding this adapter.
//
//go:embed apps bootstrap desktop devtools hyprwhspr niri noctalia quickshell-polkit vicinae
var FS embed.FS
