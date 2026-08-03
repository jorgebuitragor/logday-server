# Esquema de datos — Tareas

Estado: en diseño

- [x] Confirmar campos reales de cada entidad leyendo `task-manager`
      directamente (no asumir el resumen viejo de `arquitectura-inicial`).
- [x] Decidir qué hacer con campos de rutas locales (`filePath`,
      `linked_paths`) — excluidos del sync.
- [x] Decidir si `project`/`folder` pasan a ser entidad propia — se
      quedan como string (v1).
- [x] Decidir cómo modelar `OvertimeMonthMeta` — tabla propia keyed por
      `(user_id, year_month)`.
- [x] Definir las 7 tablas completas, columna por columna.
- [ ] Definir índices concretos para las 6 tablas de dominio restantes
      (notes, overtime_entries, overtime_month_meta, calendar_events,
      absence_days, daily_entries) — `tasks` ya tiene
      `idx_tasks_user_id_seq`, ver migración.
- [x] Decidir herramienta de migraciones: `goose` (API embebible vía
      `embed.FS`, corre automáticamente al arrancar el binario — ver
      `specs/convenciones-codigo/`). Las migraciones de `users`/
      `devices`/`used_refresh_tokens` (auth-multiusuario) y de `tasks`
      + `user_sync_counters` (sync) ya están escritas en
      `internal/db/migrations/`; las 6 tablas de dominio restantes de
      este spec, no todavía.
- [x] Implementar `tasks` completo: `internal/task/` (`POST /tasks`,
      `PUT /tasks/:id`, `DELETE /tasks/:id`, `GET /tasks`), primera
      entidad de negocio real, validada end-to-end contra un
      contenedor (crear, listar, LWW por fila con 409 en escritura
      obsoleta, borrar, aislamiento entre usuarios).
