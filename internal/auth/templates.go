package auth

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// staticFS holds the panel's static assets: the Logday brand mark
// (logo.svg, the "server" variant from the Logday brand system: mono
// white symbol on a dark rounded container, no accent — see section 07
// of the brand spec), served as both the panel's favicon and its header
// logo; and the panel's stylesheet and scripts, which live in files
// instead of inline <style>/<script> so the panel's Content-Security-Policy
// can be script-src/style-src 'self' (specs/cabeceras-seguridad/).
//
//go:embed static/*.svg static/*.css static/*.js
var staticFS embed.FS

// parseTemplates parses every template under templates/ once, at
// NewHandler time. Each page file wraps its whole content in an
// explicit {{define "setup.html"}}...{{end}} (matching the filename by
// convention, not by ParseFS auto-naming) so handlers can
// ExecuteTemplate(w, "setup.html", data) regardless of how the files
// happen to be laid out. partials.html only defines named blocks
// ("head", "nav") shared by the page files — it's never executed
// directly.
//
// Templates reference static files through {{asset "panel.js"}}, which
// appends a hash of the file's content (?v=…). Cloudflare caches
// /admin/static/* for hours regardless of the server's Cache-Control,
// and keys its cache by full URL: with the hash, a deploy that changes
// a page and its script together can't pair the new page with a stale
// script.
func parseTemplates() *template.Template {
	versions := assetVersions()
	funcs := template.FuncMap{
		"asset": func(name string) string {
			v, ok := versions[name]
			if !ok {
				panic("asset: unknown static file " + name)
			}
			return "/admin/static/" + name + "?v=" + v
		},
	}
	return template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))
}

// assetVersions hashes every embedded static file once: the first 10
// hex characters of its SHA-256, enough to tell versions apart.
func assetVersions() map[string]string {
	entries, err := staticFS.ReadDir("static")
	if err != nil {
		panic(err)
	}
	versions := make(map[string]string, len(entries))
	for _, e := range entries {
		data, err := staticFS.ReadFile("static/" + e.Name())
		if err != nil {
			panic(err)
		}
		sum := sha256.Sum256(data)
		versions[e.Name()] = hex.EncodeToString(sum[:])[:10]
	}
	return versions
}
