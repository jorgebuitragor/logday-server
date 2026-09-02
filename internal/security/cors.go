package security

import "net/http"

// CORSMiddleware allows cross-origin requests from an explicit
// allowlist of origins. It's an opt-in capability: logday-server has
// no CORS support by default (logday-web is meant to be served
// same-origin, embedded under /app — see
// specs/webapp-embebida/requirements.md), so an empty allowedOrigins
// makes this a total no-op, leaving every response byte-for-byte
// unchanged from today's behavior.
//
// Only exact origins are matched (no wildcard, no subdomain/regex
// matching) and Access-Control-Allow-Credentials is never sent — the
// JSON API authenticates via Authorization: Bearer, not cookies, so
// the credentialed CORS mode isn't needed.
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" || !allowed[origin] {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
