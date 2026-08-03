# Auth multi-usuario — Diseño

Estado: en diseño

## Modelo de datos (alto nivel)

- `users(id, email, password_hash, is_admin, created_at)`
- `devices(id, user_id, device_name, refresh_token_hash, refresh_token_expires_at, created_at, last_used_at)`
  — el refresh token se guarda hasheado, igual que el password, nunca
  en texto plano.

## Password: argon2id

Librería: `golang.org/x/crypto/argon2`, o un wrapper con encoding
estándar (p. ej. `alexedwards/argon2id`) que serializa salt y
parámetros en el mismo string `$argon2id$...` — facilita ajustar
parámetros de costo a futuro sin migración de esquema.

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
- Setup inicial: al levantar el contenedor con una DB vacía, un modo
  de bootstrap crea el primer usuario admin. Mecanismo exacto
  (variable de entorno con email/password inicial vs. wizard en primer
  acceso) — pendiente, ver `tasks.md`.
- Un admin crea usuarios adicionales vía endpoint/panel admin
  (`POST /admin/users`).

## Revocación de dispositivos

- `GET /devices` — lista los dispositivos del usuario autenticado
  (nombre, última actividad).
- `DELETE /devices/:id` — invalida el refresh token de ese dispositivo
  de inmediato. El access token ya emitido para ese dispositivo sigue
  siendo válido hasta que expire por sí solo (máx. 15 min) — no hay
  revocación instantánea de access tokens sin mantener una blacklist.
  Se acepta esta ventana de 15 min como trade-off de simplicidad.

## Explícitamente pendiente

- Recuperación de password.
- Autenticación de dos factores.
- Mecanismo exacto de bootstrap del primer admin (env var vs. wizard).
- Rate limiting de login.
- Esquema exacto de columnas/tipos de `users` y `devices`.
