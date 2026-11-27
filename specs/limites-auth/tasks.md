# Límites del server y del login — Tareas

Estado: implementado (PR de U-10, nov 2026).

- [x] `internal/security/bodylimit.go` + tests: 64 KiB en `/auth/*`,
      1 MiB en el panel, `MAX_BODY_MB` en el resto; `413` por
      `Content-Length`.
- [x] `internal/security/clientip.go` + tests: `TRUSTED_PROXIES`,
      `CLIENT_IP_HEADER`, recorrido de `X-Forwarded-For` desde la
      derecha, IPv4 mapeada en IPv6.
- [x] `internal/auth/ratelimit.go` + tests con reloj controlado: IP y
      email, IP, cuenta con espera exponencial, olvido y limpieza.
- [x] Handlers de login (API y panel): `Retry-After` en el `429`,
      contraseña de más de 1024 bytes como fallo.
- [x] Setup, alta de usuario (panel y API) y reset de contraseña:
      `400` con más de 1024 bytes.
- [x] `cmd/server/main.go`: `newHTTPServer` con timeouts, middlewares
      en orden, `parseBodyLimit`; tests en `cmd/server/main_test.go`.
- [x] `internal/realtime`: test de que `/ws` sobrevive a los timeouts.
- [x] `openapi.yaml`, `.env.example`, README (inglés y español), texto
      del panel.
- [ ] Configurar `TRUSTED_PROXIES=10.0.1.0/24` y
      `CLIENT_IP_HEADER=CF-Connecting-IP` en Gaia (Coolify), con el OK
      de Jorge, y comprobar en la lista de dispositivos del panel que se
      ve la IP real.
