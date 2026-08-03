# Sync incremental — Diseño

Estado: en diseño

## Cursor: secuencia monótona por usuario

- Cada fila relevante (`tasks`, `notes`, `overtime_entries`,
  `calendar_events`, `absence_days`, `daily_entries`) tiene una columna
  `seq` asignada por el servidor en cada INSERT/UPDATE/soft-delete,
  tomada de un contador monótono por `user_id` (p. ej. una tabla
  `user_sync_counters(user_id, next_seq)` con incremento atómico).
- El cliente nunca genera ni interpreta el `seq` — es opaco, solo lo
  guarda y lo reenvía como cursor.

## Escritura: REST por entidad, no push genérico

Cada dominio expone sus propios endpoints siguiendo el mismo patrón
(primer caso real: `internal/task`):

- `POST /tasks` — crea o actualiza (upsert por `id`, generado por el
  cliente). Body = la fila completa, incluyendo el `updated_at` del
  cliente (su hora local de edición).
- `PUT /tasks/:id` — igual que arriba, editando una fila existente.
- `DELETE /tasks/:id` — soft-delete (setea `deleted_at`).

En los tres casos:

- El servidor asigna `seq` (vía `db.NextSeq`, incremento atómico en
  `user_sync_counters`, en la misma transacción que el write).
- El servidor usa el `updated_at` que manda el cliente (no estampa uno
  propio al recibir) — es lo que hace que la comparación LWW compare
  algo real (hora de edición) en vez de hora de llegada al servidor.
  Si el `updated_at` entrante no es más reciente que el ya guardado
  para esa fila, el servidor responde `409 Conflict` y descarta la
  escritura entera (LWW por fila completa — ver limitación conocida en
  `arquitectura-inicial/requirements.md`).
- Se descartó un endpoint genérico `POST /sync/changes` simétrico al
  pull: sería prematuro/especulativo antes de tener 2+ entidades
  reales que lo necesiten, y hoy no aporta nada sobre REST plano por
  entidad — se reconsiderará si en la práctica escribir entidad por
  entidad resulta costoso (p. ej. un cliente que necesita mandar 50
  cambios tras reconectar).

## Endpoint unificado

`GET /sync/changes?since=<seq>` devuelve un array de cambios mezclados
entre todas las entidades del usuario autenticado, cada uno con al
menos:

```json
{ "type": "task", "seq": 1042, "id": "...", "deleted": false, "updated_at": "...", "data": { ... } }
```

Ordenados por `seq` ascendente. El cliente aplica cada uno en orden y
actualiza su cursor local al `seq` del último elemento procesado.

## Tombstones y purga

- Soft-delete: se setea `deleted_at`, la fila no se borra hasta la
  purga.
- Un job periódico elimina físicamente los tombstones con `deleted_at`
  de más de 90 días.
- El servidor mantiene el `seq` mínimo vigente (el del tombstone no
  purgado más antiguo). Si `since` es menor a ese mínimo, responde
  `410 Gone` con un cuerpo indicando "cursor expirado, hacer full
  resync".
- Full resync = mismo endpoint sin `since` (o `since=0`): trae todo el
  estado actual no borrado.

## Eventos WebSocket

- **Topic**: uno por `user_id` — cada dispositivo conectado se
  suscribe al canal de su propio usuario y recibe ahí los avisos de
  cambios de cualquier entidad (mismo modelo que el cursor único y el
  endpoint unificado, no hay topics separados por tipo de entidad).
- **Payload**: aviso liviano, no el registro completo:
  ```json
  { "type": "task", "id": "...", "seq": 1043 }
  ```
  El cliente que recibe el aviso dispara el mismo
  `GET /sync/changes?since=<su cursor>` que usaría al reconectar — un
  solo mecanismo de reconciliación (arriba). El WebSocket no lleva
  nunca el dato real, solo baja la latencia de cuándo el cliente se
  entera de que hay algo que traer.
- Se descarta mandar el payload completo por WS: duplicaría la lógica
  de "aplicar un cambio" (una vez desde WS, otra desde el pull), y el
  pull completo de todas formas es obligatorio como fallback porque un
  cliente puede perder mensajes WS mientras estuvo desconectado.

## Explícitamente pendiente

- Paginación de `/sync/changes` (probablemente `limit` + el `seq` del
  último elemento como cursor de página, reutilizando el mismo campo).
- Cómo viajan los updates CRDT de `yrs` dentro del campo `data` de un
  cambio de tipo `note`/`daily_entry` (payload binario vs. snapshot
  resuelto) — ver decisión de CRDT en
  [`arquitectura-inicial/design.md`](../arquitectura-inicial/design.md).
- Reconexión/heartbeat del WebSocket (ping/pong, backoff de
  reconexión).
