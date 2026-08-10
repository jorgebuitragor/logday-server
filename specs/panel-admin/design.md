# Panel de administración web — Diseño

Estado: implementado — ver `internal/auth/panel_*.go`, `internal/auth/templates/` y `internal/db/migrations/00014_add_deleted_at_to_users.sql`

## Dónde vive: dentro de `internal/auth`, no un paquete nuevo

`convenciones-codigo/design.md` ya asigna este dominio: *"internal/auth/
users/devices/sesiones: handlers, store, bootstrap del admin"*. El panel no
es un bounded context nuevo — es una superficie HTML nueva sobre las mismas
tablas `users`/`devices` que `internal/auth` ya posee. Es también la única
opción viable sin romper la superficie exportada mínima del paquete: `store`
y sus métodos son no exportados, y un paquete separado no podría llamarlos.

Archivos nuevos: `panel_session.go` (sesión de cookie + CSRF),
`panel_handlers.go` (handlers HTML + `PanelRoutes(r chi.Router)`, separado
de `Routes`), `templates.go` (`//go:embed`), `templates/*.html`.

## Sesión de navegador: cookie propia, no el modelo de device/JWT existente

El modelo de `devices` + rotación de refresh token existe para un problema
distinto: un cliente que puede estar offline hasta 30 días y necesita
retomar sesión sin repreguntar contraseña, con rotación + detección de
reuso porque un token opaco de larga vida robado es un riesgo real. Nada de
eso aplica a una persona usando el panel ocasionalmente desde un navegador
— no hay período offline que salvar, y una cookie de TTL fijo y corto es la
postura de seguridad correcta acá (re-login periódico, blast radius chico),
no una limitación. Reusar el access token de 15 min (pensado para un
cliente con su propio refresh automático en background) hubiera forzado
construir un mecanismo de refresh silencioso solo para el panel — ceremonia
no justificada por el problema real, mismo criterio que ya descartó
interfaces genéricas prematuras en otras partes de este proyecto.

- Cookie `logday_admin_session`, `HttpOnly`, `SameSite=Lax`, `Secure` si
  `r.TLS != nil`. Claims propios (`{UserID, IsAdmin, exp}`, TTL fijo 24h,
  sin rotación) vía `security.SignJWT`/`ParseJWT` — mismas primitivas que
  ya usa `tokens.go` para el access token, claims y TTL distintos.
- `requireAdminSession` **no confía en el claim `IsAdmin` del JWT**: hace
  `SELECT is_admin, deleted_at FROM users WHERE id = ?` en cada request
  (PK indexada, tráfico de panel mínimo). Sin este chequeo vivo, alguien
  recién degradado o dado de baja seguiría pasando autorización hasta 24h
  con una cookie vieja.
- Login del panel reusa el `loginLimiter` ya existente (misma clave
  `IP+email`); rechaza no-admins con el mismo mensaje genérico que
  credenciales inválidas.
- Falla de sesión (cookie ausente/inválida/expirada, o el chequeo vivo
  falla) → redirect 302 a `/admin/panel/login`, no un 401/403 JSON — es la
  UX correcta para una superficie HTML.

## CSRF

Double-submit cookie en cada form, incluido `/setup` (no hay sesión
todavía en ese punto): una cookie `HttpOnly` con un valor opaco
(`security.GenerateOpaqueToken`, no se persiste — se compara consigo
misma) más un campo hidden en el form con el mismo valor. El POST se
rechaza si no coinciden. Sin dependencia nueva.

## Setup inicial (`/setup`)

`Bootstrap()` cambia: el caso "cero usuarios activos y sin `ADMIN_EMAIL`/
`ADMIN_PASSWORD`" deja de ser un error fatal — loguea un mensaje
informativo y retorna `nil`. El path con variables de entorno queda
exactamente igual, byte a byte.

`GET/POST /setup`, top-level (no anidado bajo `/admin/panel` — es
alcanzable antes de que exista sesión alguna), ambos públicos.

La garantía contra doble-creación no es el chequeo previo al insert (eso es
una carrera TOCTOU entre requests concurrentes) sino una transacción:
`createFirstAdmin` hace `BeginTx` → `SELECT COUNT(*) FROM users WHERE
deleted_at IS NULL` → si `0`, `INSERT` → `Commit`; si no, rollback y "ya
inicializado". El `GET /setup` hace además un chequeo no transaccional solo
para UX (redirect si ya hay admin) — la garantía real es la transacción.

Al completar el setup: login automático (cookie seteada) y redirect a
`/admin/panel`.

## Administración de usuarios y dispositivos

Nuevos métodos no exportados en `store`:

- `listUsers` — todos los usuarios, activos y dados de baja (el panel es
  el único lugar que necesita ver ambos).
- `withLastAdminGuard(id, fn)` — corre `fn` dentro de una transacción,
  después de confirmar que `id` no es el único admin activo restante (no
  hace nada si `id` no es admin — así sirve tanto para `updateUserAdmin`
  cuando promueve como cuando degrada). La cuenta de admins vive
  transaccional acá adentro, no como un método `countAdmins` aparte
  llamado por fuera — evita una carrera entre el chequeo y el `UPDATE`
  real si dos requests concurrentes intentan degradar al mismo tiempo.
- `updateUserAdmin(id, isAdmin)` — promover/degradar vía
  `withLastAdminGuard`; degradar al único admin activo retorna
  `errLastAdmin` sin aplicar el cambio.
- `softDeleteUser(id)` — mismo guard, vía `withLastAdminGuard`.
  Transacción: `UPDATE users SET deleted_at = ?` + `DELETE FROM devices
  WHERE user_id = ?`. El borrado de devices es explícito porque el
  `ON DELETE CASCADE` de la FK solo dispara en `DELETE`, no en `UPDATE`
  — el soft-delete es sobre la cuenta, no implica que sus sesiones sigan
  vivas.
- `restoreUser(id)` — `UPDATE users SET deleted_at = NULL`. Sin esto el
  soft-delete no tendría ventaja práctica sobre un hard-delete.
- `updateUserPassword(id, hash)` — reset admin-asistido. Misma
  transacción que softDeleteUser: cambia el hash y borra los devices del
  usuario — un reset de contraseña es exactamente el tipo de evento
  donde corresponde forzar re-login en todos sus dispositivos.
- `listAllDevices` — `JOIN users` para el email del dueño, sin scoping
  por `user_id` (a diferencia de `listDevices`, que sí scopea).
- `createFirstAdmin` — ver arriba.

`getUserByEmail`/`countUsers`/`countAdmins` excluyen `deleted_at IS NOT
NULL` — un usuario dado de baja no cuenta como admin activo para ningún
guard, y no puede loguearse en ningún lado (panel ni API de sync).

No hace falta nada nuevo para crear un usuario (`store.createUser` ya
sirve) ni para revocar un device puntual (`store.deleteDevice(ctx, id)` ya
existe y ya no scopea por usuario).

## Templates y ruteo

`html/template` puro (sin dependencia nueva) — autoescape real importa acá:
`device_name`/`email` son strings de usuario renderizados en HTML.
Embebido vía `//go:embed templates/*.html`, parseado una vez en
`NewHandler`. Un `layout.html` con CSS inline mínimo + un archivo por
página (`setup.html`, `login.html`, `users.html`, `devices.html`).

Prefijo `/admin/panel/*` para la superficie HTML, separado del JSON
`POST /admin/users` existente — no es requisito técnico de `chi` (solo
`chi.Mount` colisiona en paths compartidos, y acá se registra todo directo
sobre `r`, sin `Mount`), es para que la separación API-JSON vs panel-HTML
sea inequívoca al leer las rutas.

```
GET/POST  /setup
GET/POST  /admin/panel/login
POST      /admin/panel/logout
GET       /admin/panel
POST      /admin/panel/users
POST      /admin/panel/users/{id}/promote
POST      /admin/panel/users/{id}/demote
POST      /admin/panel/users/{id}/delete
POST      /admin/panel/users/{id}/restore
POST      /admin/panel/users/{id}/reset-password
GET       /admin/panel/devices
POST      /admin/panel/devices/{id}/revoke
```

## Migración

`00014_add_deleted_at_to_users.sql` — `ALTER TABLE users ADD COLUMN
deleted_at TEXT` (mismo estilo que `00012_add_purged_before_seq.sql`).

## Explícitamente pendiente

- Las 7 tablas de dominio (`tasks`, `notes`, etc.) no tienen FK a `users`
  — un usuario dado de baja deja sus filas sincronizadas huérfanas en esas
  tablas. No se construye una purga cross-domain para esto: es exactamente
  el tipo de acoplamiento entre dominios que `internal/sync`/
  `internal/realtime` fueron diseñados para evitar. Con soft-delete esto
  es menos urgente que con hard-delete — la cuenta es recuperable, así que
  sus datos "vuelven a tener dueño" si se restaura.
- Recuperación de contraseña self-service (requiere SMTP).
- Reverse proxy/TLS: documentado como prosa en `README.md`, no forma
  parte de este spec — el servidor sigue sirviendo HTTP plano a propósito.
