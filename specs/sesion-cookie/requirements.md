# Sesión por cookie HttpOnly — Requirements

Estado: implementado.

## Contexto

La auditoría de calidad de `logday-web` del 22 sep 2026 (hallazgo
H-001, bloqueante) marcó que el access y el refresh token viven en
`localStorage`: cualquier script que corra en el origen (una
dependencia comprometida, una extensión, un XSS futuro) los puede leer
y llevarse. El spec hermano [`cabeceras-seguridad/`](../cabeceras-seguridad/requirements.md)
reduce la probabilidad de que ese script corra; este cubre lo otro:
que aunque corra, no pueda leer el refresh token.

El access token (TTL corto) sigue viajando por `Authorization: Bearer`,
y el cliente lo guarda solo en memoria. El refresh token pasa a una
cookie `HttpOnly`, que JavaScript no puede leer.

Desktop, Mobile y el build standalone de Logday Web (otro origen) no
cambian: el modo cookie es opt-in por cabecera.

## Requisitos (EARS)

- Cuando `/auth/login` o `/auth/refresh` reciba la cabecera
  `X-Logday-Session: cookie`, el sistema DEBERÁ entregar el refresh
  token en la cookie `logday_refresh` (`HttpOnly`, `SameSite=Strict`,
  `Path=/auth`, `Max-Age` igual al TTL del refresh token) y DEBERÁ
  devolver `refresh_token` vacío en el JSON.
- La cookie DEBERÁ llevar `Secure` cuando el pedido llegue por HTTPS,
  directo (`r.TLS`) o a través del reverse proxy
  (`X-Forwarded-Proto: https`), y NO DEBERÁ llevarlo por HTTP plano
  (instancias en LAN).
- En modo cookie, `/auth/refresh` DEBERÁ aceptar un body vacío y tomar
  el token de la cookie. Si el body trae `refresh_token`, ese DEBERÁ
  tener prioridad (migración de un navegador que lo tenía en
  `localStorage`).
- Sin la cabecera `X-Logday-Session: cookie`, el sistema NO DEBERÁ leer
  la cookie (defensa CSRF), y DEBERÁ comportarse exactamente como antes.
- En modo cookie, sin cookie ni token en el body, `/auth/refresh`
  DEBERÁ responder 401.
- Cuando el refresh token de la cookie sea inválido, esté vencido o ya
  se haya usado, el sistema DEBERÁ borrar la cookie además de responder
  401. La detección de reuso (revocar el dispositivo) DEBERÁ seguir
  igual.
- El sistema DEBERÁ exponer `POST /auth/logout`, que revoque el
  dispositivo del refresh token presentado (body o, en modo cookie,
  cookie), borre la cookie en modo cookie y responda 204 siempre,
  también con un token desconocido o ya revocado.

## Fuera de alcance

- Cookie para el access token. Exigiría protección CSRF en todos los
  endpoints de datos; con el access token en memoria y TTL corto, el
  beneficio es menor.
- Modo cookie para el build standalone en otro origen: necesitaría
  `SameSite=None` y CORS con credenciales. Ese modo sigue con tokens en
  `localStorage`.
