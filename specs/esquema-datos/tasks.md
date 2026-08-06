# Esquema de datos — Tareas

Estado: implementado — las 7 tablas tienen paquete de dominio y están
validadas end-to-end.

- [x] Confirmar campos reales de cada entidad leyendo `task-manager`
      directamente (no asumir el resumen viejo de `arquitectura-inicial`).
- [x] Decidir qué hacer con campos de rutas locales (`filePath`,
      `linked_paths`) — excluidos del sync.
- [x] Decidir si `project`/`folder` pasan a ser entidad propia — se
      quedan como string (v1).
- [x] Decidir cómo modelar `OvertimeMonthMeta` — tabla propia keyed por
      `(user_id, year_month)`.
- [x] Definir las 7 tablas completas, columna por columna.
- [x] Definir índices concretos para las 7 tablas — todas tienen
      `idx_<tabla>_user_id_seq`, ver migraciones en
      `internal/db/migrations/`. Ningún otro índice identificado como
      necesario todavía.
- [x] Decidir herramienta de migraciones: `goose` (API embebible vía
      `embed.FS`, corre automáticamente al arrancar el binario — ver
      `specs/convenciones-codigo/`). Las 11 migraciones (auth, sync,
      7 tablas de dominio) están escritas en `internal/db/migrations/`.
- [x] Implementar `tasks` completo: `internal/task/` (`POST /tasks`,
      `PUT /tasks/:id`, `DELETE /tasks/:id`, `GET /tasks`), primera
      entidad de negocio real, validada end-to-end contra un
      contenedor (crear, listar, LWW por fila con 409 en escritura
      obsoleta, borrar, aislamiento entre usuarios).
- [x] Implementar `notes` completo: `internal/note/` (mismo patrón que
      `tasks`), con `content` como `TEXT` plano (LWW por fila) en vez
      de `content_crdt` — ver desviación documentada en `design.md`.
      Sumado al fan-out de `internal/sync`; validado end-to-end junto
      con `tasks` en el mismo `/sync/changes` (orden global por `seq`
      compartido entre ambas entidades).
- [x] Implementar las 5 entidades restantes, mismo patrón:
  - `internal/overtime/` — `overtime_entries` (id propio, igual que
    `tasks`) y `overtime_month_meta` (clave natural `(user_id,
    year_month)`, sin `POST`, solo `PUT /overtime-month-meta/:yearMonth`)
    en el mismo paquete de dominio.
  - `internal/calendar/` — `calendar_events`, con validación de
    `color`/`repeat` contra los valores permitidos.
  - `internal/absence/` — `absence_days`, con validación de `type`.
  - `internal/dailyentry/` — `daily_entries`, clave natural
    `(user_id, date)`, `content` interino (LWW por fila, no CRDT —
    misma desviación que `notes`).

  Las 5 sumadas al fan-out de `internal/sync` (7 entidades en total),
  con un helper genérico `addChanges` para reducir la duplicación que
  ya no valía la pena copiar bloque por bloque (ver
  `convenciones-codigo/design.md`). Validado end-to-end contra un
  contenedor: cada endpoint REST, `GET /sync/changes` mezclando las 7
  entidades en orden global por `seq`, `id` sintético correcto para
  las dos entidades de clave compuesta, tombstone tras `DELETE` en una
  entidad de clave compuesta (`daily_entries`), 401 sin auth.
