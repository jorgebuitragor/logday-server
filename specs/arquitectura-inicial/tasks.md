# Arquitectura inicial — Tareas

Estado: en diseño. Este spec es de arquitectura, no de implementación
todavía — las tareas de abajo son de **diseño**, no de código, hasta que
las decisiones pendientes en `requirements.md` se resuelvan.

- [x] Decidir estrategia de resolución de conflictos: LWW por campo para
      campos simples, CRDT (`yrs` vía CGO) acotado a texto largo
      (`Note.content`, `daily_entries.content`).
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
- [ ] Una vez resuelto lo anterior: scaffold del repo (`go.mod`, `chi`,
      SQLite, Dockerfile, `docker-compose.yml`).
