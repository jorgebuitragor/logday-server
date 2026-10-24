// Package web embeds and serves the logday-web SPA (built separately,
// repo github.com/jorgebuitragor/logday-web) under /app — same-origin
// as the sync API, so the browser client never needs CORS (see
// specs/webapp-embebida/design.md for why this beats configuring
// CORS on the API itself).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

//go:embed dist
var distFS embed.FS

// dist is distFS rooted at its own contents — embed.FS keeps the
// "dist/" prefix on every path otherwise, which http.FileServer
// doesn't strip for you.
var dist = mustSub(distFS, "dist")

func mustSub(f embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err) // only fails if "dist" isn't embedded at all — a build-time bug, not a runtime one
	}
	return sub
}

// Routes mounts the embedded SPA at /app. Any path under /app that
// isn't a real built file falls back to index.html, so client-side
// routing (React Router or similar) works on a hard refresh/deep
// link — same pattern any static host uses for an SPA.
func Routes(r chi.Router) {
	fileServer := http.FileServer(http.FS(dist))
	r.Get("/app", http.RedirectHandler("/app/", http.StatusMovedPermanently).ServeHTTP)
	r.Get("/app/*", func(w http.ResponseWriter, r *http.Request) {
		reqPath := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/app"), "/")
		if reqPath != "" {
			if _, err := fs.Stat(dist, reqPath); err != nil {
				reqPath = "" // not a real file — fall back to index.html
			}
		}
		setCacheHeaders(w, reqPath)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + reqPath
		fileServer.ServeHTTP(w, r2)
	})
}

// setCacheHeaders keeps index.html always revalidated (it references
// the current build's hashed asset filenames — cache it and a
// redeploy silently keeps serving the old JS/CSS forever, since the
// browser never even asks) while letting the hashed assets under
// assets/ cache aggressively (their filename changes whenever their
// content does, so there's nothing to invalidate).
func setCacheHeaders(w http.ResponseWriter, reqPath string) {
	if reqPath == "" || reqPath == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
		return
	}
	if strings.HasPrefix(reqPath, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}
