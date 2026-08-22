# LWW por campo — Tareas

Estado: implementado (`feature/lww-por-campo`, no mergeado a `develop`/`main` todavía).

## Decisiones (ya tomadas, ver `requirements.md`/`design.md`)

- [x] Decidir mecanismo de escritura: `PATCH /<entidad>/:id` parcial en
      reemplazo de `PUT` de fila completa; `POST` (creación) no cambia.
- [x] Decidir almacenamiento del timestamp por campo: columna JSON
      `field_updated_at`, no columnas individuales ni tabla aparte.
- [x] Decidir qué responde el servidor cuando parte o todo el `PATCH`
      pierde el LWW: siempre 200 con el estado actual completo, nunca
      409 — el cliente no distingue "aplicado" de "rechazado".
- [x] Decidir cuándo avanza `seq`/se notifica por sync: solo si al
      menos un campo cambió de verdad.
- [x] Decidir alcance de `tags` (campo lista): LWW sobre el array
      completo, sin merge por elemento — sin cambios respecto a lo ya
      definido en `esquema-datos`.
- [x] Decidir riesgo aceptado (mismo campo, escritura concurrente): no
      se resuelve en este spec, sin copia de recuperación — ver
      "Riesgo aceptado" en `requirements.md`.
- [x] Decidir qué tabla no se toca: `daily_entries` (100% CRDT, sin
      campos LWW propios).

## Implementación

### Esquema

- [x] Migración `00018_add_field_updated_at.sql`: agrega
      `field_updated_at TEXT NOT NULL DEFAULT '{}'` a `tasks`, `notes`,
      `overtime_entries`, `overtime_month_meta`, `calendar_events`,
      `absence_days`.

### Helpers compartidos

- [x] `internal/db/patch.go`: `Field[T]`/`ParsePatch`/`PatchField` —
      distingue "campo ausente" de "campo explícitamente `null`" en un
      `PATCH` parcial (necesario para nullables como `task_code`,
      `completed_at`, `due`, `note`).
- [x] `internal/db/fieldts.go`: `FieldTimestamps` — la lógica de LWW
      por campo (`Wins`), independiente de cualquier entidad.

### Servidor — por paquete de dominio

- [x] `internal/task`: `patch` (merge campo por campo) reemplaza
      `update`; ruteo `PATCH /tasks/{id}`.
- [x] `internal/note`: mismo cambio solo en metadata (`title`,
      `folder`, `tags`, `created`, `updated`, `pinned`) —
      `POST /notes/:id/content` (CRDT) no se tocó.
- [x] `internal/overtime`: `overtime_entries` vía `:id`;
      `overtime_month_meta` vía `(user_id, year_month)` — sin `POST`
      propio, así que `PATCH` también crea la fila si `yearMonth` no
      existía (mismo rol que cumplía el `PUT` que reemplaza). Se
      eliminó `upsertMonthMeta` (quedaba muerto, sin caller).
- [x] `internal/calendar`: `overtime_events` → `calendar_events`,
      igual patrón.
- [x] `internal/absence`: `absence_days`, incluye el caso nullable
      (`note`).
- [x] Eliminada la comparación de `updated_at` de fila completa y la
      respuesta 409 en los 5 handlers afectados.
- [x] Ruteo: `r.Put(...)` → `r.Patch(...)` en cada `Routes(r chi.Router)`.

### Contrato público

- [x] `openapi.yaml`: `PATCH` en vez de `PUT` en los 6 endpoints
      afectados, con schema `*Patch` (todos los campos opcionales
      salvo `updated_at`) y sin más menciones a `409` en esos paths.
      Overview general actualizado para explicar la separación
      POST (fila completa, 409) / PATCH (por campo, nunca 409).

### Tests

- [x] Dos campos distintos editados concurrentemente: ambos
      sobreviven (uno por paquete: task, note, overtime entry,
      overtime month-meta, calendar event, absence day).
- [x] Mismo campo editado dos veces: gana el más nuevo, se descarta el
      otro sin error (`changed=false`, sin bump de `seq`).
- [x] `PATCH` íntegramente stale: 200, `seq` sin cambios.
- [x] Fila recién creada acepta cualquier timestamp en el primer
      `PATCH` a un campo (field_updated_at vacío).
- [x] `overtime_month_meta`: crea la fila si no existe, además del
      merge por campo.
- [x] Nullable explícito (`task_code`, `note`): `null` limpia el
      campo, ausente lo deja intacto — cubierto en `internal/db`
      (`TestPatchFieldAbsentVsNull`) y en `task`/`absence`.
- [x] Regresión: suite completa (`go test ./...`) en verde, sin tests
      que asumieran 409 por fila completa quedando rotos.

### Validación

- [x] `go build` / `go test ./...` / `golangci-lint run ./...` en
      verde (0 issues).
- [x] Validación manual end-to-end contra contenedor Docker real
      (`docker compose --env-file .env up -d --build`, imagen
      multi-stage, migraciones + bootstrap de admin corriendo dentro
      del contenedor): dos "dispositivos" (curl) editando `status` y
      `title` de la misma task sin verse — ambos sobreviven, confirmado
      vía `GET /tasks` y `GET /sync/changes`. Un `PATCH` con timestamp
      viejo llegando después responde 200 (no 409) y no pisa el valor
      ganador. `PATCH /overtime-month-meta/:ym` crea la fila en su
      primer uso, sin `POST`. Contenedor y volumen destruidos al
      terminar (`docker compose down -v`).
