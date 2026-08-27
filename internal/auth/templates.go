package auth

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templateFS embed.FS

// staticFS holds the panel's static assets — today just the Logday
// brand mark (logo.svg, the "server" variant from the Logday brand
// system: mono white symbol on a dark rounded container, no accent —
// see section 07 of the brand spec), served as both the panel's
// favicon and its header logo.
//
//go:embed static/*.svg
var staticFS embed.FS

// parseTemplates parses every template under templates/ once, at
// NewHandler time. Each page file wraps its whole content in an
// explicit {{define "setup.html"}}...{{end}} (matching the filename by
// convention, not by ParseFS auto-naming) so handlers can
// ExecuteTemplate(w, "setup.html", data) regardless of how the files
// happen to be laid out. partials.html only defines named blocks
// ("head", "nav") shared by the page files — it's never executed
// directly.
func parseTemplates() *template.Template {
	return template.Must(template.ParseFS(templateFS, "templates/*.html"))
}
