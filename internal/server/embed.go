package server

import (
	"embed"
	"io/fs"
)

// dist holds the Svelte build that `make ui` copies in. A plain `go build` or
// `go test` sees it empty, so the placeholder page stands in.
//
//go:embed all:dist
var built embed.FS

//go:embed placeholder
var placeholder embed.FS

func UI() fs.FS {
	dist, _ := fs.Sub(built, "dist")
	if _, err := fs.Stat(dist, "index.html"); err == nil {
		return dist
	}
	fallback, _ := fs.Sub(placeholder, "placeholder")
	return fallback
}
