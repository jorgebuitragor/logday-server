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

Por HTTPS: `__Host-logday_refresh`, `Path=/`. Por HTTP plano (LAN):
`logday_refresh`, `Path=/auth`. En ambos casos `HttpOnly`,
`SameSite=Strict` y `Max-Age` = TTL del refresh token (configurable en
el panel, 30 días por defecto).

- **Prefijo `__Host-`**: el navegador solo acepta esa cookie si es
  `Secure`, tiene `Path=/` y no tiene `Domain`. Así un subdominio
  hermano (otro servicio bajo `*.jorgebuitrago.com`) no puede plantar
  una cookie con el mismo nombre que tape la nuestra ("cookie
  tossing"), que serviría para un login CSRF (meter a la víctima en la
  cuenta del atacante) o para dejarla sin poder iniciar sesión. El
  prefijo obliga a `Path=/`, así que la cookie viaja a todos los
  endpoints, pero el servidor solo la lee en `/auth/*` y con la
  cabecera de modo. Por HTTP el prefijo no está permitido; ahí la
  cookie va limitada a `/auth`.
- **Dos cookies con nuestro nombre** se tratan como "sin sesión": una de
  las dos fue plantada.
- **`Secure`** según `secureRequest`: `r.TLS != nil` o
  `X-Forwarded-Proto: https`. TLS termina en el reverse proxy, así que
  en producción es la cabecera del proxy la que lo dice. Confiar en ella
  solo decide el flag de esta cookie: quien mienta solo rompe la suya.
  Por HTTP plano en LAN no se marca `Secure`, porque el navegador no la
  guardaría. Mismo criterio que `setSessionCookie` del panel.
  `COOKIE_SECURE=always` salta la detección, para una cadena de proxies
  que no reenvía bien el esquema (por ejemplo, un CDN en modo TLS
  "flexible").

## CSRF

La cookie solo se lee si el pedido trae `X-Logday-Session: cookie`. Una
página de otro sitio no puede agregar una cabecera propia sin preflight
de CORS, y `CORSMiddleware` solo aprueba `Authorization` y
`Content-Type`. Además `SameSite=Strict` no manda la cookie en pedidos
cross-site. Lo peor que podría forzar otro sitio es un logout, y
también lo bloquean las dos capas.

## Rotación y pestañas

El refresh token sigue rotando en cada uso, con la detección de reuso de
siempre (revoca el device). Varias pestañas comparten la misma cookie,
así que el cliente serializa los refresh entre pestañas (Web Locks, ver
`specs/sesion-cookie/` en `logday-web`).

**Margen de reuso de 20 s (`refreshReuseLeeway`).** El primer borrador
no tenía período de gracia, para no debilitar la detección de robo. La
revisión de seguridad mostró que eso no alcanza: en modo cookie *cada
carga de página* rota el token (el access token ya no se guarda), y el
Web Lock se libera si la página se descarga. Un F5 durante un refresh
en vuelo (el servidor ya rotó, pero el `Set-Cookie` nunca llegó) haría
que la página nueva presente el token viejo y el servidor revocara una
sesión legítima. Ahora, durante los 20 s siguientes a una rotación,
presentar ese token rota el token *vigente* del device en vez de
revocar; pasado ese margen, el reuso revoca como antes. Es la misma idea
que el "reuse interval" de Auth0/Okta. El costo: un token robado todavía
se puede canjear dentro de esos 20 s. Además, si un refresh pierde el
compare-and-swap contra otro del mismo device, reintenta desde el hash
nuevo (hasta 5 veces) en vez de revocar: perder esa carrera significa
que otra rotación de esa misma sesión ganó hace milisegundos.

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

En modo cookie, **si ya hay cookie, gana la cookie**: el body solo
cuenta cuando todavía no existe. Si no, una segunda pestaña que conserva
el token viejo en `localStorage` lo presentaría después de que la
primera migró, y el servidor lo vería como reuso.

## Logout y login

- `POST /auth/logout` también revoca el device cuando el token ya estaba
  rotado (un refresh en vuelo al cerrar sesión).
- Un login en modo cookie con una cookie de sesión todavía válida
  (pista perdida en `localStorage`, recuperación fallida por red)
  revoca ese device viejo, para no dejar sesiones huérfanas que podrían
  llegar al límite de dispositivos.

## Revisión de seguridad

Un sub-agente revisó la rama de forma independiente. Se aplicaron sus
hallazgos: prefijo `__Host-` y rechazo de cookies duplicadas (cookie
tossing desde subdominios hermanos), la cookie gana sobre el body
(migración con dos pestañas), el margen de reuso (recarga durante un
refresh), el logout con un token ya rotado, el login que reemplaza la
sesión anterior, `COOKIE_SECURE=always` y un test de CORS que fija el
invariante del que depende la defensa CSRF. Quedan documentados sin
cambio: el access token (JWT sin estado) sigue valiendo hasta 15 min
tras el logout, y el export de cuenta incluye el hash del refresh
token.
