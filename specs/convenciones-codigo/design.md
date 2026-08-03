# Convenciones de código — Diseño

Estado: en diseño

## Estructura de paquetes: vertical por dominio

```text
cmd/server/          entrypoint, wiring de router y servidor HTTP
internal/db/         conexión SQLite/Postgres, compartida entre dominios
internal/task/       (futuro) handler + lógica + acceso a datos de Task
internal/note/       (futuro) ídem para Note
internal/auth/       (futuro) ídem para users/devices/sesiones
internal/sync/       (futuro) ídem para el endpoint /sync/changes y WS
...
```

Cada paquete de dominio es autocontenido: su propio archivo de
handlers HTTP, su lógica, sus queries SQL. Se evita partir por capa
transversal (`internal/handlers`, `internal/services`,
`internal/repository`) porque en Go tiende a producir paquetes con
poca cohesión interna donde tocar una entidad implica tocar tres
paquetes distintos. Se evita también hexagonal/ports & adapters
(`domain`/`usecase`/`adapter`) por ser ceremonia excesiva para el
tamaño de este proyecto — mismo criterio que ya se aplicó al descartar
CRDT en todos los campos en vez de acotarlo.

## Linting: golangci-lint v2, preset reforzado

Config en `.golangci.yml` (formato v2):

- **Linters**: set `standard` de golangci-lint (`errcheck`, `govet`,
  `staticcheck`, `unused`, `ineffassign`, etc.) + `gosec` (seguridad —
  detecta cosas como timeouts faltantes en servidores HTTP, permisos
  de archivo laxos, patrones criptográficos inseguros) + `revive`
  (estilo, con reglas puntuales: comentarios en exports, naming de
  errores, parámetros no usados).
- **Formatters**: `gofmt` + `goimports`, corridos vía
  `golangci-lint fmt` (subcomando nuevo de v2, reemplaza tener
  `gofmt`/`goimports` como linters separados).

Ya validado contra el scaffold existente: encontró y se corrigieron 5
issues reales (errores no chequeados de `Close()`/`Write()`, falta de
timeouts en `http.ListenAndServe` — vulnerable a Slowloris — y
permisos de directorio laxos).

## Enforcement: Makefile + GitHub Actions

`Makefile`:

```text
make build   # go build -o bin/server ./cmd/server
make run     # go run ./cmd/server
make test    # go test ./...
make lint    # golangci-lint run ./...
make fmt     # golangci-lint fmt ./...
```

`.github/workflows/ci.yml`: dos jobs sobre `ubuntu-latest`, uno de
lint (`golangci-lint-action@v7`, pinneado a la misma versión que se
usa en local, v2.12.2) y otro de build+test — ambos corriendo en cada
push a `main` y en cada pull request.

## Explícitamente pendiente

- Convenciones de logging (¿`log` estándar, `slog`, o una librería
  estructurada?) — se decide al implementar la primera feature real.
- Manejo de errores (¿wrapping estándar con `fmt.Errorf("%w")`, tipos
  de error propios para casos de negocio?).
- Cobertura mínima de tests, si la hay.
