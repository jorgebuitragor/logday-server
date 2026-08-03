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
- [ ] Definir índices concretos.
- [ ] Escribir las migraciones (`golang-migrate`/`goose`).
