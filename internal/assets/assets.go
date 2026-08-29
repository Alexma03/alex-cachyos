// Package assets exposes the immutable source tree embedded in the configurator.
package assets

import "embed"

//go:generate go run ../../tools/sync-assets

// FS contains generated copies of the explicitly declared repository assets.
//
//go:embed data
var FS embed.FS
