# Cabeceras de seguridad — Design

Estado: implementado.

## Dónde vive

`internal/security/headers.go`: `HeadersMiddleware`, montado en
`cmd/server/main.go` justo después de `middleware.Recoverer` y antes
del CORS opcional, así que cubre todas las respuestas, incluidos los
errores y los preflight.

## Tres políticas según la ruta

| Ruta | Política | Por qué |
|---|---|---|
| `/app`, `/app/…` | estricta | Es la página que tiene la sesión del usuario. |
| `/setup`, `/admin`, `/admin/…` | panel | Las plantillas usan `<script>`, `<style>` y `onclick` inline. |
| todo lo demás | `default-src 'none'` | JSON, `/health` y `/ws` nunca se renderizan como documento. |

`/application` no cuenta como `/app`: se compara la ruta exacta o el
prefijo `/app/`.

### `/app`

```
default-src 'self'; script-src 'self'; style-src 'self';
style-src-attr 'unsafe-inline'; img-src 'self' data: blob: https:;
font-src 'self' data:; worker-src 'self' blob:; manifest-src 'self';
object-src 'none'; base-uri 'self'; form-action 'self';
frame-ancestors 'none'; connect-src 'self' ws://<host> wss://<host>
```

- **Sin script inline ni `eval`.** `logday-web` no los necesita: Vite
  emite solo `<script type="module" src>` y CSS en archivo, y React
  aplica la prop `style` por CSSOM, que la CSP no restringe.
- **`style-src-attr 'unsafe-inline'`.** El primer intento usó
  `style-src 'self'` a secas, y la vista previa de las notas perdía el
  color y la alineación que guarda Desktop (Tiptap Color, Highlight y
  TextAlign serializan `<span style="…">` en el Markdown, y DOMPurify
  conserva el atributo). Un atributo `style` no ejecuta código ni usa
  selectores, así que no abre una vía para leer la página; los
  elementos `<style>` siguen bloqueados.
- **`img-src https:`.** Las notas son Markdown y pueden embeber
  imágenes de cualquier sitio. Deja las imágenes como posible canal de
  exfiltración (un script inyectado podría pedir
  `https://atacante/?t=…` como imagen), un riesgo que se acepta.
  `connect-src` sí queda limitado al mismo origen.
- **WebSocket explícito.** CSP Level 3 dice que `'self'` cubre `ws:`/
  `wss:` del mismo host, pero WebKit viejo solo aceptaba el esquema de
  la página. Se listan los dos esquemas con el `Host` del pedido porque,
  detrás de un proxy que termina TLS, el servidor no sabe cuál usa el
  navegador. Un `Host` que no valida contra la expresión regular de
  host no se copia: la política queda en `connect-src 'self'`.

### Panel

`script-src`/`style-src` con `'unsafe-inline'`, y todo lo demás
(`connect-src`, `form-action`, `base-uri`) en `'self'`, con
`frame-ancestors 'none'` y `object-src 'none'`. El panel ya tiene su
propia cookie `HttpOnly` y la recomendación de ponerlo detrás de
Cloudflare Access (ver [`panel-admin`](../panel-admin/design.md)).

## Cabeceras comunes

- `Referrer-Policy: no-referrer`: un link dentro de una nota filtraría
  `/app/notes/<id>` al sitio de destino.
- `X-Frame-Options: DENY`, además de `frame-ancestors 'none'`, para
  navegadores que no leen CSP.
- `Cross-Origin-Opener-Policy: same-origin` y una `Permissions-Policy`
  que apaga APIs que la app no usa.

## Validación en vivo (19 oct 2026)

Contra un `logday-server` local con `logday-web` de `develop` embebido:

- Recorrido de las 7 secciones (Tareas en Kanban y lista, Notas,
  Dailys, Ausencias, Calendario, Extras, Ajustes): cero violaciones de
  CSP, y el WebSocket `/ws` abre.
- Nota con imagen externa por `https:`, `<span style="color:red">` y
  `<img onerror>`: la imagen carga, el color se aplica, y el `onerror`
  ya lo quitaba DOMPurify.
- XSS simulado en la página: un `<script>` inline no corre (violación
  de `script-src-elem`), un `fetch` a otro origen falla
  (`connect-src`) y un `<style>` inyectado no se aplica
  (`style-src-elem`).
