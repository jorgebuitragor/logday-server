# CORS configurable — Tareas

Estado: implementado.

## Decisiones

- [x] Opt-in vía `CORS_ALLOWED_ORIGINS`, vacío por defecto = deshabilitado.
- [x] Orígenes exactos, sin wildcard.
- [x] Sin `Access-Control-Allow-Credentials` (API JSON usa Bearer, no cookies).
- [x] Middleware propio en `internal/security`, sin dependencia externa.
- [x] Aplicado global (no scoped por path) — router es un único `chi.Mux` plano.

## Implementación

- [x] `internal/security/cors.go`: `CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler`.
- [x] `cmd/server/main.go`: leer `CORS_ALLOWED_ORIGINS`, parsear (`parseCORSOrigins` — split por coma + trim + descartar vacíos), wire `r.Use(security.CORSMiddleware(...))` solo si la lista no queda vacía, justo después de `middleware.Recoverer`.
- [x] `.env.example`: documentar la var nueva.
- [x] `specs/README.md`: agregar fila al índice.

## Tests y validación

- [x] `internal/security/cors_test.go`:
  - allowlist vacía → nunca agrega headers `Access-Control-*`.
  - origen no listado → nunca agrega headers.
  - origen listado, request normal (no preflight) → `Access-Control-Allow-Origin` reflejado + `Vary: Origin`, sigue al handler siguiente.
  - origen listado, `OPTIONS` (preflight) → `204`, los 3 headers de preflight, NO llega al handler siguiente.
- [x] `go build ./...` / `go vet ./...` / `golangci-lint run ./...` en verde. `go test ./...` completo en verde (todos los paquetes, no solo el nuevo).
- [x] Validación manual con el servidor corriendo local (`go run ./cmd/server`):
  - Sin `CORS_ALLOWED_ORIGINS`: confirmado que ninguna respuesta trae headers `Access-Control-*` (regresión: modo embebido actual sin cambios).
  - Con `CORS_ALLOWED_ORIGINS=http://localhost:5173,https://logday.example.com`: origen permitido refleja el header + `Vary: Origin`, origen no listado no lo trae, preflight `OPTIONS` responde `204` con los 3 headers correctos.
