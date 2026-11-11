// Package webembed holds the built React SPA (web/ builds into dist/).
// FS is nil until a real build exists; cmd/server then falls back to disk.
package webembed

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFS embed.FS

// FS is rooted at dist/, or nil if dist/index.html is missing.
var FS fs.FS

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return
	}
	if _, err := fs.Stat(sub, "index.html"); err == nil {
		FS = sub
	}
}
