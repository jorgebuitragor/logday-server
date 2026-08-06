# Sync incremental — Tareas

Estado: en progreso (`GET /sync/changes`, WS en tiempo real y purga de
tombstones con `410` implementados para las 7 entidades de negocio;
CRDT y paginación siguen pendientes)

- [x] Definir formato de cursor (secuencia monótona por usuario).
- [x] Definir forma del endpoint de cambios (unificado, no por tipo).
- [x] Definir manejo de deletes/tombstones (soft-delete + purga 90 días
      + full resync en cursor inválido).
- [x] Diseñar el evento WebSocket que dispara el pull incremental
      (topic por usuario, aviso liviano tipo+id+seq).
- [x] Definir cómo llegan las escrituras al servidor: REST por entidad
      (`POST`/`PUT`/`DELETE`), no push genérico por batch. Primer caso
      real: `internal/task` (`POST /tasks`, `PUT /tasks/:id`,
      `DELETE /tasks/:id`).
- [x] Implementar `GET /sync/changes?since=<seq>`: `internal/sync/`,
      fan-out (vía helper `addChanges`) a las 7 entidades de negocio +
      merge/sort por `seq`. Validado end-to-end: cursor incremental,
      tombstones con `deleted:true` (incluidas las dos entidades de
      clave compuesta, `id` sintético correcto), orden global por
      `seq`, 401 sin auth.
- [x] Implementar el job de purga de tombstones y el `410 Gone`/
      full-resync en cursor inválido: `internal/db/purge.go`
      (`PurgeTombstones`, corrido desde una goroutine con
      `time.Ticker` en `cmd/server/main.go`, una vez al arrancar y
      luego cada 24h — sin cron externo), con un watermark por usuario
      (`user_sync_counters.purged_before_seq`, migración `00012`) que
      nunca baja. `GET /sync/changes` responde `410` si `since` está
      por debajo del watermark del usuario. Validado con tests
      unitarios contra SQLite real (purga borra tombstones viejos y
      conserva los recientes, watermark correcto, watermark no
      retrocede en purgas sucesivas, 410 exacto en el límite) y contra
      un contenedor real (migración aplicada, job corre sin error al
      arrancar, sin regresión en `/sync/changes`).
- [x] Implementar el WebSocket de tiempo real: `internal/realtime/`
      (`GET /ws`), librería `coder/websocket`, auth por primer mensaje
      (`{"type":"auth","token":"..."}` — los navegadores no permiten
      `Authorization` en el handshake), heartbeat con ping cada 30s /
      timeout de 10s. `Hub` inyectado en las 7 entidades de negocio,
      cada una llama `Notify` tras cada escritura exitosa. Validado
      end-to-end contra un contenedor real: conectar, autenticar,
      crear un `task` vía REST, recibir el aviso
      `{type:"task", id:"...", seq:1}` por el socket. Tests unitarios
      cubren entrega de notificación, rechazo por falta de auth, y
      rechazo por token inválido.
- [ ] Definir paginación del endpoint de cambios.
- [ ] Definir forma exacta de los updates CRDT dentro del payload.
