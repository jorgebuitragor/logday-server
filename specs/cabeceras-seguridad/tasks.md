# Cabeceras de seguridad — Tareas

Estado: implementado.

## Decisiones

- [x] Middleware propio en `internal/security`, global, con la CSP elegida por ruta.
- [x] `/app` sin `'unsafe-inline'` para scripts ni `<style>`, sin `'unsafe-eval'`.
- [x] `style-src-attr 'unsafe-inline'` en `/app` por el formato que guarda Desktop.
- [x] `img-src https:` en `/app` (imágenes externas en notas); las `http:` dejan de cargar.
- [x] `connect-src 'self'` a secas, sin copiar el `Host` del pedido.
- [x] Panel enumerado de forma explícita; el resto de `/admin` recibe la política de API.
- [x] ~~Panel con `'unsafe-inline'`~~: desde el 20 oct 2026 el panel no tiene código inline y su CSP es `script-src 'self'; style-src 'self'`.
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

- [x] Quitar `'unsafe-inline'` del panel (20 oct 2026): CSS en `internal/auth/static/panel.css`, el tema inicial en `theme-init.js` (bloqueante en el `<head>`) y el resto en `panel.js` (`defer`, con delegación de eventos y atributos `data-*` en vez de `onclick`). `serveStatic` los sirve con lista explícita de nombres. `templates_test.go` falla si una plantilla vuelve a tener `<script>` sin `src`, `<style>`, `style=` u `on*=`. Verificado en el navegador: login, usuarios (crear, resetear contraseña, confirmar), dispositivos y configuración, sin violaciones de CSP.
