# Esquema de datos — Diseño

Estado: implementado — las 7 tablas existen y tienen su paquete de
dominio (`internal/task/`, `internal/note/`, `internal/overtime/`,
`internal/calendar/`, `internal/absence/`, `internal/dailyentry/`).
Índices más allá de `(user_id, seq)` siguen sin definirse (no
bloqueante). `notes.content` y `daily_entries.content` usan la
simplificación LWW por fila en vez de CRDT — ver
`arquitectura-inicial/requirements.md`.

## Convenciones generales

- Todas las tablas de datos de usuario llevan `user_id`, `seq`
  (secuencia monótona por usuario, ver `sync-incremental`),
  `updated_at` (timestamptz) y `deleted_at` (timestamptz nullable,
  soft-delete). Un solo `updated_at` por fila implica LWW por **fila
  completa** en v1, no por campo individual — ver
  `arquitectura-inicial/requirements.md` ("Resolución de conflictos")
  para el alcance real aceptado y lo que queda aspiracional.
- IDs generados por el cliente (UUID), nunca autoincrement del
  servidor — requisito de `arquitectura-inicial`.
- Enums (`status`, `type`, `color`, `repeat`) se guardan como `TEXT` +
  `CHECK` en vez de tipos enum nativos, para que el esquema sea
  portable entre SQLite y Postgres sin diverger (decisión ya tomada:
  SQLite por defecto, Postgres intercambiable).
- Campos tipo lista simples (`tags`) se guardan como `TEXT` con JSON
  serializado, no como tabla normalizada — consistente con la decisión
  de mantener `project`/`folder` como string plano en vez de entidad
  propia: no se introduce normalización que el cliente actual tampoco
  tiene. Conflictos en `tags` se resuelven LWW sobre el array completo
  (el campo, no cada elemento) — riesgo aceptado porque es una lista
  corta editada con poca frecuencia, no texto en edición continua.
- Campos excluidos del sync por ser rutas de filesystem local:
  `Task.filePath`, `Task.linked_paths`, `Note.filePath`.

## Tablas

### `tasks`

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK, client-generated |
| user_id | TEXT | FK |
| title | TEXT | |
| task_code | TEXT NULL | |
| status | TEXT CHECK IN ('todo','in-progress','done') | |
| tags | TEXT (JSON array) | |
| project | TEXT | string plano, ver `requirements.md` |
| created | DATE | |
| completed_at | DATE NULL | |
| due | DATE NULL | |
| content | TEXT | markdown, LWW por fila completa (ver limitación v1) |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

Excluidos: `filePath`, `linked_paths`. Implementada — ver
`internal/task/` y
`internal/db/migrations/00005_create_tasks.sql`.

### `notes`

Implementada — ver `internal/note/` y
`internal/db/migrations/00006_create_notes.sql`. **Con una desviación
deliberada del diseño original**: `content` es `TEXT` plano (LWW por
fila completa, igual que `tasks.content`), no `content_crdt BLOB`
todavía — la integración `yrs`/CGO decidida en `arquitectura-inicial`
no se construyó (requiere escribir bindings CGO/Rust a mano contra una
API no verificada, evaluado como riesgo demasiado alto para resolver
de paso). Queda como tarea de seguimiento explícita — ver
`arquitectura-inicial/tasks.md`. Migrar `content` → `content_crdt`
cuando se aborde esa tarea.

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK |
| user_id | TEXT | FK |
| title | TEXT | |
| folder | TEXT | string plano |
| tags | TEXT (JSON array) | |
| created | DATE | |
| updated | DATE | fecha de negocio (la que ve el usuario), distinta de `updated_at` |
| pinned | BOOLEAN | |
| content | TEXT | **interino**: LWW por fila, no CRDT — ver nota arriba |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | bookkeeping de sync, mandado por el cliente (ver `sync-incremental`) |
| deleted_at | TIMESTAMPTZ NULL | |

Excluido: `filePath`.

### `overtime_entries`

Implementada — ver `internal/overtime/` (junto con `overtime_month_meta`,
mismo paquete de dominio: son el mismo concepto de negocio repartido
en dos tablas) y
`internal/db/migrations/00007_create_overtime_entries.sql`.

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK |
| user_id | TEXT | FK |
| fecha | DATE | |
| solicitada_por | TEXT | |
| actividad | TEXT | |
| observaciones | TEXT | |
| hora_inicio | TEXT (HH:MM) | |
| hora_final | TEXT (HH:MM) | |
| total_horas | REAL | |
| extras_diurnas | REAL | |
| extras_nocturnas | REAL | |
| extras_diurnas_festivas | REAL | |
| extras_nocturnas_festivas | REAL | |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

### `overtime_month_meta`

Implementada — ver `internal/overtime/` y
`internal/db/migrations/00008_create_overtime_month_meta.sql`.
Metadata por mes (`colaborador`, `cédula`), no por entrada — hoy no
tiene id propio en el tipo TS, así que se usa `(user_id, year_month)`
como clave natural. `year_month` es el URL param en REST
(`PUT /overtime-month-meta/:yearMonth`, sin `POST` — a diferencia de
las entidades con id propio, el recurso ya se identifica por su clave
natural desde el inicio, así que upsert-por-URL alcanza) y el `id`
sintético en `/sync/changes`.

| Columna | Tipo | Notas |
|---|---|---|
| user_id | TEXT | FK, parte de PK compuesta |
| year_month | TEXT ('YYYY-MM') | parte de PK compuesta; también actúa como `id` sintético en el endpoint de cambios |
| colaborador | TEXT | |
| cedula | TEXT | |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

### `calendar_events`

Implementada — ver `internal/calendar/` y
`internal/db/migrations/00009_create_calendar_events.sql`. `time` es
`TEXT NOT NULL DEFAULT ''` (no `NULL`) — `''` significa "todo el día",
igual que el tipo TS de referencia (`time: string`, no opcional).

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK |
| user_id | TEXT | FK |
| title | TEXT | |
| date | DATE | |
| time | TEXT | `''` = todo el día |
| description | TEXT | |
| color | TEXT CHECK IN ('indigo','amber','emerald','rose','sky','violet') | |
| reminder_minutes | INTEGER | 0 = sin recordatorio |
| repeat | TEXT CHECK IN ('none','daily','weekly','biweekly','monthly','yearly') | |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

### `absence_days`

Implementada — ver `internal/absence/` y
`internal/db/migrations/00010_create_absence_days.sql`.

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK |
| user_id | TEXT | FK |
| date | DATE | |
| type | TEXT CHECK IN ('incapacidad','vacaciones','otro') | |
| note | TEXT NULL | |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

### `daily_entries`

Implementada — ver `internal/dailyentry/` y
`internal/db/migrations/00011_create_daily_entries.sql`. Misma
desviación deliberada que `notes`: `content` es `TEXT` plano (LWW por
fila), no `content_crdt BLOB` todavía — ver nota en `notes` arriba y
`arquitectura-inicial/tasks.md` para el seguimiento de CRDT. Clave
natural `(user_id, date)`, sin id propio — mismo patrón de rutas que
`overtime_month_meta` (`PUT /daily-entries/:date`, sin `POST`).

| Columna | Tipo | Notas |
|---|---|---|
| user_id | TEXT | FK, parte de PK compuesta |
| date | DATE | parte de PK compuesta; actúa como `id` sintético en el endpoint de cambios |
| content | TEXT | **interino**: LWW por fila, no CRDT |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

## Explícitamente pendiente

- Índices más allá de `(user_id, seq)` (ya presente en las 7 tablas)
  — no se ha identificado ningún patrón de consulta que lo necesite
  todavía.
- Formato exacto del payload CRDT dentro de `content`/`content_crdt` y
  de cómo viaja en el endpoint de cambios, para cuando se resuelva el
  seguimiento de CRDT — ver `sync-incremental/design.md`.
