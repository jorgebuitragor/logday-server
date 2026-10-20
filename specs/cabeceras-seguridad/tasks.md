# Cabeceras de seguridad — Tareas

Estado: implementado.

## Decisiones

- [x] Middleware propio en `internal/security`, global, con la CSP elegida por ruta.
- [x] `/app` sin `'unsafe-inline'` para scripts ni `<style>`, sin `'unsafe-eval'`.
- [x] `style-src-attr 'unsafe-inline'` en `/app` por el formato que guarda Desktop.
- [x] `img-src https:` en `/app` (imágenes externas en notas), aceptando ese canal.
- [x] Panel con `'unsafe-inline'` hasta sacar el código inline de las plantillas.
- [x] Sin HSTS: es responsabilidad del reverse proxy.

## Implementación

- [x] `internal/security/headers.go`: `HeadersMiddleware` y las tres políticas.
- [x] `cmd/server/main.go`: `r.Use(security.HeadersMiddleware)` tras `middleware.Recoverer`.
- [x] `specs/README.md`: fila en el índice.

## Tests y validación

- [x] `internal/security/headers_test.go`: cabeceras comunes en toda respuesta; política estricta en `/app` (sin `'unsafe-inline'` fuera de `style-src-attr`, sin `'unsafe-eval'`); `/application` no es `/app`; política del panel en `/setup` y `/admin/…`; política que niega todo en la API; un `Host` inválido no se copia a la CSP; hosts con puerto e IPv6 sí.
- [x] Validación en vivo con `logday-web` embebido (ver `design.md`).

## Pendiente

- [ ] Quitar `'unsafe-inline'` del panel: mover los `<script>` y los `onclick` de `internal/auth/templates/` a un archivo estático o a nonces.
