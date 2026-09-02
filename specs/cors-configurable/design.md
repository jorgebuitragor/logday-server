# CORS configurable — Diseño

Estado: implementado.

## `internal/security/cors.go`

Primer middleware HTTP de `internal/security` — hoy ese paquete solo
tiene primitivas sin conocimiento de HTTP (`password.go`, `token.go`).
Se justifica por adyacencia natural con seguridad (mismo criterio por
el que `internal/security`/`internal/crdt` son la excepción a
"vertical por dominio" en `convenciones-codigo/requirements.md`), en
vez de crear un paquete `internal/httpmw` nuevo para una sola función.

```go
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
```

Sin dependencia externa (`go-chi/cors` u otra) — la necesidad es
chica (allowlist exacta + preflight) y el proyecto ya evita ceremonia
innecesaria para su tamaño (ver `convenciones-codigo/requirements.md`,
"se evitó dividir por capa transversal... por ceremonia excesiva").

## Wiring en `cmd/server/main.go`

Nueva env var `CORS_ALLOWED_ORIGINS`, mismo patrón que el resto
(`envOr`/`os.Getenv`, sin `Config` struct — no existe ese patrón en
este repo todavía, ver `convenciones-codigo`). Parseada con `strings.Split`
por coma, cada elemento con `strings.TrimSpace`, descartando vacíos:

```go
var corsOrigins []string
for _, o := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
    if o = strings.TrimSpace(o); o != "" {
        corsOrigins = append(corsOrigins, o)
    }
}
if len(corsOrigins) > 0 {
    r.Use(security.CORSMiddleware(corsOrigins))
}
```

Wireado justo después de `r.Use(middleware.Recoverer)` (línea ~86),
mismo lugar que el resto del middleware global.

**Por qué global y no scoped a la API JSON**: el router es un único
`chi.Mux` plano — todos los dominios (`task`, `note`, `sync`, `web`
para `/app`, `auth.PanelRoutes` para `/admin/panel`) registran sus
rutas directamente sobre `r` sin `chi.Mount()` por prefijo (confirmado
al explorar el repo: no hay otro mecanismo de scoping por path hoy,
el único agrupamiento existente es `r.Group` para `RequireAuth`/
`RequireAdmin`, no por dominio). Aplicarlo global es seguro: nunca
toca requests same-origin (el navegador no manda `Origin` en esos
casos) y el middleware es un no-op total si `CORS_ALLOWED_ORIGINS`
no está configurada — ni siquiera se registra.

## `.env.example`

Mismo estilo de comentario que `JWT_SECRET`/`ADMIN_EMAIL`:

```
# Optional. Orígenes exactos permitidos para requests cross-origin
# (ej. logday-web hosteada aparte de este servidor), separados por
# coma. Vacío (default) = CORS deshabilitado, igual que hoy. Sin
# soporte de wildcard — cada origen se lista exacto.
#   CORS_ALLOWED_ORIGINS=https://logday.example.com,http://localhost:5173
CORS_ALLOWED_ORIGINS=
```

## Alternativas descartadas

- **`go-chi/cors`** (librería oficial del ecosistema chi) — más
  robusta y probada, pero trae configuración (regex de orígenes,
  modos de credentials, etc.) que este caso de uso no necesita. Se
  prefiere ~30 líneas propias y sin dependencia nueva para algo tan
  acotado.
- **Scoping por path (solo la API JSON, no `/app` ni `/admin/panel`)**
  vía un `chi.Mount()` nuevo — implicaría reestructurar el router
  plano actual (todo cuelga de un único `chi.Mux`) para algo que ya
  es seguro de aplicar global, dado que CORS nunca afecta same-origin.
- **Wildcard `*`** — más simple para el caso "no me importa quién
  llame", pero un self-hoster que configura esto ya sabe qué origen va
  a usar (su propio deploy de `logday-web`), y reflejar orígenes
  exactos es la práctica estándar recomendada cuando se combina con
  cualquier forma de autenticación.

## Fuera de este diseño

- Config vía archivo/`Config` struct — no existe ese patrón en el
  repo todavía (`convenciones-codigo/requirements.md`: "no decidido,
  se define si hace falta"); `CORS_ALLOWED_ORIGINS` sigue el mismo
  `os.Getenv` disperso que ya usan `JWT_SECRET`/`ADMIN_EMAIL`/`PORT`.
- CORS para `/ws` (WebSocket) — el handshake de WebSocket no pasa por
  el mecanismo de CORS de fetch/XHR (usa su propia verificación de
  `Origin`, ya cubierta y con `InsecureSkipVerify` documentado en
  `sync-incremental/design.md`); no se toca acá.
