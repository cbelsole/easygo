//go:build !embedweb

// Package web exposes the built frontend (web/dist) for embedding in the
// server binary. Without the embedweb build tag nothing is embedded and the
// server serves web/dist from disk instead.
package web

import "io/fs"

// Dist returns nil: the frontend is not embedded in this build.
func Dist() fs.FS { return nil }
