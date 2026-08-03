# Auth multi-usuario — Requirements

Estado: en diseño

## Contexto

Depende de [`arquitectura-inicial`](../arquitectura-inicial/requirements.md)
(multi-usuario desde v1, aislamiento por usuario, sesión independiente
por dispositivo/cliente). Este spec define cómo se autentica un usuario
y cómo se gestionan sus sesiones por dispositivo.

## Requisitos (EARS)

### Registro de usuarios

- El sistema NO DEBERÁ exponer un endpoint de registro público de
  cuentas.
- El sistema DEBERÁ permitir que un administrador cree usuarios nuevos
  en la instancia.
- El sistema DEBERÁ permitir crear el primer usuario administrador
  durante el setup inicial de una instancia vacía, sin depender de que
  ya exista un admin previo.

### Password

- El sistema DEBERÁ almacenar únicamente el hash de la contraseña,
  nunca el valor en texto plano.
- El sistema DEBERÁ usar **argon2id** como algoritmo de hash de
  password.

### Tokens y sesiones por dispositivo

- Cuando un dispositivo se autentique, el sistema DEBERÁ emitir un
  access token (JWT) de vida corta y un refresh token de vida más
  larga, asociados a ese dispositivo específico — no a una sesión
  global del usuario.
- El access token DEBERÁ expirar a los 15 minutos de emitido.
- El refresh token DEBERÁ expirar a los 30 días de su último uso (el
  TTL se extiende en cada refresh exitoso, no es fijo desde la emisión
  inicial).
- Cuando un cliente use un refresh token para obtener un access token
  nuevo, el sistema DEBERÁ invalidar el refresh token usado y emitir
  uno nuevo (rotación).
- Cuando el sistema detecte el reintento de un refresh token ya
  invalidado, DEBERÁ interpretarlo como posible robo de token y
  revocar toda la sesión del dispositivo asociado.

### Revocación manual

- El sistema DEBERÁ permitir que un usuario liste sus dispositivos/
  sesiones activas.
- El sistema DEBERÁ permitir que un usuario revoque manualmente la
  sesión de cualquiera de sus dispositivos.

## Fuera de este spec

- Recuperación de password (depende de si la instancia tiene SMTP
  configurado — no decidido).
- Autenticación de dos factores.
- Roles/permisos granulares más allá de admin vs. usuario normal.
- UI concreta de "dispositivos conectados" en cada cliente — decisión
  de cada cliente, no del servidor.
- Mecanismo exacto de bootstrap del primer admin y rate limiting de
  login — ver `tasks.md`.
