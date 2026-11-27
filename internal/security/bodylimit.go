package security

import (
	"net/http"
	"strings"
)

// Límites del cuerpo de las peticiones (specs/limites-auth). Los
// endpoints que se pueden llamar sin sesión (login, refresh, setup,
// login del panel) aceptan poco, para que nadie sin cuenta pueda
// mandarle al server cuerpos enormes. El resto exige sesión y lleva
// notas enteras (contenido CRDT, dibujos de Excalidraw), así que usa el
// límite general, configurable con MAX_BODY_MB.
const (
	AuthBodyLimit         int64 = 64 << 10
	PanelBodyLimit        int64 = 1 << 20
	DefaultBodyLimitBytes int64 = 16 << 20
)

// BodyLimitFor devuelve el límite que aplica a path.
func BodyLimitFor(path string, defaultLimit int64) int64 {
	switch {
	case strings.HasPrefix(path, "/auth/"):
		return AuthBodyLimit
	case path == "/setup" || strings.HasPrefix(path, "/admin/panel"):
		// El panel solo manda formularios; el más grande es Configuración,
		// con el texto de la política de datos.
		return PanelBodyLimit
	default:
		return defaultLimit
	}
}

// BodyLimitMiddleware corta el cuerpo de cada petición en su límite. Si
// Content-Length ya lo supera responde 413 sin leer nada; si no lo trae
// (chunked), http.MaxBytesReader hace fallar la lectura al pasarse, y el
// handler responde 400 como con cualquier cuerpo inválido.
func BodyLimitMiddleware(defaultLimit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := BodyLimitFor(r.URL.Path, defaultLimit)
			if r.ContentLength > limit {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
