# Límites del server y del login — Requirements

Estado: implementado — ver `design.md` y `tasks.md`.

## Contexto

U-10 de la auditoría de calidad y seguridad de nov 2026: el server no
limitaba el tamaño del cuerpo ni el tiempo de una conexión, y el rate
limit del login usaba la IP de la conexión. Detrás de un reverse proxy
esa IP es la del proxy, así que todos los clientes compartían un mismo
contador: cualquiera podía frenar el login de cualquier email (en Gaia,
los logs mostraban todas las peticiones desde `10.0.1.3`, el Traefik de
Coolify). El lanzamiento público invita a exponer el server a internet,
así que esto tenía que cerrarse antes.

Extiende el rate limit de [`auth-multiusuario`](../auth-multiusuario/design.md)
y la configuración del panel de [`panel-admin`](../panel-admin/requirements.md)
(intentos y ventana).

## Requisitos (EARS)

### Tamaño del cuerpo

- Cuando una petición a `/auth/*` traiga un cuerpo de más de 64 KiB, el
  sistema DEBERÁ rechazarla.
- Cuando una petición a `/setup` o `/admin/panel*` traiga un cuerpo de
  más de 1 MiB, el sistema DEBERÁ rechazarla.
- Cuando cualquier otra petición traiga un cuerpo de más de
  `MAX_BODY_MB` (16 MiB por defecto, entre 1 y 1024), el sistema DEBERÁ
  rechazarla.
- Si `Content-Length` ya supera el límite, el sistema DEBERÁ responder
  `413` sin leer el cuerpo. Si el cuerpo no trae largo, el sistema
  DEBERÁ cortar la lectura en el límite y responder `400`.

### Tiempos de conexión

- El sistema DEBERÁ cerrar una conexión que tarde más de 5 s en mandar
  las cabeceras, más de 1 min en mandar la petición entera, más de 2 min
  en recibir la respuesta, o que quede inactiva más de 2 min.
- El sistema NO DEBERÁ aplicar esos tiempos a un WebSocket de `/ws` ya
  abierto.

### IP del cliente

- Cuando `TRUSTED_PROXIES` esté vacío, el sistema DEBERÁ usar la IP de
  la conexión, como antes.
- Cuando la conexión venga de una IP o red de `TRUSTED_PROXIES`, el
  sistema DEBERÁ tomar la IP del cliente de `CLIENT_IP_HEADER`
  (`X-Forwarded-For` por defecto).
- Con `X-Forwarded-For`, el sistema DEBERÁ recorrer los saltos de
  derecha a izquierda y quedarse con el primero que no sea un proxy de
  confianza. Con otra cabecera, DEBERÁ leerla como una sola IP.
- Si la conexión no viene de un proxy de confianza, o la cabecera no
  trae una IP válida, el sistema DEBERÁ usar la IP de la conexión.
- El sistema DEBERÁ usar esa IP en el log de acceso, en el rate limit
  del login y en la IP que guarda de cada dispositivo.
- Si `TRUSTED_PROXIES` no se puede leer, el sistema NO DEBERÁ arrancar.

### Rate limit del login (API y panel)

Con `N` intentos y una ventana `V`, configurables en el panel:

- Cuando una IP acumule `N` fallos con un mismo email dentro de `V`, el
  sistema DEBERÁ responder `429` a esa IP y ese email hasta que el fallo
  más viejo salga de la ventana.
- Cuando una IP acumule `4N` fallos con cualquier email dentro de `V`,
  el sistema DEBERÁ responder `429` a esa IP.
- Cuando una cuenta acumule más de `N` fallos desde cualquier IP, el
  sistema DEBERÁ responder `429` a esa cuenta durante una espera que
  empieza en 1 s y se duplica con cada fallo nuevo, hasta 5 min.
- El sistema NO DEBERÁ bloquear una cuenta por tiempo indefinido.
- Cuando una cuenta pase 1 h sin fallos, el sistema DEBERÁ volver a
  contar sus fallos desde cero.
- Cuando un login sea exitoso, el sistema DEBERÁ borrar los fallos de esa
  IP con ese email y los de la cuenta, pero no los de la IP.
- Cada `429` DEBERÁ traer `Retry-After` con los segundos de espera,
  redondeados hacia arriba.
- El sistema DEBERÁ responder `429` antes de comprobar la contraseña,
  aunque sea correcta.

### Largo de la contraseña

- Cuando se cree un usuario o se cambie una contraseña (setup, panel y
  `POST /admin/users`) con más de 1024 bytes, el sistema DEBERÁ responder
  `400`.
- Cuando un login traiga una contraseña de más de 1024 bytes, el sistema
  DEBERÁ responder como a una contraseña incorrecta, sin calcular el
  hash, y contarlo como fallo.

## Fuera de este spec

- Rate limit de endpoints que no son login (refresh, sync).
- Un store compartido para el rate limit: el target sigue siendo una
  sola instancia.
- Fijar las imágenes del pipeline por digest (U-11).
