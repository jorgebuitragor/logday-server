# Web app embebida — Tareas

Estado: implementado.

## Decisiones

- [x] Servir `logday-web` embebido bajo `/app`, mismo origen que la
      API — no CORS configurable.
- [x] Build de Docker vía build context adicional (checkout local
      hermano), no `git clone` — `logday-web` es privado.
- [x] Paquete `web` en la raíz del módulo (no `internal/`) — requisito
      de `//go:embed`.
- [x] Placeholder committeado en `web/dist/` para no romper
      `go build`/`go test` locales sin Docker.

## Implementación

- [x] `web/handler.go`: embed + `Routes(r chi.Router)`, mount
      `/app` con fallback de SPA.
- [x] `web/dist/index.html`: placeholder.
- [x] `cmd/server/main.go`: wire `logdayweb.Routes(r)`.
- [x] `Dockerfile`: etapa `web-builder` (Node, `COPY --from=logday-web`,
      `npm ci && npm run build`), copiada al build de Go antes de
      compilar.
- [x] `docker-compose.yml`: `build.additional_contexts`.
- [x] `README.md`: instrucciones actualizadas (clonar `logday-web`
      hermano, comando de build).
- [x] `logday-web/vite.config.ts`: `base: '/app/'`.

## Tests y validación

- [x] `web/handler_test.go`: redirect a `/app/`, sirve `index.html`
      en la raíz, fallback de SPA en rutas de cliente desconocidas,
      no le pisa rutas de API existentes.
- [x] `go build`/`go test ./...`/`golangci-lint run ./...` en verde.
- [x] Validación manual con el placeholder: `/app/`, deep link, y
      `/health` sin interferencia, vía `go run` local.
- [x] Validación end-to-end contra un build de Docker real (imagen
      completa, `web-builder` + `builder` + runtime): assets con
      `/app/` correcto, JS con `Content-Type` correcto, login
      funcionando vía Playwright headless contra el contenedor
      corriendo, cero errores de consola.
