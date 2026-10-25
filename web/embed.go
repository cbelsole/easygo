//go:build embedweb

// Package web exposes the built frontend (web/dist) for embedding in the
// server binary. Build with: go build -tags embedweb ./cmd/easygo
// after running `npm run build` in this directory.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend, or nil if it was not embedded.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
