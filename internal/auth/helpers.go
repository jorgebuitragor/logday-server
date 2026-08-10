package auth

import (
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// renderTemplate executes the named admin-panel template with data,
// writing status first. Errors are logged, not written to w — by the
// time template execution fails, headers/partial body may already be
// flushed, so there's nothing safe left to send the client.
func renderTemplate(w http.ResponseWriter, tmpl *template.Template, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("rendering template %q: %v", name, err)
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
