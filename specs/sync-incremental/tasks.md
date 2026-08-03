# Sync incremental — Tareas

Estado: en diseño

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
- [ ] Definir paginación del endpoint de cambios.
- [ ] Definir forma exacta de los updates CRDT dentro del payload.
- [ ] Definir reconexión/heartbeat del WebSocket.
