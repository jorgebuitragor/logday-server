# AGENTS.md — logday-server

Backend en Go de Logday: API de sync, panel de administración y Logday Web embebido en `/app`. Es la fuente de verdad del protocolo de sync y del modelo de datos que consumen los tres clientes, que viven como repos hermanos en `../`: `task-manager` (Desktop), `logday-web` y `logday-mobile`.

## Comandos

- `make test` (`go test ./...`), `make lint` (golangci-lint v2), `make build`, `make run`.
- CI (`.github/workflows/ci.yml`): golangci-lint, `go build ./...` y `go test ./...` con Go 1.25.
- Imagen Docker: necesita `../logday-web` clonado al lado; el build lo compila y lo embebe (`--build-context logday-web=../logday-web`). Para producción, `--platform linux/amd64`: el servidor es x86_64 y en un Mac la imagen sale ARM por defecto.

## Contrato con los clientes

- `openapi.yaml` es el contrato de la API (auth, CRUD por recurso, `/sync/changes`, `/ws`). Un cambio de endpoint, de campo o de semántica de sync se refleja ahí en el mismo PR, y se avisa: afecta a los tres clientes.
- Todas las entidades llevan `user_id`, `seq`, `updated_at` y `deleted_at` (borrado lógico), con IDs UUID generados en el cliente. Los conflictos se resuelven con LWW por campo; `Note.content` y `daily_entries.content` son CRDT (Yjs vía `ygo`).
- Migraciones goose embebidas en `internal/db/migrations/`: nunca editar una ya publicada, agregar una nueva.

## Convenciones propias

- Paquetes verticales por dominio en `internal/<dominio>/` (`models.go`, `store.go`, `handlers.go`…). `handlers.go` expone `Routes(r chi.Router)`, nunca `Routes() chi.Router`: `chi.Mount` hace panic si dos routers se montan en el mismo path.
- `internal/realtime` se inyecta en cada dominio, no se importa, para evitar ciclos.
- Errores: `fmt.Errorf("<acción>: %w", err)` para infraestructura; centinelas sin exportar comparados con `errors.Is`; todo 500 pasa por `httperr.Internal(w, r, err)`, que loguea y responde un error genérico.
- Logging con el paquete `log` estándar, sin `slog`.
- Textos para el usuario (panel admin, mensajes) en español con tú o usted, nunca voseo.
- Specs SDD en `specs/` (ver `specs/README.md`): el spec se actualiza en el mismo PR que el código.
- Ramas: Gitflow (`CONTRIBUTING.md`). El trabajo va en `feature/<slug>` desde `develop`, con PR a `develop`.

## No hacer sin preguntar a Jorge

- Mergear a `develop` o `main`, crear tags o cortar releases.
- Desplegar a producción (Gaia, vía Coolify) o tocar su API. Nunca llamadas `DELETE` a la API de Coolify.
- Cambiar el contrato de sync de forma incompatible con clientes ya publicados.

## Dónde está el contexto

Decisiones y estado del ecosistema en el vault de Obsidian `~/Developer/Obsidean/logday`, si está disponible: hub `00 Indice/logday-server (hub).md`.
