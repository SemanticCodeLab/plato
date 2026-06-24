package main

import (
	"embed"
	"io/fs"
)

// distFS embeds the built web SPA. A placeholder index.html is committed so the
// binary builds before `npm run build`; running the build overwrites web/dist.
//
//go:embed all:web_dist
var distFS embed.FS

// webFS returns the SPA filesystem rooted at the dist directory.
func webFS() fs.FS {
	sub, err := fs.Sub(distFS, "web_dist")
	if err != nil {
		return nil
	}
	return sub
}
