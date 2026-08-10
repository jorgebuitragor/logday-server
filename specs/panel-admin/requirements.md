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
- El sistema DEBERÁ validar el formato del email en toda vía de creación
  de usuario (panel, API JSON, setup inicial) antes de aceptarlo — sin
  esto no tiene sentido el filtro de dominios permitidos, que asume un
  email con forma válida.
- El sistema DEBERÁ confirmar visualmente al admin cuando una acción del
  panel se completa (crear/promover/degradar/dar de baja/restaurar
  usuario, resetear contraseña, revocar device, guardar configuración),
  no solo cuando falla.
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
- El sistema DEBERÁ mostrar, junto a cada dispositivo, la IP de su
  conexión más reciente y un ícono que identifique aproximadamente su
  tipo (móvil, tablet, cliente API/CLI o escritorio/navegador por
  default) a partir del User-Agent registrado.

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
- El sistema DEBERÁ pedir confirmación explícita antes de ejecutar
  acciones destructivas o que afectan sesiones de otros usuarios (dar de
  baja, resetear password, revocar un device, promover/degradar admin,
  cerrar la sesión propia).

### Configuración de la instancia

- El sistema DEBERÁ permitir que un admin, desde el panel, configure el
  nombre de la instancia, la retención de tombstones (días) y el rate
  limit de login (intentos y ventana) — reemplazando lo que hoy son
  constantes fijas en el código.
- Los cambios a estos tres valores DEBERÁN aplicarse sin reiniciar el
  servidor.
- El sistema DEBERÁ validar rangos razonables para cada valor antes de
  guardarlo (ver `design.md` para los límites concretos).
- El sistema DEBERÁ ofrecer, desde el panel, generar un valor sugerido
  para `JWT_SECRET` — sin persistirlo ni aplicarlo en caliente (ver
  "Fuera de este spec": la rotación en caliente sigue sin implementarse
  a propósito).
- El sistema DEBERÁ permitir configurar, desde el panel, una lista de
  dominios de email permitidos para crear usuarios (vacío = cualquier
  dominio). Aplica a la creación de usuarios vía panel y vía API JSON;
  NO aplica al setup inicial del primer admin.
- El sistema DEBERÁ permitir configurar la longitud mínima de
  contraseña, aplicada de forma consistente en las cuatro vías que hoy
  crean o cambian una contraseña (setup, crear usuario por panel, crear
  usuario por API JSON, reset de contraseña).
- El sistema DEBERÁ permitir configurar la duración del access token,
  del refresh token de dispositivo, y de la sesión del panel de admin —
  reemplazando lo que hoy son constantes fijas en el código. Los cambios
  aplican al próximo login/refresh, sin invalidar sesiones ya emitidas.
- El sistema DEBERÁ permitir configurar un máximo de dispositivos
  simultáneos por usuario (0 = sin límite); alcanzado el máximo, un
  nuevo login se rechaza hasta que se revoque algún device existente.

## Fuera de este spec

- Recuperación de contraseña **self-service** (requiere SMTP, sigue sin
  decidirse — ver `auth-multiusuario/requirements.md`). El reset de
  contraseña de este spec es **admin-asistido**, no self-service.
- Autenticación de dos factores para el panel.
- **Rotación en caliente de `JWT_SECRET`**: el panel genera un valor
  sugerido para copiar a mano en `.env` (ver arriba), pero no lo persiste
  ni cambia la clave activa del proceso — sigue siendo una variable de
  entorno, leída una sola vez al arrancar. Cambiar esto requeriría mover
  la raíz de confianza de JWT de env var a base de datos, una decisión
  de arquitectura aparte que no se toma acá.
- Configuración de SMTP.
- Purga de datos sincronizados (tasks, notes, etc.) al dar de baja un
  usuario — ver "Explícitamente pendiente" en `design.md`.
- Paginación del listado de usuarios/dispositivos — instancia
  self-hosted de equipo chico, no se espera necesitarla en v1.
