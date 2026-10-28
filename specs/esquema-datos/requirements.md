# Esquema de datos — Requirements

Estado: implementado — ver `design.md` y `tasks.md`.

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

### Formato de fechas e instantes (decisión del 28 oct 2026)

Campos de instante: `Task.created`, `Task.completed_at`, `Note.created`
y `Note.updated`. Campos de fecha de calendario: `Task.due`,
`overtime_entries.fecha`, `daily_entries.date`, `absence_days.date` y
`calendar_events.date`.

- Cuando un cliente escriba un campo de instante, el cliente DEBERÁ
  enviarlo como instante UTC con el formato de `toISOString()`
  (`2026-10-28T19:30:00.000Z`).
- El sistema DEBERÁ aceptar en un campo de instante tanto un instante
  como un `YYYY-MM-DD` legado, y NO DEBERÁ migrar ni reescribir los
  valores `YYYY-MM-DD` ya guardados.
- El sistema DEBERÁ guardar y devolver los campos de instante y de
  fecha de calendario como el mismo string recibido, sin validar su
  formato, normalizarlo, ordenarlo ni compararlo. LWW no mira su
  contenido.
- Cuando un cliente lea un campo de instante con un `YYYY-MM-DD`
  legado, el cliente DEBERÁ interpretarlo como esa fecha de calendario
  en su zona local, nunca como medianoche UTC.
- Los campos de fecha de calendario DEBERÁN seguir en `YYYY-MM-DD`
  local, sin hora ni zona.
- Si `Task.completed_at` es `null`, el sistema DEBERÁ omitir la clave en
  la respuesta.
- Si una nota antigua no tiene `updated`, el sistema DEBERÁ devolverlo
  como string vacío.
- `openapi.yaml` NO DEBERÁ declarar `format: date-time` en los campos de
  instante, porque los valores legados lo incumplirían.

Por qué: hasta esta decisión Desktop y Web escribían `YYYY-MM-DD` y
Mobile instantes en los mismos campos, y la mezcla causaba fallos en los
clientes (días corridos por la zona horaria, orden por fecha incorrecto).
El server no valida el formato para no rechazar datos legados que
todavía vienen en colas offline o en frontmatter editado a mano. Detalle
en el vault: `02 Decisiones/Fechas de creación y edición como instantes
ISO.md`.

## Fuera de este spec

- Migración/backfill de datos existentes desde los archivos locales de
  `task-manager` hacia estas tablas.
- Índices y optimizaciones de consulta — se definen en implementación.
- Sincronización de `linked_paths`/`filePath` (explícitamente excluida,
  no diferida).
