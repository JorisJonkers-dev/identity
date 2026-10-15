// Package web embeds the built single-page app, so the Go binary serves it beside the API.
package web

import (
	"embed"
	"io/fs"
)

// The web build writes dist/; a tracked dist/.gitkeep keeps the pattern matching before it has.
//
//go:embed all:dist
var dist embed.FS

// Dist returns the files of the web build that was present when the binary was compiled.
func Dist() fs.FS { return sub(dist, "dist") }

// sub panics on an invalid directory name, which "dist" is not.
func sub(fsys fs.FS, dir string) fs.FS {
	s, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return s
}
