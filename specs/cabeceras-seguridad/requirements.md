# Cabeceras de seguridad — Requirements

Estado: implementado.

## Contexto

La auditoría de calidad de `logday-web` del 22 sep 2026 (hallazgo
H-001) marcó como bloqueante que el access y el refresh token viven en
`localStorage`, donde cualquier script que corra en el origen los puede
leer. La corrección de fondo (refresh token en cookie `HttpOnly`) es
el spec hermano [`sesion-cookie/`](../sesion-cookie/requirements.md).
Este spec cubre la otra mitad: que un script inyectado no llegue a
ejecutarse en la página que tiene la sesión, y que si se ejecuta no
pueda mandar datos a otro host.

Hasta este cambio, `logday-server` no mandaba ninguna cabecera de
seguridad: ni CSP, ni `X-Frame-Options`, ni `Referrer-Policy`.

## Requisitos (EARS)

- El sistema DEBERÁ mandar en toda respuesta `X-Content-Type-Options:
  nosniff`, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`,
  `Cross-Origin-Opener-Policy: same-origin` y una `Permissions-Policy`
  que deshabilite cámara, micrófono, geolocalización, pagos y USB.
- Cuando la ruta sea `/app` o esté bajo `/app/`, el sistema DEBERÁ
  mandar una CSP que solo permita scripts y hojas de estilo del mismo
  origen, sin `'unsafe-inline'` para scripts ni elementos `<style>`, y
  sin `'unsafe-eval'`.
- La CSP de `/app` DEBERÁ permitir atributos `style` inline
  (`style-src-attr 'unsafe-inline'`), porque el editor de Logday
  Desktop guarda color, resaltado y alineación como
  `<span style="…">` dentro del Markdown de la nota.
- La CSP de `/app` DEBERÁ permitir imágenes de cualquier origen
  `https:`, `data:` y `blob:` (las notas pueden embeber imágenes
  externas), y DEBERÁ limitar `connect-src` al mismo origen, incluido
  el WebSocket `/ws` en `ws://` y `wss://`.
- El sistema NO DEBERÁ copiar a la CSP un header `Host` que no tenga
  forma de host (nombre o IP, puerto opcional, IPv6 entre corchetes).
- Cuando la ruta sea `/setup` o esté bajo `/admin`, el sistema DEBERÁ
  mandar una CSP para el panel que permita scripts y estilos inline
  (las plantillas los usan) y limite todo lo demás al mismo origen.
- Para cualquier otra ruta (API JSON, `/health`, `/ws`), el sistema
  DEBERÁ mandar una CSP que lo niegue todo.

## Fuera de alcance

- HSTS: TLS termina en el reverse proxy (ver
  [`arquitectura-inicial`](../arquitectura-inicial/requirements.md)),
  que es la capa que sabe si la instancia se sirve de verdad por HTTPS.
- Quitar `'unsafe-inline'` del panel admin: requiere mover los
  `<script>` y los `onclick` de las plantillas a archivos estáticos o
  nonces. Queda pendiente.
- La CSP del build standalone de `logday-web`: lo sirve otro host, que
  es quien tiene que mandarla.
