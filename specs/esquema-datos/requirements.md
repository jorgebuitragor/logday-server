# Esquema de datos — Requirements

Estado: en diseño

## Contexto

Depende de [`arquitectura-inicial`](../arquitectura-inicial/requirements.md)
(resolución de conflictos: LWW por campo + CRDT acotado a texto largo)
y de [`sync-incremental`](../sync-incremental/requirements.md) (cada
tabla necesita `seq`, `updated_at`, `deleted_at`).

Fuente de verdad de los campos: tipos TS actuales en el repo de
referencia `task-manager` (`src/types/*.ts` y, para `daily_entries`,
`src/store/appStore.ts` + `src/lib/dailyFileFormat.ts`, leídos
directamente en 2026-08-02 — no asumir que este resumen sigue vigente
si pasa mucho tiempo).

## Requisitos (EARS)

### Aislamiento y sync (aplica a toda tabla de datos de usuario)

- Cada tabla DEBERÁ incluir `user_id`, `seq`, `updated_at` y
  `deleted_at` como mínimo, sin excepción.
- El sistema NO DEBERÁ sincronizar campos que representen rutas de
  archivo del filesystem local de un dispositivo (`Task.filePath`,
  `Task.linked_paths`, `Note.filePath`) — permanecen como estado local
  de cada cliente.

### Resolución de conflictos por campo

- El sistema DEBERÁ aplicar CRDT únicamente a `Note.content` y
  `daily_entries.content` (ver `arquitectura-inicial/design.md`).
- Todo otro campo editable DEBERÁ resolverse por last-write-wins a
  nivel de ese campo individual, incluyendo campos de tipo lista
  (p. ej. `tags`) — no hay merge estructurado de listas en v1.

### Projects/Folders

- El sistema DEBERÁ tratar `Task.project` y `Note.folder` como texto
  plano, sin tabla ni entidad propia, reflejando el modelo actual del
  cliente.

## Fuera de este spec

- Migración/backfill de datos existentes desde los archivos locales de
  `task-manager` hacia estas tablas.
- Índices y optimizaciones de consulta — se definen en implementación.
- Sincronización de `linked_paths`/`filePath` (explícitamente excluida,
  no diferida).
