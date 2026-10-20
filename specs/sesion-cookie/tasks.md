# Sesión por cookie HttpOnly — Tareas

Estado: implementado.

## Decisiones

- [x] Opt-in por cabecera `X-Logday-Session: cookie`; sin ella, contrato idéntico al anterior.
- [x] Solo el refresh token va en cookie; el access token sigue por `Authorization: Bearer`.
- [x] Cookie `HttpOnly`, `SameSite=Strict`, `Path=/auth`; `Secure` por `r.TLS` o `X-Forwarded-Proto`.
- [x] La cabecera propia es la defensa CSRF: sin ella la cookie se ignora.
- [x] Sin período de gracia para refresh concurrentes: lo serializa el cliente.
- [x] `POST /auth/logout` nuevo, también útil para Desktop y Mobile (body).

## Implementación

- [x] `internal/auth/session_cookie.go`: cabecera, cookie, `refreshTokenFromRequest`, `logout`.
- [x] `internal/auth/handlers.go`: `login`/`refresh` en modo cookie; ruta `/auth/logout`.
- [x] `openapi.yaml`: cabecera `X-Logday-Session`, cambios en login/refresh y `/auth/logout`.
- [x] `specs/README.md`: fila en el índice.

## Tests

- [x] `internal/auth/session_cookie_test.go`: login en modo cookie sin token en el body y con los atributos de la cookie; login en modo body sin cambios; refresh rota la cookie; la cookie se ignora sin la cabecera; sin cookie es 401; migración desde un token en el body; reuso revoca y borra la cookie; logout revoca, borra y es idempotente; logout en modo body; `Secure` detrás de un proxy HTTPS.
- [x] `TestRotateRefreshTokenTwiceWithTheSameOldHashIsReuse` (store) y `TestConcurrentRefreshWithTheSameTokenIsReuse` (HTTP): una rotación que pierde la carrera es reuso, nunca 500.
- [x] Validación en vivo con `logday-web` embebido (ver el spec del lado Web).

## Pendiente

- [ ] Que Desktop y Mobile llamen a `POST /auth/logout` al desconectar, para revocar su device en vez de solo olvidar los tokens.
