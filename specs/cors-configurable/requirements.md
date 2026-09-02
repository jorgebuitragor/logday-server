# CORS configurable — Requirements

Estado: implementado.

## Contexto

Este servidor no implementa CORS a propósito (ver
`arquitectura-inicial/requirements.md`): `logday-web` se sirve
embebida bajo `/app` en el mismo binario, mismo origen que la API, así
que el navegador nunca necesita CORS. `webapp-embebida/requirements.md`
ya evaluó "CORS configurable en el servidor" contra "mismo origen" y
descartó CORS porque "rompe cero configuración obligatoria"
(`webapp-embebida/requirements.md:16-21`), dejando explícitamente
fuera de alcance "CORS real para un deploy de `logday-web` separado
del servidor" (`webapp-embebida/requirements.md:47-48`).

Este spec cubre justo ese caso que quedó fuera: alguien quiere
desplegar `logday-web` en un origen distinto (ej. un hosting estático)
hablando con una instancia de `logday-server` remota. La
reconciliación con la decisión previa es que CORS acá es **opt-in y
apagado por defecto** — sin configurar nada, el servidor se comporta
exactamente igual que hoy (cero headers CORS, cero superficie nueva).
No reabre "cero configuración obligatoria" para el modo embebido,
solo lo habilita para quien lo pide explícitamente.

Este spec es solo el lado servidor. Que `logday-web` sepa apuntar a un
servidor en otro origen (URL configurable, pantalla de conexión, modo
de build standalone) es trabajo aparte, no cubierto acá.

## Requisitos (EARS)

- El sistema DEBERÁ leer una variable de entorno opcional
  `CORS_ALLOWED_ORIGINS` (lista de orígenes exactos separados por
  coma) al arrancar.
- Si `CORS_ALLOWED_ORIGINS` no está seteada o queda vacía tras
  parsear, el sistema NO DEBERÁ agregar ningún header `Access-Control-*`
  a ninguna respuesta — comportamiento idéntico al actual.
- Cuando una request trae un header `Origin` que coincide exactamente
  con alguno de los orígenes configurados, el sistema DEBERÁ responder
  con `Access-Control-Allow-Origin` reflejando ese origen (nunca `*`)
  y `Vary: Origin`.
- Cuando una request trae un header `Origin` que NO coincide con
  ninguno configurado, el sistema NO DEBERÁ agregar headers
  `Access-Control-*` a esa respuesta.
- Cuando llega una request `OPTIONS` de preflight con un `Origin`
  permitido, el sistema DEBERÁ responder directamente `204` con
  `Access-Control-Allow-Origin`, `Access-Control-Allow-Methods`,
  `Access-Control-Allow-Headers` (incluye `Authorization` y
  `Content-Type`) y `Access-Control-Max-Age`, sin pasar la request al
  resto del router.
- El sistema NO DEBERÁ soportar `*` (wildcard) como origen permitido —
  solo orígenes exactos.
- El sistema NO DEBERÁ enviar `Access-Control-Allow-Credentials` — la
  API JSON autentica por `Authorization: Bearer`, no por cookies
  (`internal/auth/middleware.go`), así que el modo "credentials" de
  CORS no hace falta.

## Fuera de este spec

- Que `logday-web` (o cualquier otro cliente browser) sepa configurar
  y usar un servidor en otro origen — repo/spec aparte.
- Wildcard, regex, o matching por subdominio de orígenes — solo
  orígenes exactos por ahora, sin necesidad demostrada de algo más
  flexible.
- CORS para `/admin/panel` (usa cookies de sesión, no Bearer) — el
  panel de administración no está pensado para consumirse cross-origin
  y queda fuera de alcance; si algún día hiciera falta, es un caso
  distinto (necesitaría `Access-Control-Allow-Credentials` y las
  implicaciones de seguridad que eso trae).
