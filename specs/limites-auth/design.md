# Límites del server y del login — Design

## Componentes

- `internal/security/bodylimit.go`: `BodyLimitMiddleware` y
  `BodyLimitFor(path, default)`. El límite se elige por prefijo de ruta.
- `internal/security/clientip.go`: `ParseClientIPConfig` (lee
  `TRUSTED_PROXIES` y `CLIENT_IP_HEADER`), `ClientIPConfig.ClientIP` y
  `ClientIPMiddleware`, que reemplaza `r.RemoteAddr` por la IP real.
- `internal/auth/ratelimit.go`: `loginLimiter` con `Check(ip, email)`,
  `RecordFailure(ip, email)` y `Reset(ip, email)`.
- `cmd/server/main.go`: `newHTTPServer` con los timeouts,
  `parseBodyLimit` (`MAX_BODY_MB`) y el orden de los middlewares.

## Decisiones

### Reemplazar `RemoteAddr` en vez de guardar la IP en el contexto

Así el log de acceso de chi, el rate limit y la IP de cada dispositivo
usan la misma IP sin tocar cada lugar. El middleware va antes de
`middleware.Logger`. Es lo que hace `middleware.RealIP` de chi, pero
ese cree las cabeceras de cualquiera: con él, un cliente elegiría su IP
y se saltaría el rate limit.

### Cabecera configurable

En Gaia, Cloudflare va delante de Traefik y Traefik no confía en sus
direcciones (no tiene `forwardedHeaders.trustedIPs`), así que descarta el
`X-Forwarded-For` que llega y escribe uno nuevo con la IP del borde de
Cloudflare. La IP real solo viene en `CF-Connecting-IP`. Por eso
`CLIENT_IP_HEADER` permite leer una cabecera de un solo valor. Solo es
segura si al proxy únicamente se llega por Cloudflare: en Gaia el
firewall limita 80/443 a los rangos de Cloudflare.

### Espera por cuenta, no bloqueo

Un bloqueo por cuenta deja a cualquiera que conozca un email sacar a su
dueño del sistema. La espera exponencial (1 s, 2 s, 4 s… hasta 5 min)
frena una adivinanza repartida entre muchas IPs a un intento cada 5 min
como mucho (288 al día, contra un hash argon2id), y al dueño lo hace
esperar 5 min en el peor caso.

### Límite por IP de 4N

Una oficina detrás de un NAT comparte IP: el límite por IP tiene que
dejar fallar a varias personas a la vez. Cuatro veces el límite por
email (20 por minuto con la configuración por defecto) alcanza, y corta
a quien prueba contraseñas contra muchas cuentas.

### Memoria

Las claves sin fallos recientes se borran al revisarlas, y una pasada por
minuto (al registrar un fallo) borra las que nadie volvió a revisar. Así
el estado solo guarda fallos dentro de la ventana, o cuentas con fallos
en la última hora.

### Tamaños

- 64 KiB en `/auth/*`: el login y el refresh caben de sobra, y nadie sin
  cuenta puede mandar más.
- 1 MiB en el panel: el formulario más grande es Configuración, con el
  texto de la política de datos.
- 16 MiB en el resto, configurable con `MAX_BODY_MB`: una nota puede
  traer un dibujo de Excalidraw con imágenes. Si un self-hoster lo
  necesita, lo sube sin recompilar.

### Timeouts y `/ws`

`net/http` borra los deadlines de la conexión cuando el upgrade de
WebSocket la toma (`hijackLocked` llama a `SetDeadline(time.Time{})`),
así que `ReadTimeout` y `WriteTimeout` no cortan `/ws`.
`TestServeWSOutlivesServerTimeouts` lo comprueba con timeouts cortos y
el `Logger` de chi delante.

### Contraseña de hasta 1024 bytes

argon2id tarda lo mismo con cualquier largo, así que no es un tema de
CPU: deja el límite explícito en vez de depender del límite del cuerpo.
1024 bytes caben de sobra para cualquier frase o gestor de contraseñas.

## Alternativas descartadas

- `middleware.RealIP` de chi: confía en las cabeceras de cualquiera.
- Bloquear la cuenta tras N fallos: ver "Espera por cuenta".
- Rate limit por IP en todos los endpoints: el resto exige sesión, y
  el riesgo de U-10 estaba en lo que no la exige.
