# Auth multi-usuario — Diseño

Estado: implementado (v1) — ver `internal/auth/` y
`internal/db/migrations/00001_create_users.sql`,
`00002_create_devices.sql`, `00003_create_used_refresh_tokens.sql`.

## Modelo de datos (alto nivel)

- `users(id, email, password_hash, is_admin, created_at)`
- `devices(id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at)`
  — el refresh token se guarda hasheado, igual que el password, nunca
  en texto plano.
- `used_refresh_tokens(token_hash, device_id, used_at)` — agregada
  durante la implementación, no estaba en el diseño original. Necesaria
  para distinguir "token inválido" (nunca existió) de "token ya
  rotado" (reuso = posible robo) al hacer refresh: sin este registro,
  ambos casos son indistinguibles porque `devices.refresh_token_hash`
  se sobreescribe en cada rotación. Se purga oportunistamente en cada
  rotación (`DELETE ... WHERE used_at < now - refresh_ttl - 1 día`), no
  hace falta un job aparte.

## Password: argon2id

Librería: `github.com/alexedwards/argon2id`, wrapper con encoding
estándar que serializa salt y parámetros en el mismo string
`$argon2id$...` — facilita ajustar parámetros de costo a futuro sin
migración de esquema.

## Tokens

- **Access token**: JWT, TTL 15 min, firmado con HS256 (secreto de
  servidor). RS256 no aporta nada aquí porque el servidor es el único
  emisor y verificador — no hay terceros validando el token.
- **Refresh token**: opaco (no JWT), valor random de alta entropía,
  almacenado hasheado en `devices.refresh_token_hash`. TTL 30 días
  desde el último uso.
- **Rotación**: cada refresh exitoso invalida el token usado
  (`refresh_token_hash` se reemplaza) y emite uno nuevo. Un intento de
  reuso de un refresh token ya invalidado revoca toda la sesión del
  `device_id` asociado (se borra/invalida la fila de `devices`).

## Registro: invite-only

- Sin endpoint de registro público.
- Setup inicial: al arrancar, si la tabla `users` está vacía, el
  servidor lee `ADMIN_EMAIL`/`ADMIN_PASSWORD` del entorno y crea ese
  usuario como admin (`internal/auth/bootstrap.go`). Si no están
  seteadas y no hay usuarios, el servidor arranca de todos modos y
  sirve `GET /setup` — un admin se crea desde ahí en su lugar (ver
  [`panel-admin/`](../panel-admin/design.md)). Antes de que existiera
  el panel, este caso hacía fallar el arranque del servidor
  (`log.Fatal`) — ya no. En arranques posteriores (ya con usuarios) es
  un no-op, aunque las variables sigan presentes en el entorno.
- Un admin crea usuarios adicionales vía `POST /admin/users`
  (requiere `Authorization: Bearer <access_token>` de un usuario con
  `is_admin=true`).

## Revocación de dispositivos

- `GET /devices` — lista los dispositivos del usuario autenticado
  (nombre, última actividad).
- `DELETE /devices/:id` — invalida el refresh token de ese dispositivo
  de inmediato. El access token ya emitido para ese dispositivo sigue
  siendo válido hasta que expire por sí solo (máx. 15 min) — no hay
  revocación instantánea de access tokens sin mantener una blacklist.
  Se acepta esta ventana de 15 min como trade-off de simplicidad.

## Rate limiting de login

Limiter en memoria (`internal/auth/ratelimit.go`), sin dependencias
externas (Redis, etc.): ventana deslizante de 5 intentos fallidos por
minuto, clave `IP+email`. Se resetea en cada login exitoso. Suficiente
para el target de despliegue de este proyecto (instancia self-hosted
única, no un cluster multi-réplica) — si el proyecto alguna vez corre
en múltiples réplicas, este limiter dejaría de ser efectivo (cada
réplica cuenta por separado) y habría que moverlo a un store
compartido.

## Explícitamente pendiente

- Recuperación de password (diferido a v2, requiere decidir SMTP).
- Autenticación de dos factores.
- Endpoints/esquema de datos de las 7 entidades de negocio
  (`esquema-datos/`) — auth es el primer feature implementado, el
  resto sigue pendiente de código.
