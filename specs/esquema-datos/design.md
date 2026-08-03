# Esquema de datos — Diseño

Estado: en diseño

## Convenciones generales

- Todas las tablas de datos de usuario llevan `user_id`, `seq`
  (secuencia monótona por usuario, ver `sync-incremental`),
  `updated_at` (timestamptz) y `deleted_at` (timestamptz nullable,
  soft-delete).
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
| content | TEXT | markdown, LWW por campo (no CRDT) |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

Excluidos: `filePath`, `linked_paths`.

### `notes`

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
| content_crdt | BLOB | estado CRDT (`yrs`), ver `arquitectura-inicial/design.md` |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | bookkeeping de sync |
| deleted_at | TIMESTAMPTZ NULL | |

Excluido: `filePath`.

### `overtime_entries`

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

Metadata por mes (`colaborador`, `cédula`), no por entrada — hoy no
tiene id propio en el tipo TS, así que se usa `(user_id, year_month)`
como clave natural.

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

| Columna | Tipo | Notas |
|---|---|---|
| id | TEXT (UUID) | PK |
| user_id | TEXT | FK |
| title | TEXT | |
| date | DATE | |
| time | TEXT (HH:MM) NULL | vacío/NULL = todo el día |
| description | TEXT | |
| color | TEXT CHECK IN ('indigo','amber','emerald','rose','sky','violet') | |
| reminder_minutes | INTEGER | 0 = sin recordatorio |
| repeat | TEXT CHECK IN ('none','daily','weekly','biweekly','monthly','yearly') | |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

### `absence_days`

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

Hoy en el cliente es `Record<date,string>` (un archivo `.md` por mes,
sin campos extra más allá de fecha + texto libre — confirmado leyendo
`appStore.ts` y `dailyFileFormat.ts`). Clave natural `(user_id, date)`,
sin id propio.

| Columna | Tipo | Notas |
|---|---|---|
| user_id | TEXT | FK, parte de PK compuesta |
| date | DATE | parte de PK compuesta; actúa como `id` sintético en el endpoint de cambios |
| content_crdt | BLOB | estado CRDT (`yrs`) |
| seq | INTEGER | |
| updated_at | TIMESTAMPTZ | |
| deleted_at | TIMESTAMPTZ NULL | |

## Explícitamente pendiente

- Migraciones concretas (`golang-migrate`/`goose`, ya decidido en
  `arquitectura-inicial`, falta escribirlas).
- Índices (mínimo esperable: `(user_id, seq)` en cada tabla para el
  endpoint de cambios, pero no se define aquí el detalle final).
- Formato exacto del payload CRDT dentro de `content_crdt` y de cómo
  viaja en el endpoint de cambios — ver `sync-incremental/design.md`.
