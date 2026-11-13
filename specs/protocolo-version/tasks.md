# Versión de protocolo — Tareas

Estado: en progreso.

## Servidor

- [x] `internal/compat`: `Protocol`, `MinClientProtocol`, `ServerVersion`,
      `VersionHandler` y `RequireClientProtocol`, con pruebas.
- [x] `cmd/server/main.go`: `GET /version` y el middleware sobre las rutas
      de la API, con `POST /auth/logout` exento.
- [x] CORS: el preflight aprueba `X-Logday-Protocol`, con prueba.
- [x] `Makefile` y `Dockerfile` fijan `ServerVersion`.
- [x] `openapi.yaml`: sección "Versión de protocolo", `GET /version`,
      `ClientProtocol`, `UpgradeRequired` y `VersionResponse`.
- [x] Probado con el binario: `/version` responde, una cabecera inválida da
      `400`, sin cabecera pasa al login (`401`), `/app` y `/health` siguen
      igual.
- [ ] Pasar `--build-arg VERSION=<tag>` en el build de producción
      (Coolify), para que `/version` no diga `dev`.

## Clientes

- [ ] Mobile: cabecera, comprobación de `GET /version`, `426` y avisos
      (`logday-mobile`, `specs/sync-mobile`).
- [ ] Desktop: lo mismo. Sin él, cuando suba el mínimo, un Desktop viejo
      recibe `426` y no lo explica.
- [ ] Web: mandar la cabecera antes de subir `MinClientProtocol`.
