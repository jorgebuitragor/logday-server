package auth

import (
	"encoding/json"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/mail"
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

// validEmail reports whether email is a syntactically valid address
// (RFC 5322, via the standard library — no new dependency). This is a
// format check only, not a deliverability check: it doesn't touch DNS/MX
// records or send anything, so it happily accepts a domain that's
// syntactically fine and never resolves (even a single-label one like
// "a@b"). It rejects the actually-malformed cases — no "@", empty local
// part, spaces where a real address wouldn't have them — which is enough
// to catch a fat-fingered form submission.
func validEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
