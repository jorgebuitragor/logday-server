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
| `/setup`, `/admin/panel`, `/admin/panel/…`, `/admin/static/…` | panel | Las plantillas usan `<script>`, `<style>` y `onclick` inline. |
| todo lo demás | `default-src 'none'` | JSON (incluido `/admin/users`), `/health` y `/ws` nunca se renderizan como documento. |

`/application` no cuenta como `/app`: se compara la ruta exacta o el
prefijo `/app/`. El panel se enumera de forma explícita, en vez de
"todo `/admin`", para que un endpoint JSON futuro bajo `/admin` no
herede `'unsafe-inline'`.

### `/app`

```
default-src 'self'; script-src 'self'; style-src 'self';
style-src-attr 'unsafe-inline'; img-src 'self' data: https:;
font-src 'self' data:; connect-src 'self'; worker-src 'none';
frame-src 'none'; manifest-src 'self'; object-src 'none';
base-uri 'self'; form-action 'self'; frame-ancestors 'none'
```

**Qué protege y qué no.** Protege contra que un script inyectado se
*ejecute*: inline, por atributo de evento o con `eval`. No impide que un
script que ya corre saque datos: le quedan las imágenes (`img-src
https:`), la navegación (`location`, `window.open`; CSP no tiene
`navigate-to`) y canales que CSP no controla. `connect-src 'self'`
solo cierra `fetch`, XHR y WebSocket. Por eso esta CSP va junto con la
cookie `HttpOnly` del refresh token ([`sesion-cookie/`](../sesion-cookie/design.md)),
que es la que lo deja fuera del alcance de cualquier script.

- **Sin script inline ni `eval`.** `logday-web` no los necesita: Vite
  emite solo `<script type="module" src>` y CSS en archivo, y React
  aplica la prop `style` por CSSOM, que la CSP no restringe.
- **`style-src-attr 'unsafe-inline'`.** El primer intento usó
  `style-src 'self'` a secas, y la vista previa de las notas perdía el
  color y la alineación que guarda Desktop (Tiptap Color, Highlight y
  TextAlign serializan `<span style="…">` en el Markdown, y DOMPurify
  conserva el atributo). Un atributo `style` no ejecuta código ni usa
  selectores, así que no abre una vía para leer la página; los
  elementos `<style>` siguen bloqueados. Además, el sanitizador de
  `logday-web` (`renderMarkdown.ts`) deja pasar solo `color`,
  `background-color` y `text-align`, así que una nota no puede tapar la
  interfaz con `position:fixed` ni cargar recursos con `url()`.
- **`img-src https:`.** Las notas son Markdown y pueden embeber
  imágenes de cualquier sitio. Las imágenes por `http:` dejan de
  cargar (en HTTPS ya las bloqueaba el contenido mixto). Quitar `https:`
  exigiría servir las imágenes a través de un proxy del servidor.
- **`connect-src 'self'` a secas.** CSP Level 3 hace que `'self'` cubra
  `ws:`/`wss:` del mismo host en los navegadores actuales. Un primer
  borrador agregaba `ws://<Host>` y `wss://<Host>` tomados del pedido,
  pero detrás de un proxy que reescribe `Host` esas entradas no
  corresponden a la página, y además obligaban a validar un valor que
  controla el cliente. Se descartó.
- **`worker-src 'none'` y `frame-src 'none'`.** La app no usa workers
  ni iframes.
- **Sin `blob:` en `img-src`.** Las descargas (Excel, exportaciones)
  usan `blob:` para navegar con `<a download>`, no como imagen.

### Panel

`script-src 'self'; style-src 'self'`, sin `'unsafe-inline'`, y todo lo
demás (`connect-src`, `form-action`, `base-uri`) en `'self'`, con
`frame-ancestors 'none'` y `object-src 'none'`. Hasta el 20 oct 2026 el
panel necesitaba `'unsafe-inline'` porque las plantillas tenían
`<script>`, `<style>` y `onclick` inline. Ahora el CSS y el JS están en
`internal/auth/static/` (`panel.css`, `theme-init.js` y `panel.js`), y
`templates_test.go` impide que vuelva a entrar código inline. El panel ya tiene su
propia cookie `HttpOnly` y la recomendación de ponerlo detrás de
Cloudflare Access (ver [`panel-admin`](../panel-admin/design.md)).

## Cabeceras comunes

- `Referrer-Policy: same-origin`: un link dentro de una nota no filtra
  `/app/notes/<id>` al sitio de destino, y los pedidos al mismo origen
  conservan la cabecera `Origin`. Con `no-referrer`, el navegador manda
  `Origin: null` en los POST same-origin, lo que rompería cualquier
  comprobación de CSRF por `Origin` en el futuro.
- `X-Frame-Options: DENY`, además de `frame-ancestors 'none'`, para
  navegadores que no leen CSP.
- `Cross-Origin-Opener-Policy: same-origin` y una `Permissions-Policy`
  que apaga APIs que la app no usa.

## Revisión de seguridad

Un sub-agente revisó la rama de forma independiente. No encontró nada
de severidad alta. Se aplicaron sus hallazgos: la spec ya no promete que
un script que corre no pueda sacar datos; el panel se enumera de forma
explícita (antes `/admin/users`, un endpoint JSON, recibía la política
del panel); se quitaron `blob:` y el `Host` del pedido y se agregaron
`worker-src`/`frame-src 'none'`; `Referrer-Policy` pasó a
`same-origin`; y los tests comparan la política completa.

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
