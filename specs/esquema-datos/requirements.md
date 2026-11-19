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

### Campos que terminan en rutas (U-06, 19 nov 2026)

Desktop guarda cada registro como archivo y arma la ruta con el `id` de
la entidad, `Task.project`, `Note.folder` y, para dailys y horas extra,
el año y el mes cortados con slices fijos de `daily_entries.date` y
`overtime_entries.fecha` (y de `overtime_month_meta.year_month`).

- Cuando un cliente escriba un `id`, un `Task.project`, una
  `overtime_entries.fecha` o use `{date}` o `{yearMonth}` en la URL, el
  sistema DEBERÁ responder `400` si el valor está vacío (salvo
  `Task.project`, que puede ir vacío), contiene `/` o `\`, es `.` o
  `..`, tiene caracteres de control, no es UTF-8 válido o supera su
  largo máximo (128 bytes el `id`, 200 `project`, 64 las fechas).
- Cuando un cliente escriba `Note.folder`, el sistema DEBERÁ aceptar el
  valor vacío o segmentos separados por `/`, y DEBERÁ responder `400` si
  algún segmento está vacío o incumple la regla anterior, o si el total
  supera 400 bytes.
- Un parámetro de la URL DEBERÁ cumplir la regla también decodificado
  (`%2F`, `%5C`).
- El sistema NO DEBERÁ exigir UUID en `id` ni `YYYY-MM-DD` en las fechas.

Por qué: un valor con separadores que llega por el sync escribía fuera
de la carpeta de datos en todos los Desktop de la cuenta (U-06 de la
auditoría de nov 2026). Desktop ya acota sus rutas desde logday#72, pero
Mobile y los Desktop anteriores dependen del server. No se exige formato
porque hay `id` legados (los tests usan `task-1`) y fechas legadas en
colas offline (ver la decisión siguiente): sin `/` ni `\` ninguna puede
salir de la carpeta de datos con los slices fijos de Desktop. Validado en
`internal/pathsafe`.

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
  contenido. La única excepción son los caracteres que forman una ruta de
  archivo (ver "Campos que terminan en rutas").
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

### Progreso de tareas (decisión del 29 oct 2026)

`Task.progress` es un porcentaje opcional: entero de 0 a 100, o `null`
= sin progreso. `null` no equivale a `0`: "sin progreso" es la tarea
que no lleva porcentaje, `0` es la que lo lleva y va en cero.

- El sistema DEBERÁ aceptar `progress` en `POST /tasks` y en
  `PATCH /tasks/{id}` solo como entero JSON entre 0 y 100, o `null`.
- Si `progress` llega fuera de rango o no es un entero JSON (`-1`,
  `101`, `40.5`, `40.0`, `1e2`, `"40"`), el sistema DEBERÁ responder
  `400` con el mensaje `progress must be an integer between 0 and
  100, or null`, sin escribir nada.
- En `PATCH`, `progress` DEBERÁ resolverse por LWW de campo como el
  resto (clave `progress` en `field_updated_at`): `null` lo borra y la
  clave ausente no lo toca.
- En `POST`, `null` DEBERÁ dejar la tarea sin progreso. Si la clave
  falta, una tarea nueva DEBERÁ quedar sin progreso y una existente
  DEBERÁ conservar el que tenía.
- El sistema DEBERÁ incluir `progress` en toda respuesta que serialice
  una tarea (`GET /tasks`, `POST`, `PATCH`, `data` de `/sync/changes`,
  y por tanto el resync completo), siempre con la clave presente:
  `null` explícito cuando no hay progreso.
- Un cliente DEBERÁ tratar la clave ausente igual que `null` al leer,
  porque un servidor anterior a este cambio no la manda.

Por qué: la extensión de OpenProject de Desktop necesita mostrar el
`percentageDone` de un work package, y otros orígenes pueden usarlo
igual. Que el `POST` sin la clave conserve el valor existente protege a
los clientes ya publicados, que no conocen el campo: si reenvían una
tarea entera no deben borrar el progreso que puso otro dispositivo. La
clave siempre presente en la respuesta (a diferencia de `task_code`,
`completed_at` y `due`, que se omiten si son `null`) deja explícito que
el servidor conoce el campo y que la tarea no tiene progreso.

## Fuera de este spec

- Migración/backfill de datos existentes desde los archivos locales de
  `task-manager` hacia estas tablas.
- Índices y optimizaciones de consulta — se definen en implementación.
- Sincronización de `linked_paths`/`filePath` (explícitamente excluida,
  no diferida).
