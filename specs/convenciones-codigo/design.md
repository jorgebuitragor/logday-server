# Convenciones de código — Diseño

Estado: en diseño

## Estructura de paquetes: vertical por dominio

```text
cmd/server/          entrypoint, wiring de router y servidor HTTP
internal/db/         conexión SQLite/Postgres + migraciones (goose), compartida entre dominios
internal/security/   primitivas criptográficas genéricas (hash de password, JWT, tokens opacos) — sin lógica de negocio
internal/auth/       users/devices/sesiones: handlers, store, bootstrap del admin
internal/task/       handlers, store y ChangesSince (exportada) de Task
internal/sync/       fan-out a ChangesSince de cada dominio + GET /sync/changes
internal/note/       (futuro) ídem para Note
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

`internal/security` es la única excepción deliberada a "todo vive en
su paquete de dominio": agrupa primitivas criptográficas que no tienen
ningún conocimiento del dominio auth (hash de password, firmar/parsear
JWT, generar/hashear tokens opacos de alta entropía) y que cualquier
dominio futuro podría necesitar sin depender de `internal/auth`
completo. La regla para decidir si algo va ahí en vez de en el paquete
de dominio que lo usa: si la función no sabe qué es un `User` o un
`Device` — solo trabaja con `string`/`[]byte`/`jwt.Claims` genéricos —
va en `security`. Si conoce la forma de las entidades del dominio (p.
ej. los claims concretos `{UserID, DeviceID, IsAdmin}` de un access
token), se queda en el paquete de dominio, que arma esos claims y
llama a `security.SignJWT`/`security.ParseJWT`.

### `internal/sync`: agregador, no otro dominio más

`internal/sync` no encaja en "un paquete por dominio" porque no tiene
tabla propia ni entidad propia — su trabajo es leer de las tablas de
*otros* dominios para armar `GET /sync/changes`. Patrón elegido:

- Cada dominio sincronizable expone una **función de paquete**
  exportada (no un método de su `store`, que es privado):
  `func ChangesSince(ctx context.Context, db *sql.DB, userID string, since int64) ([]Task, error)`
  en `internal/task`. Toma `*sql.DB` directamente en vez de un
  `*store` — así el paquete llamador no necesita nombrar un tipo
  privado de otro paquete (Go no lo permite: se puede *usar* un valor
  de tipo no exportado obtenido por inferencia, pero no se puede
  *escribir* su nombre de tipo en otro paquete — p. ej. como campo de
  struct o firma de interfaz — bloqueando cualquier variante donde
  `sync` intentara guardarse una referencia tipada al `store` de
  `task`).
- `internal/sync` importa cada dominio directamente y llama a su
  `ChangesSince` (hoy solo `task.ChangesSince`), en vez de una
  interfaz `Source` genérica con auto-registro. Se descartó la
  interfaz genérica por prematura: con una sola entidad real no hay
  nada que generalizar todavía; agregar la segunda (`note`) es agregar
  un bloque más en `sync/store.go`, no una migración de arquitectura.
  Reconsiderar si con 3-4 entidades el fan-out se vuelve repetitivo.
- Los resultados de cada `ChangesSince` ya vienen ordenados por `seq`
  (cada uno pide `ORDER BY seq ASC` a su propia tabla); `sync` solo
  hace merge + sort final, válido porque `seq` es un único contador
  por usuario compartido entre todas las entidades
  (`internal/db.NextSeq`), no uno por tabla.

El store SQL de cada dominio (`store.go`) no se extrae de forma
parecida: sus queries son inherentemente específicas de las tablas de
ese dominio (usuarios, tasks, etc.), no hay comportamiento genérico
que valga la pena compartir — a diferencia de hashing/JWT/tokens
opacos, que son la misma operación sin importar quién la llame.

### Convención de archivos dentro de un paquete de dominio

No todo dominio necesita todos estos archivos — solo los que apliquen
— pero cuando aplican, van con este nombre, para que abrir cualquier
paquete de dominio nuevo (`task`, `note`, `sync`...) se sienta igual:

| Archivo | Contenido |
|---|---|
| `models.go` | Structs de las entidades del dominio (típicamente no exportados — ver `internal/auth/models.go`). |
| `store.go` | Acceso SQL: tipo `store` no exportado + `NewStore` exportado (lo necesita `cmd/server` para construirlo), resto de métodos no exportados salvo que otro paquete los necesite. |
| `handlers.go` | Handlers HTTP del dominio + método `Routes(r chi.Router)` que registra sus rutas directamente sobre el router del caller. **No** `Routes() chi.Router` devolviendo un sub-router para montar con `r.Mount("/", ...)` — todos los dominios cuelgan de la raíz (`/auth/login`, `/tasks`, `/devices`...), y `chi.Mount` hace panic si dos routers distintos se montan en el mismo path (`'/'` colisiona). Se descubrió este bug al integrar `task` junto a `auth`. |
| `middleware.go` | Solo si el dominio expone middleware reusable por otros dominios (hoy únicamente `auth`, con `RequireAuth`/`RequireAdmin`). |
| `helpers.go` | Utilidades HTTP privadas del dominio (`writeJSON`, `clientIP`, etc.) — no confundir con `internal/security`, que es para crypto genérica, no HTTP. |
| `bootstrap.go` | Solo si el dominio necesita inicialización especial al arrancar el servidor (hoy únicamente `auth`, para el primer admin). |
| `ratelimit.go`, `tokens.go`, ... | Archivos ad hoc para conceptos propios del dominio que no encajan en lo anterior — no es una lista cerrada. |

La superficie exportada de cada paquete de dominio se mantiene mínima
a propósito: solo lo que otro paquete (`cmd/server` u otro dominio)
realmente necesita llamar. Todo lo demás queda privado, incluso si
"lógicamente" parece que debería ser público — evita el ruido de
`revive` exigiendo comentarios en símbolos que nadie fuera del
paquete usa (ver `internal/auth`: solo `Handler`, `NewHandler`,
`NewStore`, `Bootstrap`, `Routes`, `RequireAuth`, `RequireAdmin` y
`UserIDFromContext` son exportados).

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
