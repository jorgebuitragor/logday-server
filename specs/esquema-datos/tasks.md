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
- [ ] Definir índices concretos para las 7 tablas de dominio (tasks,
      notes, overtime_entries, overtime_month_meta, calendar_events,
      absence_days, daily_entries) — aún no implementadas en código,
      solo diseñadas.
- [x] Decidir herramienta de migraciones: `goose` (API embebible vía
      `embed.FS`, corre automáticamente al arrancar el binario — ver
      `specs/convenciones-codigo/`). Las migraciones de `users`/
      `devices`/`used_refresh_tokens` (auth-multiusuario) ya están
      escritas en `internal/db/migrations/`; las 7 tablas de dominio
      de este spec, no todavía.
