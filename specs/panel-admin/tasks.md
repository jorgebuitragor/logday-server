# Panel de administración web — Tareas

Estado: implementado

- [x] Decidir tipo de interfaz: panel server-rendered embebido en el
      binario Go (`html/template`), no SPA/frontend separado — mismo
      enfoque que Vaultwarden.
- [x] Decidir alcance v1: setup inicial + administración continua en un
      mismo panel.
- [x] Decidir dónde vive el código: dentro de `internal/auth`, no un
      paquete nuevo — ver `design.md`.
- [x] Decidir modelo de sesión del panel: cookie propia con TTL fijo de
      24h (no el modelo de device/refresh-token de los clientes de sync),
      con revalidación viva de `is_admin`/`deleted_at` en cada request,
      no solo el claim del JWT.
- [x] Decidir protección CSRF: double-submit cookie, sin dependencia
      nueva.
- [x] Decidir semántica de borrado de usuario: soft-delete (reversible),
      con revocación inmediata de sus devices.
- [x] Decidir si el admin puede resetear contraseñas de otros usuarios:
      sí, admin-asistido (no self-service, no requiere SMTP).
- [x] Decidir tratamiento de datos sincronizados huérfanos al dar de
      baja un usuario: aceptado como gap documentado, no se construye
      purga cross-domain — ver "Explícitamente pendiente" en `design.md`.
- [x] Decidir documentación de reverse proxy/TLS: prosa en `README.md`
      únicamente, sin archivo de ejemplo versionado en el repo.

## Implementación

- [x] Migración `00014_add_deleted_at_to_users.sql`.
- [x] `models.go`: `DeletedAt` en `user`, struct para device+email del
      dueño.
- [x] `store.go`: `listUsers`, `updateUserAdmin`, `softDeleteUser`,
      `restoreUser`, `updateUserPassword`, `listAllDevices`,
      `createFirstAdmin`, `withLastAdminGuard`; excluir soft-deleted de
      `getUserByEmail`/`countUsers`. (El `countAdmins` standalone del
      diseño original se eliminó por redundante — la cuenta vive
      directamente, transaccional, dentro de `withLastAdminGuard`.)
- [x] `bootstrap.go`: el caso sin env vars deja de ser fatal.
- [x] `panel_session.go`: claims propios, cookie de sesión,
      `requireAdminSession`, CSRF double-submit.
- [x] `templates.go` + `templates/*.html`: partials (head/nav), setup,
      login, users, devices.
- [x] `panel_handlers.go`: handlers + `PanelRoutes(r chi.Router)`.
- [x] `helpers.go`: `renderTemplate`.
- [x] `cmd/server/main.go`: wire `authHandler.PanelRoutes(r)`.
- [x] Tests: `store_test.go`, `bootstrap_test.go`, `panel_session_test.go`,
      `panel_handlers_test.go` (httptest + cookie jar) — 22 tests nuevos,
      incluido uno de concurrencia para `createFirstAdmin` y uno que
      prueba la revalidación viva de `is_admin` contra un cookie de
      sesión todavía no expirado.
- [x] `go build`/`go test`/`golangci-lint run` en verde (0 issues).
- [x] Validación manual end-to-end contra contenedores Docker reales:
      contenedor existente (con `ADMIN_EMAIL`/`ADMIN_PASSWORD`) — login,
      crear/promover/degradar/dar de baja/restaurar/resetear password de
      usuario, guard de último admin, revocar device ajeno, todo
      confirmado con curl+cookies; contenedor nuevo aislado (sin admin
      bootstrapeado) — `/setup` completo con validación de contraseñas
      distintas, auto-login tras crear el primer admin, y auto-bloqueo
      del wizard en el segundo `GET /setup`.
- [x] Actualizar `specs/auth-multiusuario/{requirements,design}.md` — el
      bootstrap ya no falla sin env vars.
- [x] `README.md`: reescritura completa (deploy, env vars, `/setup`,
      reverse proxy).
- [x] `specs/README.md`: fila nueva en el índice de features.
