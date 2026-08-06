# Sync incremental — Diseño

Estado: en progreso — `GET /sync/changes` y el WebSocket de tiempo
real implementados (ver `internal/sync/`, `internal/realtime/`); CRDT
sigue sin construir.

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

`GET /sync/changes?since=<seq>` (implementado en `internal/sync/`)
devuelve un array de cambios mezclados entre todas las entidades del
usuario autenticado, cada uno con al menos:

```json
{ "type": "task", "seq": 1042, "id": "...", "deleted": false, "updated_at": "...", "data": { ... } }
```

Ordenados por `seq` ascendente. El cliente aplica cada uno en orden y
actualiza su cursor local al `seq` del último elemento procesado.
`since` es opcional — sin él (o `since=0`) trae todo el estado actual.

**Arquitectura de `internal/sync`**: no tiene tabla propia. Su
`store.go` hace fan-out a una función exportada por cada dominio
sincronizable — las 7 entidades implementadas (`task`, `note`,
`overtime` entries + month-meta, `calendar`, `absence`,
`dailyentry`) — vía un helper genérico `addChanges` (ver
`convenciones-codigo/design.md`), y mezcla+ordena los resultados por
`seq` — válido porque `seq` es un único contador por usuario
compartido entre todas las entidades (ver
arriba), no uno por tabla, así que los sub-resultados ya vienen
ordenados y solo hace falta un merge. Agregar una entidad nueva
(`note`, `overtime_entries`, ...) es agregar un bloque más en ese
fan-out, no rediseñar el endpoint.

`task.ChangesSince` es una función de paquete (no un método del
`store` de `task`, que es privado) que toma `*sql.DB` directamente —
así `internal/sync` no necesita nombrar el tipo privado `task.store`,
solo pasar el `*sql.DB` que ya tiene. Ver
`convenciones-codigo/design.md` para el razonamiento completo de por
qué se eligió este patrón en vez de una interfaz `Source` genérica
(prematuro con una sola entidad real).

## Tombstones y purga

- Soft-delete: se setea `deleted_at`, la fila no se borra hasta la
  purga. Ya expuesto en `GET /sync/changes` (`deleted: true`),
  validado end-to-end.
- Un job periódico elimina físicamente los tombstones con `deleted_at`
  de más de 90 días — **no implementado todavía**.
- El servidor DEBERÍA mantener el `seq` mínimo vigente (el del
  tombstone no purgado más antiguo) y responder `410 Gone` si `since`
  es menor a ese mínimo — **no implementado todavía**, a propósito:
  sin el job de purga, un cursor nunca puede quedar realmente
  inválido, así que construir el chequeo ahora sería código sin forma
  de probarse honestamente. Se construye junto con la purga.
- Full resync = mismo endpoint sin `since` (o `since=0`): trae todo el
  estado actual no borrado.

## Eventos WebSocket

Implementado en `internal/realtime/` (`GET /ws`).

- **Librería**: [`coder/websocket`](https://github.com/coder/websocket)
  (antes `nhooyr.io/websocket`, transferida a Coder) — API idiomática
  con `context.Context`, sin dependencias más allá de la stdlib.
- **Topic**: uno por `user_id` — `Hub` (`internal/realtime/hub.go`)
  mantiene `map[user_id]set[*client]` protegido por mutex, sin topics
  separados por tipo de entidad.
- **Auth del handshake**: los navegadores no permiten mandar el header
  `Authorization` al abrir un WebSocket nativo, así que la conexión se
  acepta primero y el cliente DEBE mandar un primer mensaje
  `{"type":"auth","token":"<access_token>"}` dentro de 5s; el servidor
  llama a `auth.Handler.VerifyAccessToken` (agregado a `internal/auth`
  específicamente para este caso — parsea/valida el JWT fuera del
  flujo de middleware `RequireAuth`) y cierra la conexión si falla o
  no llega a tiempo.
- **CORS del WS**: `AcceptOptions.InsecureSkipVerify = true` — se
  permite cualquier origen en vez de restringir a same-origin, porque
  el server está pensado para clientes en orígenes distintos
  (desktop/web/móvil, ver `arquitectura-inicial`) y, a diferencia de
  una cookie de sesión que un navegador adjunta automáticamente, el
  token de auth solo viaja si el propio código del cliente lo manda
  explícitamente en el primer mensaje — una página maliciosa en otro
  origen no puede leerlo ni enviarlo por la política del mismo origen,
  así que el riesgo de CSRF que la verificación de origen mitigaría ya
  está cubierto por el diseño del mecanismo de auth.
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
- **Cómo se dispara `Notify`**: cada dominio recibe un `*realtime.Hub`
  inyectado en su `NewHandler` (mismo patrón que `*auth.Handler`) y
  llama a `hub.Notify(userID, "<tipo>", id, seq)` tras cada escritura
  exitosa (upsert o soft-delete) en su propio handler — no en el
  `store`, es una responsabilidad de la capa HTTP. Los `store.go` de
  las 6 entidades con `id` propio y de las 2 con clave natural tuvieron
  que empezar a devolver el `seq` asignado en el delete (antes
  descartado) para poder incluirlo en el aviso.
- **Heartbeat/keepalive**: cada conexión corre dos goroutines
  (`client.readLoop`/`client.writeLoop`, `internal/realtime/client.go`)
  coordinadas por un canal `send` (nunca cerrado, para evitar panics
  de "send on closed channel" bajo la carrera entre `Notify` y una
  desconexión concurrente) y un canal `done` (cerrado una sola vez vía
  `sync.Once`). `writeLoop` manda un ping cada 30s (`pingInterval`); si
  no hay pong dentro de 10s (`pingTimeout`), cierra la conexión — lo
  que a su vez desbloquea el `Read` de `readLoop` con un error y
  termina esa goroutine también.
- **Reconexión**: es responsabilidad de cada cliente (backoff, cuándo
  reintentar) — el servidor no la implementa ni la asume; solo
  garantiza que reconectar usa el mismo `GET /sync/changes` que la
  notificación en vivo.

## Explícitamente pendiente

- Paginación de `/sync/changes` (probablemente `limit` + el `seq` del
  último elemento como cursor de página, reutilizando el mismo campo).
- Cómo viajan los updates CRDT de `yrs` dentro del campo `data` de un
  cambio de tipo `note`/`daily_entry` (payload binario vs. snapshot
  resuelto) — ver decisión de CRDT en
  [`arquitectura-inicial/design.md`](../arquitectura-inicial/design.md).
