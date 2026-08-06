# Sync incremental — Tareas

Estado: en progreso (`GET /sync/changes` implementado para las 7
entidades de negocio; WS en tiempo real y CRDT siguen pendientes)

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
- [ ] Implementar el `410 Gone`/full-resync en cursor inválido —
      diferido a propósito: sin el job de purga de tombstones (>90
      días) implementado todavía, un cursor nunca puede quedar
      realmente inválido, así que el código no tendría forma de
      probarse honestamente. Se construye junto con la purga.
- [ ] Definir paginación del endpoint de cambios.
- [ ] Definir forma exacta de los updates CRDT dentro del payload.
- [ ] Definir reconexión/heartbeat del WebSocket.
