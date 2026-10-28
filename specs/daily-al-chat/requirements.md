# Daily al chat del equipo — Requirements

Estado: en diseño.

## Contexto

Hoy cada cliente arma el mensaje del daily ("Buenos días. El día…") y
el usuario lo copia y lo pega a mano en el chat de su equipo. Este spec
cubre enviarlo directamente a ese chat, que es la primera de las
**integraciones opcionales** de Logday: funciones incluidas en el
producto, apagadas por defecto, que cada usuario activa si su empresa
las usa (ver la nota "Integraciones opcionales" del vault).

Proveedores soportados, todos por **webhook entrante**: Google Chat,
Slack y Microsoft Teams (flujo de Workflows). Un webhook entrante es una
URL que el chat le da al usuario. Quien la tenga puede publicar en ese
espacio, así que se trata como un secreto.

El envío lo hace el servidor, no los clientes:

- **Logday Web** no puede llamar al webhook: su CSP solo permite
  `connect-src 'self'` (`cabeceras-seguridad/`) y los proveedores no
  responden con CORS.
- **El envío programado** tiene que funcionar con las apps cerradas.
- **La URL del webhook** se guarda en un solo lugar y no se sincroniza a
  cada dispositivo.

Consecuencia: la integración requiere una cuenta con sync. Quien use
Desktop solo en local no la tiene.

Fuera de alcance:

- Google Calendar, para decidir cuándo enviar. Queda para un spec aparte.
- OpenProject. Se hará en Desktop, porque solo es accesible desde la red
  de la empresa.
- Varios destinos por usuario. En v1 hay uno solo.
- Recibir mensajes del chat.

## Requisitos (EARS)

### Disponibilidad

- R1. El sistema DEBERÁ leer la variable de entorno opcional
  `INTEGRATIONS_KEY` (32 bytes en base64) al arrancar. Si falta o es
  inválida, las integraciones NO DEBERÁN estar disponibles en la
  instancia, y el arranque NO DEBERÁ fallar por eso.
- R2. El sistema DEBERÁ tener en la Configuración de la instancia un
  interruptor "Permitir enviar el daily a chats externos", apagado por
  defecto, que el admin cambia desde el panel.
- R3. Mientras falte la clave o el interruptor esté apagado, los
  endpoints de la integración DEBERÁN responder 503 con un código de
  error propio, salvo `GET /integrations`, que DEBERÁ responder
  `available: false`. Los envíos programados NO DEBERÁN ejecutarse.

### Configuración por usuario

- R4. Cada usuario DEBERÁ poder configurar como máximo un destino de
  chat: proveedor (`google_chat`, `slack` o `teams`) y URL del webhook.
- R5. Cuando se guarda una URL, el sistema DEBERÁ validar que sea
  `https` y que su host y su ruta correspondan al proveedor elegido
  (lista fija por proveedor, ver el design). Si no corresponden, DEBERÁ
  rechazarla con 400 y sin hacer ninguna llamada de red.
- R6. El sistema DEBERÁ guardar la URL cifrada (AES-256-GCM con
  `INTEGRATIONS_KEY`). NUNCA DEBERÁ devolverla en una respuesta, ni
  escribirla en logs o en el export de datos. Solo DEBERÁ exponer una
  pista: el host y los últimos 4 caracteres.
- R7. Al activar la integración por primera vez, el usuario DEBERÁ
  aceptar un aviso de que el texto de sus dailys se enviará a un
  servicio de terceros. El sistema DEBERÁ guardar la fecha de esa
  aceptación y rechazar la activación con 400 si no viene.
- R8. Cuando el usuario borra la integración, el sistema DEBERÁ
  eliminar la URL cifrada y toda la configuración. Al borrar la cuenta,
  DEBERÁ eliminarse con ella.
- R9. Si la URL guardada no se puede descifrar (por ejemplo, porque
  cambió la clave), el sistema DEBERÁ marcar la integración como "hay
  que volver a configurar", sin intentar enviar.

### Envío manual

- R10. Cuando el usuario pulsa "Enviar al chat", el cliente DEBERÁ
  enviar al servidor el mismo texto que pondría el botón Copiar, junto
  con la fecha del daily. El servidor DEBERÁ publicarlo tal cual en el
  webhook configurado.
- R11. El sistema DEBERÁ rechazar con 400 un texto vacío o de más de
  4000 caracteres.
- R12. El sistema DEBERÁ limitar los envíos (manuales, de prueba y
  programados) a 20 por hora por usuario. Al pasarse, DEBERÁ responder
  429.
- R13. Cuando el proveedor responde con error o no responde en 10 s, el
  sistema DEBERÁ responder 502 con el código HTTP del proveedor (si lo
  hubo), y guardar el resultado como último envío.
- R14. El sistema NO DEBERÁ seguir redirecciones del proveedor.
- R15. El sistema DEBERÁ ofrecer un envío de prueba con un texto fijo,
  para que el usuario confirme que la URL funciona.
- R16. Tras cada envío, el sistema DEBERÁ guardar la hora, la fecha del
  daily enviado y el resultado (ok o el error), y DEBERÁ mostrarlos en
  `GET /integrations`.

### Envío programado (fase 3)

- R17. El usuario DEBERÁ poder activar un envío automático a una hora
  local fija (`HH:MM`), con su zona horaria IANA (por ejemplo,
  `America/Bogota`).
- R18. El envío programado DEBERÁ ocurrir solo en días hábiles: de lunes
  a viernes, sin festivos de Colombia y sin los días de ausencia del
  usuario. Es la misma regla que usa Desktop para el "día hábil
  anterior".
- R19. Si el daily de ese día no existe o está vacío, el sistema NO
  DEBERÁ enviar nada, y DEBERÁ guardar el resultado "omitido: sin
  daily".
- R20. Si ese daily ya se envió ese día (a mano o de forma programada),
  el sistema NO DEBERÁ volver a enviarlo.
- R21. El texto del envío programado DEBERÁ ser idéntico al que arma
  Desktop con Copiar para esa fecha: el día hábil anterior y el de hoy,
  con los nombres de día y mes en español.
- R22. Si el servidor estaba caído a la hora programada, DEBERÁ enviar
  al volver si no pasaron más de 30 minutos. Si pasaron más, DEBERÁ
  omitir el envío de ese día.

### Clientes

- R23. Desktop, Web y Mobile DEBERÁN mostrar una sección "Integraciones"
  en Opciones solo si `GET /integrations` responde `available: true`.
- R24. Los clientes DEBERÁN mostrar el botón "Enviar al chat" junto a
  Copiar solo si la integración está activa.
- R25. Ningún cliente DEBERÁ guardar la URL del webhook después de
  enviarla al servidor.
