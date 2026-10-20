# Sesión por cookie HttpOnly — Design

Estado: implementado.

## Contrato

| | Modo body (Desktop, Mobile, Web standalone) | Modo cookie (Web embebida) |
|---|---|---|
| Activación | sin cabecera | `X-Logday-Session: cookie` |
| `/auth/login` | `refresh_token` en el JSON | `refresh_token: ""` + cookie `logday_refresh` |
| `/auth/refresh` | body `{refresh_token}` obligatorio | body opcional; token de la cookie, rota la cookie |
| `/auth/logout` (nuevo) | body `{refresh_token}`, revoca el device | cookie, revoca el device y borra la cookie |

El código vive en `internal/auth/session_cookie.go`; `login` y
`refresh` (`internal/auth/handlers.go`) solo deciden al final si el
token va en el body o en la cookie.

## La cookie

`logday_refresh`, `HttpOnly`, `SameSite=Strict`, `Path=/auth`,
`Max-Age` = TTL del refresh token (configurable en el panel, 30 días por
defecto).

- **`Path=/auth`**: el navegador la manda solo a los endpoints de auth,
  nunca a la API de datos.
- **`Secure`** según `secureRequest`: `r.TLS != nil` o
  `X-Forwarded-Proto: https`. TLS termina en el reverse proxy, así que
  en producción es la cabecera del proxy la que lo dice. Confiar en ella
  solo decide el flag de esta cookie: quien mienta solo rompe la suya.
  Por HTTP plano en LAN no se marca `Secure`, porque el navegador no la
  guardaría. Mismo criterio que `setSessionCookie` del panel.

## CSRF

La cookie solo se lee si el pedido trae `X-Logday-Session: cookie`. Una
página de otro sitio no puede agregar una cabecera propia sin preflight
de CORS, y `CORSMiddleware` solo aprueba `Authorization` y
`Content-Type`. Además `SameSite=Strict` no manda la cookie en pedidos
cross-site. Lo peor que podría forzar otro sitio es un logout, y
también lo bloquean las dos capas.

## Rotación y pestañas

El refresh token sigue rotando en cada uso, con la misma detección de
reuso de siempre (revoca el device). Varias pestañas comparten la misma
cookie, así que dos refresh simultáneos con el mismo valor revocarían la
sesión. Eso lo resuelve el cliente serializando los refresh entre
pestañas (Web Locks, ver `specs/sesion-cookie/` en `logday-web`). El
servidor no agrega un período de gracia: aceptar un token ya usado
debilitaría justo la detección de robo.

**Bug encontrado al validar (19 oct 2026), anterior a este cambio.**
El control negativo del e2e de pestañas (desactivar el Web Lock)
reprodujo dos refresh simultáneos con el mismo token. El segundo no se
trataba como reuso: respondía **500** (`UNIQUE constraint failed:
used_refresh_tokens.token_hash`) y no revocaba el dispositivo. El
handler lee el dispositivo fuera de la transacción de rotación, así que
los dos pedidos pasaban esa lectura. Corregido con un compare-and-swap:
el `UPDATE devices … WHERE refresh_token_hash = <viejo>` de
`rotateRefreshToken` no afecta filas si otro ya rotó, y eso vuelve como
`errRefreshTokenReused`, que el handler trata igual que un token ya
usado (revoca el device y responde 401). Afectaba también a Desktop y
Mobile.

## Migración

Un navegador que ya tenía el refresh token en `localStorage` lo manda
una vez en el body de `/auth/refresh` con la cabecera de modo cookie. El
servidor lo rota y desde ahí vive en la cookie. No hace falta volver a
iniciar sesión.
