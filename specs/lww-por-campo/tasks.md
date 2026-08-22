# LWW por campo — Tareas

Estado: en diseño — nada implementado todavía.

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

- [ ] Migración: agregar `field_updated_at TEXT NOT NULL DEFAULT '{}'`
      a `tasks`, `notes`, `overtime_entries`, `overtime_month_meta`,
      `calendar_events`, `absence_days` (las 6 tablas con campos LWW).

### Servidor — por paquete de dominio

- [ ] `internal/task`: reemplazar `update` (PUT fila completa) por
      `patch` (merge campo por campo contra `field_updated_at`);
      actualizar `store.go` y `handlers.go`.
- [ ] `internal/note`: aplicar el mismo cambio solo a los campos de
      metadata (`title`, `folder`, `tags`, `pinned`) — el endpoint de
      `content_crdt` (`POST /notes/:id/content`) no se toca, sigue
      siendo un camino separado.
- [ ] `internal/overtime`: aplicar a `overtime_entries` (vía `:id`) y a
      `overtime_month_meta` (vía `(user_id, year_month)`, sin `id`
      propio — confirmar que el merge por campo funciona igual con
      clave natural en vez de UUID).
- [ ] `internal/calendar`: aplicar a `calendar_events`.
- [ ] `internal/absence`: aplicar a `absence_days`.
- [ ] Eliminar la comparación de `updated_at` de fila completa y la
      respuesta 409 en los handlers de estos paquetes (queda obsoleta,
      ver `requirements.md`).
- [ ] Ruteo: cambiar `r.Put(...)` por `r.Patch(...)` en cada
      `Routes(r chi.Router)` afectado.

### Contrato público

- [ ] Actualizar `openapi.yaml`: método `PATCH` en vez de `PUT` para
      estos endpoints, request body parcial (todos los campos
      opcionales salvo `updated_at`), sin más menciones a 409 en estos
      paths.

### Tests

- [ ] Dos campos distintos editados concurrentemente (mismo `id`,
      timestamps distintos, ninguno subsume al otro): ambos
      sobreviven.
- [ ] Mismo campo editado dos veces con distinto `updated_at`: gana el
      más nuevo, se descarta el otro sin error.
- [ ] `PATCH` donde todos los campos pierden el LWW: responde 200 con
      estado actual, `seq` no avanza, no hay evento de sync.
- [ ] Fila recién creada (`field_updated_at` vacío tras `POST`): el
      primer `PATCH` a cualquier campo se aplica sin comparación.
- [ ] `overtime_month_meta` (clave natural, sin `id`): merge por campo
      funciona igual que en las entidades con UUID.
- [ ] Regresión: los tests existentes que asumían 409 por fila completa
      se actualizan o eliminan según corresponda.

### Validación

- [ ] `go build` / `go test ./...` / `golangci-lint run` en verde.
- [ ] Validación manual end-to-end contra contenedor Docker real: dos
      "dispositivos" (dos secuencias curl) editando campos distintos
      de la misma tarea sin verse, confirmar que ambos cambios quedan
      en la fila final vía `GET /sync/changes`.
