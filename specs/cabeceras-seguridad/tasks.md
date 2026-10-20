# Cabeceras de seguridad — Tareas

Estado: implementado.

## Decisiones

- [x] Middleware propio en `internal/security`, global, con la CSP elegida por ruta.
- [x] `/app` sin `'unsafe-inline'` para scripts ni `<style>`, sin `'unsafe-eval'`.
- [x] `style-src-attr 'unsafe-inline'` en `/app` por el formato que guarda Desktop.
- [x] `img-src https:` en `/app` (imágenes externas en notas); las `http:` dejan de cargar.
- [x] `connect-src 'self'` a secas, sin copiar el `Host` del pedido.
- [x] Panel enumerado de forma explícita; el resto de `/admin` recibe la política de API.
- [x] Panel con `'unsafe-inline'` hasta sacar el código inline de las plantillas.
- [x] Sin HSTS: es responsabilidad del reverse proxy.

## Implementación

- [x] `internal/security/headers.go`: `HeadersMiddleware` y las tres políticas.
- [x] `cmd/server/main.go`: `r.Use(security.HeadersMiddleware)` tras `middleware.Recoverer`.
- [x] `specs/README.md`: fila en el índice.

## Tests y validación

- [x] `internal/security/headers_test.go`: cabeceras comunes en toda respuesta; política completa de `/app` comparada entera; sin `'unsafe-inline'` fuera de `style-src-attr` ni `'unsafe-eval'`; `/application` no es `/app`; política del panel solo en sus rutas; política de API en `/admin/users`, `/admin`, `/setup/` y `/admin/panelx`.
- [x] Revisión de seguridad independiente (sub-agente), hallazgos aplicados.
- [x] Validación en vivo con `logday-web` embebido (ver `design.md`).

## Pendiente

- [ ] Quitar `'unsafe-inline'` del panel: mover los `<script>` y los `onclick` de `internal/auth/templates/` a un archivo estático o a nonces.
