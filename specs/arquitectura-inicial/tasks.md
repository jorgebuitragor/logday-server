# Arquitectura inicial — Tareas

Estado: implementado. Todas las tareas de diseño y su implementación
están completas.

- [x] Decidir estrategia de resolución de conflictos: LWW por campo para
      campos simples, CRDT acotado a texto largo (`Note.content`,
      `daily_entries.content`) — la librería cambió de `yrs`/CGO
      (decisión original) a `Deln0r/ygo` (Go puro), ver más abajo.
      Satisface: requisito "Resolución de conflictos" en `requirements.md`.
- [x] Diseñar el protocolo de sync incremental (formato de cursor, forma
      del endpoint "cambios desde X", manejo de deletes/tombstones).
      Movido a su propio spec: [`sync-incremental/`](../sync-incremental/requirements.md).
      Satisface: requisitos de sync en tiempo real y reconciliación al
      reconectar.
- [x] Diseñar el esquema de auth multi-usuario (tablas `users`,
      `devices`/`sessions`, formato de tokens JWT + refresh).
      Movido a su propio spec: [`auth-multiusuario/`](../auth-multiusuario/requirements.md).
      Satisface: requisitos de multi-usuario y aislamiento por usuario.
- [x] Definir el esquema de datos completo tabla por tabla, a partir del
      mapeo de `design.md` y los tipos TS de `task-manager`.
      Movido a su propio spec: [`esquema-datos/`](../esquema-datos/requirements.md).
- [x] Diseñar los eventos/topics de WebSocket para el sync en tiempo
      real (qué se notifica, con qué payload mínimo).
      Resuelto en [`sync-incremental/`](../sync-incremental/design.md).
      Satisface: requisito de sync en tiempo real.
- [x] Decidir el orden de integración de clientes (desktop primero) y
      cómo convive temporalmente con el sync por git durante la
      transición (reemplazo directo, sin convivencia).
- [x] Una vez resuelto lo anterior: scaffold del repo (`go.mod`, `chi`,
      SQLite, Dockerfile, `docker-compose.yml`). Validado end-to-end:
      `docker compose up` levanta un binario estático (CGO+musl,
      `mattn/go-sqlite3`) de ~25MB corriendo como usuario no-root, con
      SQLite persistido en volumen y `/health` respondiendo 200.
- [x] Implementar la integración CRDT para `Note.content`/
      `daily_entries.content`. Investigada primero la API real de
      `yffi` (verificada contra el código fuente actual, no de
      memoria — viable pero requiere bindings CGO/Rust a mano),
      encontradas de paso dos alternativas Go puro wire-compatibles
      con Yjs (`Deln0r/ygo`, `reearth/ygo`); elegida `Deln0r/ygo` —
      evita CGO/Rust/toolchain cruzado por completo, coherente con el
      criterio que ya guio el resto de decisiones de este proyecto.
      `internal/crdt` envuelve la librería sin conocimiento de
      dominio (mismo criterio que `internal/security`).
      Migración `00013` reemplaza `content TEXT` por
      `content_crdt BLOB` en `notes` y `daily_entries`. Ver
      `design.md` para el historial completo de la decisión
      (incluida una nota de proceso: la integración de `note` la
      escribió un agente que se saltó su mandato de solo investigar,
      código revisado y conservado por su calidad real, no por
      confianza ciega).
      Validado end-to-end contra un contenedor real: dos ediciones
      concurrentes offline al mismo campo de texto se mezclan
      correctamente sin perderse, en `note` y en `daily_entries`.
