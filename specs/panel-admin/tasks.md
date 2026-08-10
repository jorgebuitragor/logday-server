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

## Ronda 2: look & feel de Logday, confirmaciones, configuración

- [x] Decidir alcance de theming: dark por defecto + light automático por
      `prefers-color-scheme`, sin selector de tema completo (v1) — luego
      se sumó un toggle manual igual (ver más abajo), decisión revisada
      a pedido explícito.
- [x] Decidir uso del logo real: sí, copiar `task-manager/src/assets/logo.png`
      a `internal/auth/static/`, embebido y servido como favicon + logo.
- [x] Decidir documentación de reverse proxy: solo prosa en README, sin
      archivo de ejemplo — ya cubierto arriba, confirmado de nuevo.
- [x] Portar paleta/tipografía/radios reales de Logday (`task-manager/src/App.css`)
      a `templates/partials.html` — dark+light con los mismos valores hex,
      sin inventar colores.
- [x] Agregar toggle manual de tema (íconos sol/luna, `data-theme` +
      `localStorage`, sin flash del tema incorrecto).
- [x] Corregir contraste de bordes de inputs (`--border` → `--border-high`)
      — el valor calcado de Logday tenía muy poco contraste contra el
      fondo casi negro del panel, reportado como bug visual.
- [x] Agregar íconos reales (Lucide, no genéricos) en toda la superficie
      interactiva: `templates/icons.html`, 18 íconos.
- [x] Convertir "Crear usuario" de formulario inline a `<dialog>` modal.
- [x] Corregir espaciado del botón "Crear usuario" pegado al header de
      la tabla (`.page-header` en vez de estilos inline sin margen).
- [x] Agregar diálogo de confirmación compartido (`data-confirm` en
      forms) para: cerrar sesión, dar de baja usuario, resetear
      password, revocar device, promover/degradar admin.
- [x] Decidir alcance del apartado de "Configuración" — preguntado
      explícitamente al usuario, eligió las 4 opciones propuestas:
      nombre de instancia, estado/generador de `JWT_SECRET`, retención
      de tombstones, rate limit de login.
- [x] Decidir alcance de "rotación" de `JWT_SECRET`: generador de
      sugerencia (no persiste, no aplica en caliente) en vez de
      rotación real — ver justificación en `design.md`.
- [x] Paquete `internal/settings` nuevo: `Settings`, `Get`, `Update`,
      `TombstoneRetention()`, `LoginRateLimitWindow()`.
- [x] Migración `00015_create_instance_settings.sql`.
- [x] Conectar `internal/db/purge.go` y `internal/auth/ratelimit.go` a
      settings en vivo (sin restart) en vez de constantes fijas.
- [x] `templates/settings.html` + handlers `panelSettings`,
      `panelUpdateSettings`, `panelGenerateSecret` + validación de rangos.
- [x] Instance name dinámico en `<title>`/`.brand-name` en las 5 páginas
      del panel (`Handler.instanceName`, con fallback si falla la lectura).
- [x] Tests: `internal/settings/settings_test.go` (paquete de test
      externo `settings_test`, para evitar el ciclo de import con
      `internal/db`, que ahora importa `internal/settings`),
      `TestPanelSettingsPage` en `panel_handlers_test.go`.
- [x] `go build`/`go test`/`golangci-lint run` en verde (0 issues) tras
      cada cambio.
- [x] Validación manual contra un contenedor Docker real: login,
      diálogos de confirmación, página de configuración completa
      (guardar valores válidos/inválidos, generar secreto sugerido,
      nombre de instancia reflejado en el header).
- [x] Actualizar `specs/panel-admin/{requirements,design}.md` con la
      sección de Configuración y la de confirmaciones/look&feel.

## Ronda 3: estilo real del modal de confirmación, IP e ícono de dispositivo

- [x] Leer los componentes reales `ConfirmDeleteModal.tsx`/`ModalPanel.tsx`/
      `ModalOverlay.tsx` de `task-manager` y calcar su tamaño, tipografía,
      colores de botón (`bg-red-500`/`hover:bg-red-600`) y backdrop
      (`bg-black/60 backdrop-blur-sm` = blur 4px, no 2px) en `#confirm-modal`
      — antes era una aproximación genérica, no el componente real.
- [x] Quitar el botón de cierre en X del header del confirm-modal (el
      componente real no lo tiene); agregar cierre al hacer click en el
      backdrop, igual que `ModalOverlay`.
- [x] Migración `00016_add_device_ip_and_agent.sql`: `last_ip`/`user_agent`
      en `devices`.
- [x] `createDevice`/`rotateRefreshToken` escriben `clientIP(r)`/
      `r.UserAgent()` (creación y en cada refresh, igual criterio que
      `last_used_at`).
- [x] `listAllDevices` selecciona las columnas nuevas; `device.IconName()`
      clasifica por heurística de substring sobre el User-Agent
      (tablet/smartphone/terminal/laptop-default).
- [x] Íconos Lucide nuevos: `smartphone`, `tablet`, `terminal`.
- [x] `devices.html`: columna IP + ícono de tipo junto al nombre del
      dispositivo (`.device-cell`).
- [x] `go build`/`go test`/`golangci-lint run` en verde (0 issues);
      validación manual contra Docker real.
- [x] Actualizar `specs/panel-admin/{requirements,design,tasks}.md`.

## Ronda 4: dominios de email, contraseña mínima, TTLs de sesión, máximo de dispositivos

- [x] Preguntado explícitamente al usuario qué ajustes sumar además del
      filtro de dominio (su idea original) — eligió los tres restantes:
      longitud mínima de contraseña, TTLs de sesión, máximo de
      dispositivos por usuario.
- [x] Comunicado explícitamente que el filtro de dominio es un guardrail
      operativo, no un control de acceso (no existe registro público en
      ningún momento) — el usuario decidió incluirlo igual.
- [x] Migración `00017_add_config_flexibility.sql`: 6 columnas nuevas en
      `instance_settings`, defaults idénticos al valor hoy hardcodeado.
- [x] `internal/settings`: 6 campos nuevos + `EmailDomainAllowed`,
      `AccessTokenTTL`, `RefreshTokenTTL`, `PanelSessionTTL`.
- [x] `tokens.go`/`panel_session.go`: se eliminan `accessTokenTTL`,
      `refreshTokenTTL`, `panelSessionTTL`; `issueAccessToken`/
      `issuePanelSession`/`setSessionCookie` reciben `ttl` como
      parámetro; `ensureCSRFCookie` queda con un `csrfCookieTTL` fijo
      propio en vez de la constante prestada.
- [x] `store.go`: `rotateRefreshToken` +parámetro `refreshTTL`; nuevo
      `countDevices`.
- [x] `handlers.go`: `login` chequea `MaxDevicesPerUser` antes de crear
      el device (403 si se alcanzó); `login`/`refresh` usan TTLs en
      vivo; `adminCreateUser` (JSON) valida largo mínimo y dominio —
      corrige que antes no validaba ningún largo.
- [x] `panel_handlers.go`: `setupSubmit`/`panelLoginSubmit` usan
      `cfg.PanelSessionTTL()`; `panelCreateUser`/`panelResetPassword`/
      `setupSubmit` usan `cfg.MinPasswordLength`; `panelCreateUser`
      valida dominio (setup queda exento a propósito);
      `panelUpdateSettings` valida y persiste los 6 campos nuevos.
- [x] `settings.html`: secciones "Cuentas" y "Sesiones" nuevas.
- [x] Tests nuevos: `EmailDomainAllowed` (tabla de casos),
      `TestPanelCreateUserRespectsEmailDomainAllowlist`,
      `TestLoginRejectsBeyondMaxDevicesPerUser`, bounds de los 6 campos
      nuevos en `TestPanelSettingsPage`; firmas actualizadas en
      `tokens_test.go`/`panel_session_test.go`.
- [x] `go build`/`go test`/`golangci-lint run` en verde (0 issues);
      validación manual contra Docker real (dominio rechazado/aceptado,
      TTL corto expirando, máximo de dispositivos rechazando el
      N+1-ésimo login).
- [x] Actualizar `specs/panel-admin/{requirements,design,tasks}.md`.

## Ronda 5: validación de email, "Resetear password" en modal, toasts

- [x] `helpers.go`: `validEmail` vía `net/mail.ParseAddress` (sin
      dependencia nueva) — check de formato, no de deliverabilidad.
      Conectado en `adminCreateUser` (JSON), `panelCreateUser`,
      `setupSubmit`.
- [x] "Resetear password" pasa de `<details>` inline por fila a un
      `<dialog>` único compartido en `users.html` (mismo patrón que
      `create-user-modal`), abierto por fila vía
      `openResetPasswordModal(id, email)`; incluye generador de
      contraseña aleatoria (Web Crypto, alfabeto sin caracteres
      ambiguos) + botón de copiar.
- [x] `usersPageData`/`create-user-modal`/`reset-password-modal` usan
      `MinPasswordLength` en vivo (ya no `minlength="8"` hardcodeado).
- [x] Toast de confirmación (`{{define "toast"}}` en `partials.html`,
      patrón post/redirect/get: el handler redirige con
      `?success=mensaje`, el GET siguiente lo renderiza una vez y el
      toast se auto-oculta a los 3.5s). Nuevo token `--success` y
      `redirectWithSuccess`, usado en: crear/promover/degradar/dar de
      baja/restaurar usuario, resetear contraseña, revocar device,
      guardar configuración. Login/logout/setup quedan sin toast (el
      redirect mismo ya es la confirmación visible).
- [x] Tests: `helpers_test.go` (`validEmail`, tabla de casos),
      `TestAdminCreateUserRejectsMalformedEmail`, caso de email
      malformado + verificación del toast renderizado en
      `TestPanelUserAndDeviceLifecycle`, `TestPanelSettingsPage`
      actualizado al nuevo `Location` con `?success=`.
- [x] `go build`/`go test`/`golangci-lint run` en verde (0 issues);
      validación manual contra Docker real (email inválido rechazado,
      modal de reset con generador, toast visible tras reset de
      contraseña).
- [x] Actualizar `specs/panel-admin/{requirements,tasks}.md`.
