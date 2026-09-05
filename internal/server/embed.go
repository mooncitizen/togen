package server

import (
	"embed"
	"io/fs"
)

// Issue 11 replaces the contents of ui/ with the built canvas from ui/dist.
//
//go:embed ui
var embedded embed.FS

func UI() fs.FS {
	sub, _ := fs.Sub(embedded, "ui")
	return sub
}
