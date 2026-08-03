# Arquitectura inicial — Tareas

Estado: en diseño. Este spec es de arquitectura, no de implementación
todavía — las tareas de abajo son de **diseño**, no de código, hasta que
las decisiones pendientes en `requirements.md` se resuelvan.

- [ ] Decidir estrategia de resolución de conflictos (last-write-wins vs.
      alternativa más precisa) — condiciona todo el protocolo de sync.
      Satisface: requisito "Resolución de conflictos" en `requirements.md`.
- [ ] Diseñar el protocolo de sync incremental (formato de cursor, forma
      del endpoint "cambios desde X", manejo de deletes/tombstones).
      Satisface: requisitos de sync en tiempo real y reconciliación al
      reconectar.
- [ ] Diseñar el esquema de auth multi-usuario (tablas `users`,
      `devices`/`sessions`, formato de tokens JWT + refresh).
      Satisface: requisitos de multi-usuario y aislamiento por usuario.
- [ ] Definir el esquema de datos completo tabla por tabla, a partir del
      mapeo de `design.md` y los tipos TS de `task-manager`.
- [ ] Diseñar los eventos/topics de WebSocket para el sync en tiempo
      real (qué se notifica, con qué payload mínimo).
      Satisface: requisito de sync en tiempo real.
- [ ] Decidir el orden de integración de clientes (desktop primero, dado
      que ya existe) y cómo convive temporalmente con el sync por git
      durante la transición.
- [ ] Una vez resuelto lo anterior: scaffold del repo (`go.mod`, `chi`,
      SQLite, Dockerfile, `docker-compose.yml`).
