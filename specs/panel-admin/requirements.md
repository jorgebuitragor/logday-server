# Panel de administración web — Requirements

Estado: implementado — ver `design.md` y `tasks.md`

## Contexto

Depende de [`auth-multiusuario`](../auth-multiusuario/requirements.md) —
este spec no introduce un modelo de usuarios/dispositivos nuevo, agrega una
superficie HTML sobre el mismo modelo (`internal/auth`).

Hoy la única forma de administrar una instancia es la API JSON: el primer
admin se crea por variables de entorno (`ADMIN_EMAIL`/`ADMIN_PASSWORD`) o el
servidor rechaza arrancar, y cualquier usuario adicional requiere `curl`/
Postman contra `POST /admin/users` armando el body a mano. Este spec cubre
un panel web, servido por el mismo binario (sin frontend/SPA separado, mismo
enfoque que Vaultwarden), para que un equipo pueda operar la instancia sin
depender de quien sepa manejar la API cruda.

## Requisitos (EARS)

### Setup inicial

- Cuando el servidor arranque sin usuarios y sin `ADMIN_EMAIL`/
  `ADMIN_PASSWORD` seteadas, el sistema DEBERÁ arrancar de todos modos
  (no fallar) y servir una pantalla de configuración inicial.
- El sistema DEBERÁ seguir soportando el bootstrap por variables de
  entorno exactamente como hoy — la pantalla de configuración inicial es
  una alternativa, no un reemplazo.
- El sistema DEBERÁ garantizar que la pantalla de configuración inicial
  solo pueda crear un admin una única vez, incluso ante requests
  concurrentes, y DEBERÁ dejar de ser alcanzable en cuanto exista un
  usuario activo en la instancia.

### Administración de usuarios

- El sistema DEBERÁ permitir que un admin autenticado, desde el panel,
  liste todos los usuarios de la instancia (activos y dados de baja).
- El sistema DEBERÁ permitir que un admin cree usuarios nuevos desde el
  panel, con los mismos datos que ya acepta `POST /admin/users`.
- El sistema DEBERÁ permitir que un admin promueva o degrade el rol de
  admin de otro usuario.
- El sistema NO DEBERÁ permitir dejar la instancia sin ningún admin
  activo — degradar o dar de baja al último admin DEBERÁ rechazarse.
- El sistema DEBERÁ permitir que un admin dé de baja a un usuario
  (soft-delete, reversible) y lo restaure.
- Cuando un usuario sea dado de baja, el sistema DEBERÁ revocar
  inmediatamente todos sus dispositivos/sesiones activas.
- El sistema DEBERÁ permitir que un admin resetee la contraseña de otro
  usuario.
- Cuando se resetee la contraseña de un usuario, el sistema DEBERÁ
  revocar inmediatamente todos sus dispositivos/sesiones activas.
- Un usuario dado de baja NO DEBERÁ poder autenticarse, ni por el panel
  ni por la API de sync, hasta ser restaurado.

### Administración de dispositivos

- El sistema DEBERÁ permitir que un admin, desde el panel, vea los
  dispositivos de **cualquier** usuario de la instancia (a diferencia de
  `GET /devices`, que solo muestra los propios).
- El sistema DEBERÁ permitir que un admin revoque el dispositivo de
  cualquier usuario desde el panel.

### Sesión del panel

- El sistema DEBERÁ requerir autenticación de admin para acceder a
  cualquier pantalla del panel más allá de login/setup.
- La sesión del panel DEBERÁ ser independiente del modelo de
  device/refresh-token usado por los clientes de sync — ver `design.md`
  para la justificación.
- El sistema DEBERÁ revalidar en cada request si el usuario de la sesión
  sigue siendo un admin activo, no solo confiar en lo que la sesión
  decía al momento de emitirse.
- El sistema DEBERÁ proteger cada formulario del panel contra CSRF.

## Fuera de este spec

- Recuperación de contraseña **self-service** (requiere SMTP, sigue sin
  decidirse — ver `auth-multiusuario/requirements.md`). El reset de
  contraseña de este spec es **admin-asistido**, no self-service.
- Autenticación de dos factores para el panel.
- Configuración de SMTP, rotación de `JWT_SECRET`, o cualquier otro
  ajuste operativo más allá de usuarios/dispositivos — fuera de alcance
  hasta que exista una necesidad concreta.
- Purga de datos sincronizados (tasks, notes, etc.) al dar de baja un
  usuario — ver "Explícitamente pendiente" en `design.md`.
- Paginación del listado de usuarios/dispositivos — instancia
  self-hosted de equipo chico, no se espera necesitarla en v1.
