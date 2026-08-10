# Panel de administración web — Diseño

Estado: implementado — ver `internal/auth/panel_*.go`, `internal/auth/templates/`, `internal/settings/`, `internal/db/migrations/00014_add_deleted_at_to_users.sql` y `00015_create_instance_settings.sql`

## Dónde vive: dentro de `internal/auth`, no un paquete nuevo

`convenciones-codigo/design.md` ya asigna este dominio: *"internal/auth/
users/devices/sesiones: handlers, store, bootstrap del admin"*. El panel no
es un bounded context nuevo — es una superficie HTML nueva sobre las mismas
tablas `users`/`devices` que `internal/auth` ya posee. Es también la única
opción viable sin romper la superficie exportada mínima del paquete: `store`
y sus métodos son no exportados, y un paquete separado no podría llamarlos.

Archivos nuevos: `panel_session.go` (sesión de cookie + CSRF),
`panel_handlers.go` (handlers HTML + `PanelRoutes(r chi.Router)`, separado
de `Routes`), `templates.go` (`//go:embed` de templates y de
`static/logo.png`), `templates/*.html`, `static/logo.png` (isotipo de
Logday, copiado de `task-manager/src/assets/logo.png`, usado como favicon
y logo del header — ver "Look & feel" abajo).

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

`getUserByEmail`/`countUsers` excluyen `deleted_at IS NOT NULL` — un
usuario dado de baja no cuenta como admin activo para ningún guard
(incluida la cuenta transaccional dentro de `withLastAdminGuard`), y no
puede loguearse en ningún lado (panel ni API de sync).

No hace falta nada nuevo para crear un usuario (`store.createUser` ya
sirve) ni para revocar un device puntual (`store.deleteDevice(ctx, id)` ya
existe y ya no scopea por usuario).

### IP y tipo de dispositivo en `/admin/panel/devices`

`devices` no tenía ninguna columna con señal estructurada sobre el
dispositivo — `device_name` es texto libre que pone el cliente al
loguearse (p. ej. "Postman", "member-laptop"), no algo confiable para
clasificar. Migración `00016_add_device_ip_and_agent.sql` agrega
`last_ip`/`user_agent` (`NOT NULL DEFAULT ''`, así que devices creados
antes de esta migración simplemente muestran "—"/ícono genérico en vez de
romper). Se escriben en `login` (creación, `clientIP(r)`/`r.UserAgent()`)
y de nuevo en cada `refresh` (`rotateRefreshToken`, junto a
`last_used_at`) — reflejan la conexión **más reciente**, no la original,
igual criterio que `last_used_at`.

El ícono de tipo de dispositivo en `devices.html` es un método
`device.IconName()` (heurística por substring sobre el `User-Agent` en
minúsculas: `ipad`/`tablet` → tablet, `iphone`/`android`/`mobile` →
smartphone, `postman`/`curl`/`insomnia`/`python-requests`/`httpie` →
terminal, cualquier otro caso o UA vacío → laptop, el default, que cubre
tanto el cliente de escritorio de Logday como acceso por navegador). Es
una heurística deliberadamente simple (no hay dependencia de parseo de
User-Agent) — el objetivo es una señal visual aproximada para el admin,
no una clasificación exacta de dispositivo.

## Templates y ruteo

`html/template` puro (sin dependencia nueva) — autoescape real importa acá:
`device_name`/`email` son strings de usuario renderizados en HTML.
Embebido vía `//go:embed templates/*.html`, parseado una vez en
`NewHandler`. `partials.html` define bloques compartidos (`head` — CSS
inline + favicon, `nav` — header con tabs + diálogo de confirmación,
`theme-toggle-button`/`theme-toggle-script`) más un archivo por página
(`setup.html`, `login.html`, `users.html`, `devices.html`,
`settings.html`); `icons.html` define un `{{template "icon-X"}}` por cada
ícono usado (ver "Look & feel" abajo).

## Look & feel: paleta y componentes reales de Logday, no genéricos

El panel replica el sistema de diseño real de la app de escritorio
(`task-manager/src/App.css`), no una paleta inventada: variables CSS con
los mismos valores hex/rgba que usa la app (dark por defecto —
`--bg-base:#121212`, `--accent:#818cf8`/`#6366f1` — con una variante light
de los mismos roles), mismo stack de fuente de sistema
(`-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif`),
mismos radios de borde (`8px` controles, `16px` paneles tipo modal),
mismos botones sólidos/ghost sin bordes ni sombras. El logo real
(`static/logo.png`) se usa como favicon y como isotipo del header/cards.

**Tema claro/oscuro**: sigue `prefers-color-scheme` por defecto, pero hay
un toggle manual (íconos sol/luna de Lucide) que fuerza `data-theme` en
`<html>` y persiste la elección en `localStorage` — un script mínimo e
inline en `head` aplica esa elección antes del primer paint, para que no
haya flash del tema incorrecto. El swap entre ícono sol/ícono luna lo
resuelve CSS puro (mismos selectores que ya controlan la paleta), no JS.

**Íconos**: copiados literalmente de [Lucide](https://lucide.dev)
(`lucide-static`, ISC — la misma librería que usa `task-manager` vía
`lucide-react`) como SVG inline embebido en cada `{{define "icon-X"}}` de
`icons.html` — sin dependencia de build ni CDN, sizeados vía una clase
`.icon` (`width/height: 1em`) para escalar con el font-size de alrededor.

**Modales**: `<dialog>` nativo del navegador (`showModal()`/`close()`,
sin JS framework) para "Crear usuario" y para el diálogo de confirmación
compartido — con `::backdrop` difuminado y una animación de entrada tipo
resorte, calcada del `ModalPanel`/`ModalOverlay` real de `task-manager`.

## Confirmación de acciones críticas

Un único `<dialog id="confirm-modal">` por página (definido dentro de
`nav`, así que aparece en cualquier página que incluya el header — hoy
usuarios/dispositivos/configuración) intercepta el submit de cualquier
`<form data-confirm="mensaje" data-confirm-title="título"
data-confirm-tone="danger">` (el atributo `tone` es opcional, default al
botón primario/accent en vez de rojo), muestra el mensaje, y solo deja
pasar el submit real si el usuario confirma — el form original y su campo
CSRF quedan intactos, el diálogo es puramente un gate de UI delante.
`form.submit()` (a diferencia de un click real o `.requestSubmit()`) no
vuelve a disparar el evento `submit`, así que no hay loop.

Acciones marcadas como críticas: cerrar sesión, dar de baja usuario,
resetear password, revocar device, promover/degradar admin, restaurar
usuario (a pedido explícito del usuario — aunque es reversible, sigue
siendo una acción con efecto real: la cuenta vuelve a poder loguearse).
Deliberadamente sin confirmación: crear usuario (ya pasa por su propio
modal de datos), guardar configuración (ajuste operativo, no
destructivo), generar sugerencia de `JWT_SECRET` (no persiste ni aplica
nada).

El estilo de `#confirm-modal` está calcado del componente real
`ConfirmDeleteModal.tsx` de `task-manager` (no de una aproximación genérica
tipo `SettingsModal`): panel de 320px, `padding` 1.25rem, header compacto
(ícono `triangle-alert` + título `text-sm font-semibold`, sin botón de
cierre en X — el real tampoco lo tiene), mensaje `text-xs`
`color: var(--text-secondary)`, botones pequeños (`0.75rem`,
`padding: 0.375rem 0.75rem`, `border-radius: 8px`). El color del ícono y
del botón de confirmar cambian según `data-confirm-tone`: `danger` usa
rojo (`--danger` para el ícono, `#ef4444`/`#dc2626` hover para el botón
sólido — los mismos `bg-red-500`/`hover:bg-red-600` de Tailwind que usa el
componente real, no el token `--danger` genérico del resto del panel, que
es más claro/`red-400` y ahí solo se usa para texto/íconos); cualquier otro
tono usa `--accent`/`--accent-strong`. El backdrop es
`rgba(0,0,0,0.6)` + `blur(4px)` (`bg-black/60 backdrop-blur-sm` de
Tailwind — `backdrop-blur-sm` es 4px, no 2px). Clic en el backdrop cierra
el modal (`e.target === dialog` dentro del listener de `click` en el
`<dialog>`), igual que el `onClick={onClose}` de `ModalOverlay` en el
cliente real; el modal más grande de "Crear usuario" (formulario, no un
`ConfirmDeleteModal`) sigue con el tratamiento genérico de `.modal-body`/
`.modal-header` de más arriba.

`showModal()` no bloquea por sí solo el scroll de la página de fondo (solo
pone el `<dialog>` en el top layer e `inert` al resto, pero el `body`
sigue siendo scrolleable detrás) — `html:has(dialog.modal[open]) {
overflow: hidden; }` en `partials.html` lo bloquea mientras cualquier
modal esté abierto, vía CSS puro (`:has()`), sin JS adicional ni acoplar
esto a la lógica de apertura/cierre de cada dialog.

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
GET       /admin/panel/settings
POST      /admin/panel/settings
POST      /admin/panel/settings/generate-secret
GET       /admin/static/logo.png
```

## Configuración de instancia (`internal/settings`)

Tres constantes que antes vivían hardcodeadas en el código pasan a ser una
fila única configurable (`instance_settings`, `id` fijo en `1`,
sembrada por la migración): nombre de la instancia, retención de
tombstones en días, e intentos/ventana del rate limit de login.

**Por qué es un paquete nuevo y no vive en `internal/auth`**: a
diferencia del resto del panel, esta config la leen paquetes que no
tienen nada que ver entre sí (`internal/db` para la purga,
`internal/auth` para el rate limiter y para el panel mismo) — meterlo
en `internal/auth` habría obligado a `internal/db` a importar `internal/auth`
completo por una sola lectura. `internal/settings` sigue el mismo patrón
ya usado por `internal/task.ChangesSince` et al.: funciones de paquete
(`Get`/`Update`), no un tipo `store` que otros paquetes no podrían nombrar.
Sin tipo `store` en absoluto acá, a diferencia de los dominios reales —
es literalmente una fila, dos funciones, no hay estado que valga la pena
encapsular en un constructor.

- `internal/db/purge.go`: `PurgeTombstones` ya no usa una constante de 90
  días — llama `settings.Get` al principio de cada corrida (arranca una
  vez al boot + una vez por día, nunca un hot path) y usa
  `cfg.TombstoneRetention()`.
- `internal/auth/ratelimit.go`: `loginLimiter` ahora guarda un `*sql.DB` y
  llama `settings.Get` en cada `Allow`/`RecordFailure` — un `SELECT` por
  PK extra en un intento de login no es un costo real, y así un cambio
  hecho en el panel aplica de inmediato, sin reiniciar. Si la lectura
  falla, cae a los mismos valores que hoy son el default (5 intentos/60s)
  en vez de bloquear todo o dejar todo abierto.
- El nombre de instancia se lee en cada render de página
  (`Handler.instanceName`, con el mismo fallback a "Logday Server" si la
  lectura falla) y se inyecta en `<title>` y en `.brand-name` — todas las
  page-data structs (`formPageData`, `usersPageData`, `devicesPageData`,
  `settingsPageData`) llevan un campo `InstanceName`.
- Validación de rangos en `panelUpdateSettings` (no en `settings.Update`,
  que solo persiste): nombre 1–60 caracteres, retención 1–3650 días,
  intentos 1–100, ventana 10–3600 segundos.

**`JWT_SECRET`: generador, no rotación en caliente.** `POST
/admin/panel/settings/generate-secret` genera un valor con
`security.GenerateOpaqueToken` (misma primitiva que los refresh tokens
opacos) y lo muestra una sola vez en un campo de solo lectura con botón de
copiar — nunca se persiste ni se aplica en runtime. Rotar de verdad la
clave activa invalidaría todas las sesiones sin aviso y requeriría mover
la raíz de confianza de JWT de variable de entorno a base de datos —
decisión de arquitectura aparte, no tomada acá (ver "Fuera de este spec"
en `requirements.md`).

## Ronda 4: dominios de email, contraseña mínima, TTLs de sesión, máximo de dispositivos

Cuatro campos más en `instance_settings` (migración `00017`), mismo
patrón que los cuatro originales — validación de rangos vive en
`panelUpdateSettings`, `settings.Update` solo persiste.

- **`AllowedEmailDomains`** (CSV, `""` = cualquier dominio) +
  `Settings.EmailDomainAllowed(email)`. Se aplica en `adminCreateUser`
  (JSON `POST /admin/users`) y `panelCreateUser` — **no** en
  `setupSubmit`: el primer admin de la instancia no debe poder
  auto-bloquearse, y conceptualmente no hay quién haya configurado la
  restricción todavía en ese momento. Es deliberadamente un *guardrail
  operativo*, no un control de acceso: las tres vías de creación de
  usuario ya requieren ser admin (o ser el arranque inicial) — no existe
  registro público que este filtro pudiera estar gateando. Su valor real
  es evitar que un admin cargue por error el dominio equivocado al
  invitar gente.
- **`MinPasswordLength`** (default `8`, antes hardcodeado en 3 lugares
  de `panel_handlers.go` y ausente por completo en `adminCreateUser` —
  ese endpoint JSON no validaba ningún largo, solo que no estuviera
  vacía; queda corregido de paso).
- **`AccessTokenTTLMinutes`/`RefreshTokenTTLDays`/`PanelSessionTTLHours`**
  (defaults `15`/`30`/`24`, idénticos a las constantes que reemplazan:
  `tokens.go`'s `accessTokenTTL`/`refreshTokenTTL` y
  `panel_session.go`'s `panelSessionTTL`, ambas eliminadas). `login` y
  `refresh` (`handlers.go`) hacen `settings.Get` y pasan
  `cfg.AccessTokenTTL()`/`cfg.RefreshTokenTTL()` a
  `issueAccessToken`/`createDevice`/`rotateRefreshToken`;
  `setupSubmit`/`panelLoginSubmit` (`panel_handlers.go`) hacen lo mismo
  con `cfg.PanelSessionTTL()` para `issuePanelSession`/`setSessionCookie`.
  Un cambio de TTL aplica al próximo login/refresh, no invalida sesiones
  ya emitidas (el `exp` queda fijo en el JWT desde que se firmó).
  **`ensureCSRFCookie` no cambió de firma** — su `MaxAge` usaba
  `panelSessionTTL` prestada, ahora usa una constante propia y fija
  (`csrfCookieTTL = 24h`, mismo valor numérico): un token CSRF solo
  necesita sobrevivir de "renderizar un form" a "enviarlo", no la vida de
  toda la sesión, así que no valía la pena encadenar `settings.Get` a
  cada handler GET que solo renderiza un form.
- **`MaxDevicesPerUser`** (default `0` = sin límite) + nuevo
  `store.countDevices(ctx, userID)`. `login` rechaza con `403` antes de
  crear el device si `cfg.MaxDevicesPerUser > 0` y ya se alcanzó el
  límite — mensaje pide revocar un device primero, sin evicción
  automática (silenciosamente cerrar la sesión de otro dispositivo sería
  un efecto sorpresa). `refresh` no chequea esto — reutiliza un device
  existente, no agrega uno nuevo.

## Migración

- `00014_add_deleted_at_to_users.sql` — `ALTER TABLE users ADD COLUMN
  deleted_at TEXT` (mismo estilo que `00012_add_purged_before_seq.sql`).
- `00015_create_instance_settings.sql` — crea `instance_settings` con
  una fila sembrada (`id = 1`, valores default) en la misma migración.
- `00017_add_config_flexibility.sql` — 6 columnas más en
  `instance_settings`, defaults idénticos al valor hoy hardcodeado (ver
  "Ronda 4" arriba) — una instancia existente no cambia de comportamiento
  hasta que un admin toque algo.

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
