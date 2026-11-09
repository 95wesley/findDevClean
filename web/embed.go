// Package web embeds the built UI so the server ships as one binary.
package web

import "embed"

// FS holds the Vite build output under dist/. Run `make ui` before building
// the binary to refresh it.
//
//go:embed all:dist
var FS embed.FS
