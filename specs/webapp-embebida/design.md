# Web app embebida — Diseño

Estado: implementado.

## Paquete `web`

`web/handler.go`, paquete `web` en la raíz del módulo (no
`internal/webapp`) — `//go:embed` no admite `..`, así que el
directorio embebido tiene que ser un subdirectorio del paquete que lo
declara. `web/dist/` es ese subdirectorio.

```go
//go:embed dist
var distFS embed.FS
var dist = mustSub(distFS, "dist") // re-rootea para no arrastrar el prefijo "dist/"
```

`Routes(r chi.Router)` monta:
- `GET /app` → 301 a `/app/` (URL canónica).
- `GET /app/*` → si el path pedido existe como archivo real en el
  build, lo sirve tal cual; si no, reescribe a `/` y sirve
  `index.html` (fallback de SPA). Ver `handler_test.go` para los
  casos cubiertos (raíz, ruta de cliente desconocida, no le pisa
  rutas de la API).

## Placeholder de `web/dist/`

`web/dist/index.html` es un placeholder committeado — una página
mínima que explica que esa build no tiene el frontend real. Existe
únicamente para que `go build`/`go run`/`go test` funcionen sin
Docker: `//go:embed dist` falla en tiempo de compilación si el
directorio no existe o está vacío, así que sin este placeholder
cualquier desarrollo local del servidor (lo que se viene haciendo
toda la vida de este repo) se rompería.

El build de Docker (etapa `builder`) sobreescribe `web/dist` con el
output real de `logday-web` **antes** de compilar
(`COPY --from=web-builder /web-src/dist ./web/dist`), así que el
placeholder nunca llega a un binario de producción.

## Build de Docker: contexto adicional, no `git clone`

`logday-web` es privado — un `git clone` anónimo en la etapa
`web-builder` del Dockerfile fallaría sin credenciales. Se usa en su
lugar un **build context adicional** de BuildKit
(`docker/dockerfile:1.4`+):

```dockerfile
FROM node:22-alpine AS web-builder
WORKDIR /web-src
COPY --from=logday-web . .
RUN npm ci && npm run build
```

Y quien construye la imagen pasa ese contexto apuntando a un checkout
local hermano:

```bash
docker build --build-context logday-web=../logday-web -t logday-server .
```

`docker-compose.yml` usa el equivalente declarativo
(`build.additional_contexts`). Ver README, sección "Levantar el
servidor", para el comando completo (incluye clonar `logday-web`
primero si no está ya).

**Alternativas descartadas:**
- `git clone` con token de GitHub como build secret — más flexible
  para CI remoto, pero agrega un paso de configuración de
  credenciales que hoy nadie necesita (todo el desarrollo es local,
  un solo mantenedor). Queda como mejora futura si se automatiza un
  pipeline de release.
- Hacer `logday-web` público — descartado, es una decisión de
  visibilidad tomada aparte, no algo que este spec deba forzar.
- Git submodule — descartado por fricción de DX (fácil de olvidar
  actualizar) sin beneficio real sobre el build context de BuildKit
  para este caso de uso.

## `base: '/app/'` en `logday-web`

Del lado de `logday-web`, `vite.config.ts` fija `base: '/app/'` (en
dev y en build, no solo build) — así los assets del bundle referencian
`/app/assets/...` en vez de `/assets/...`, coincidiendo con el mount
point acá. Como consecuencia, las llamadas a la API del cliente son
siempre relativas (`fetch('/tasks')`) — mismo origen tanto en dev
(proxeado por Vite hacia un `logday-server` local) como en producción
(embebido, literalmente el mismo origen) — sin URL base configurable
en ningún lado.

## Fuera de este diseño

- Cacheo/compresión de los assets estáticos (`http.FileServer` los
  sirve tal cual, sin gzip ni cache-control explícito) — no se ha
  identificado que sea necesario todavía dado el tamaño actual del
  bundle (~50KB gzip).
- Versionado explícito de qué commit/tag de `logday-web` corresponde
  a cada release de `logday-server` — hoy es "lo que esté en el
  checkout local al momento del build", sin un `ARG` de referencia
  fijado.
