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

### Principio: las integraciones no dependen del sync

Logday es local-first: Desktop y Mobile funcionan sin servidor. Las
integraciones también, así que hay dos modos:

- **Modo local** (Desktop y Mobile): la configuración y la URL se
  guardan en el equipo, en el almacén seguro del sistema, y el envío
  sale desde el equipo. No hace falta servidor ni cuenta.
- **Modo cuenta** (con sync): el servidor guarda la configuración y la
  URL cifrada, y el envío sale desde el servidor. Sirve en todos los
  equipos de la cuenta, y el envío programado sale aunque las apps
  estén cerradas. Logday Web siempre usa este modo: su CSP solo permite
  `connect-src 'self'` (`cabeceras-seguridad/`) y los proveedores no
  responden con CORS, así que no puede llamar al webhook.

Este spec define el contrato común de los dos modos (proveedores,
validación, texto, reglas del envío programado) y la parte del servidor
(modo cuenta). La parte local de cada cliente va en el spec de ese
cliente, y cumple los mismos requisitos marcados como "ambos modos".

Fuera de alcance:

- Google Calendar, para decidir cuándo enviar. Queda para un spec aparte.
- OpenProject. Se hará en Desktop, porque solo es accesible desde la red
  de la empresa.
- Varios destinos por usuario. En v1 hay uno solo por modo.
- Recibir mensajes del chat.

## Requisitos (EARS)

### Modos

- R1. Desktop y Mobile DEBERÁN ofrecer la integración en modo local,
  tengan o no una sesión de sync.
- R2. Cuando el cliente tiene sesión de sync y el servidor responde
  `available: true`, el cliente DEBERÁ dejar elegir dónde se guarda la
  integración: "En tu cuenta" (por defecto) o "Solo en este equipo".
  Sin sesión, o con `available: false`, DEBERÁ usar el modo local sin
  preguntar.
- R3. Si existen las dos configuraciones, el envío DEBERÁ usar la de la
  cuenta.
- R4. Cuando el usuario pasa de local a cuenta, el cliente DEBERÁ subir
  la configuración al servidor y borrar la local, incluida la URL.
- R5. Web DEBERÁ usar solo el modo cuenta. Si el servidor responde
  `available: false`, NO DEBERÁ mostrar la integración.

### Disponibilidad del modo cuenta (servidor)

- R6. El sistema DEBERÁ leer la variable de entorno opcional
  `INTEGRATIONS_KEY` (32 bytes en base64) al arrancar. Si falta o es
  inválida, el modo cuenta NO DEBERÁ estar disponible en la instancia, y
  el arranque NO DEBERÁ fallar por eso.
- R7. El sistema DEBERÁ tener en la Configuración de la instancia un
  interruptor "Permitir enviar el daily a chats externos desde el
  servidor", apagado por defecto, que el admin cambia desde el panel.
- R8. Mientras falte la clave o el interruptor esté apagado, los
  endpoints de la integración DEBERÁN responder 503 con un código de
  error propio, salvo `GET /integrations`, que DEBERÁ responder
  `available: false`. Los envíos programados del servidor NO DEBERÁN
  ejecutarse. El interruptor no afecta al modo local.

### Configuración (ambos modos)

- R9. Cada usuario DEBERÁ poder configurar como máximo un destino de
  chat por modo: proveedor (`google_chat`, `slack` o `teams`) y URL del
  webhook.
- R10. Al guardar una URL, DEBERÁ validarse que sea `https` y que su
  host y su ruta correspondan al proveedor elegido (lista fija, ver el
  design). Si no corresponden, DEBERÁ rechazarse sin hacer ninguna
  llamada de red.
- R11. La URL NUNCA DEBERÁ mostrarse después de guardarla, ni escribirse
  en logs, archivos de datos, exports o el sync. Solo se muestra una
  pista: el host y los últimos 4 caracteres.
  - En el servidor, se guarda cifrada con AES-256-GCM y
    `INTEGRATIONS_KEY`.
  - En Desktop, en el llavero del sistema operativo.
  - En Mobile, en `expo-secure-store`.
- R12. Al activar la integración por primera vez en un modo, el usuario
  DEBERÁ aceptar un aviso de que el texto de sus dailys se enviará a un
  servicio de terceros. DEBERÁ guardarse la fecha de esa aceptación.
- R13. Cuando el usuario quita la integración, DEBERÁN borrarse la URL
  y toda la configuración de ese modo. En el servidor, también al borrar
  la cuenta.
- R14. Si la URL guardada no se puede leer (por ejemplo, porque cambió
  la clave del servidor o se borró del llavero), la integración DEBERÁ
  marcarse como "hay que volver a configurar", sin intentar enviar.

### Envío manual (ambos modos)

- R15. Cuando el usuario pulsa "Enviar al chat", DEBERÁ enviarse el
  mismo texto que pondría el botón Copiar para ese daily.
- R16. DEBERÁ rechazarse un texto vacío o de más de 4000 caracteres.
- R17. Los envíos (manuales, de prueba y programados) DEBERÁN limitarse
  a 20 por hora por usuario en el servidor, y por equipo en modo local.
  En el servidor, el exceso responde 429.
- R18. Cuando el proveedor responde con error o no responde en 10 s,
  DEBERÁ informarse el fallo con el código HTTP del proveedor (si lo
  hubo), y guardarse el resultado como último envío. En el servidor, la
  respuesta es 502.
- R19. NO DEBERÁN seguirse redirecciones del proveedor.
- R20. DEBERÁ ofrecerse un envío de prueba con un texto fijo, para que
  el usuario confirme que la URL funciona.
- R21. Tras cada envío, DEBERÁN guardarse la hora, la fecha del daily
  enviado y el resultado (ok o el error), y DEBERÁN mostrarse en la
  pantalla de Integraciones.

### Envío programado

- R22. El usuario DEBERÁ poder activar un envío automático a una hora
  local fija (`HH:MM`), con su zona horaria IANA (por ejemplo,
  `America/Bogota`).
- R23. Dónde corre:
  - en modo cuenta, en el servidor, con las apps cerradas;
  - en modo local, en Desktop mientras la app está abierta (la pantalla
    DEBERÁ decirlo);
  - en Mobile no hay envío programado local, porque iOS no garantiza
    tareas en segundo plano a una hora fija.
- R24. El envío programado DEBERÁ ocurrir solo en días hábiles: de lunes
  a viernes, sin festivos de Colombia y sin los días de ausencia del
  usuario. Es la misma regla que usa Desktop para el "día hábil
  anterior".
- R25. Si el daily de ese día no existe o está vacío, NO DEBERÁ
  enviarse nada, y DEBERÁ guardarse el resultado "omitido: sin daily".
- R26. Si ese daily ya se envió ese día por ese modo (a mano o de forma
  programada), NO DEBERÁ volver a enviarse.
- R27. El texto del envío programado DEBERÁ ser idéntico al que arma
  Desktop con Copiar para esa fecha: el día hábil anterior y el de hoy,
  con los nombres de día y mes en español.
- R28. Si a la hora programada el servidor estaba caído o Desktop
  cerrado, DEBERÁ enviarse al volver si no pasaron más de 30 minutos. Si
  pasaron más, DEBERÁ omitirse el envío de ese día.

- R29. Mientras exista una integración en la cuenta, el modo local NO
  DEBERÁ hacer envíos programados, para no enviar dos veces. La
  pantalla DEBERÁ deshabilitar esa opción y explicar por qué.

### Clientes

- R30. Desktop y Mobile DEBERÁN mostrar siempre la sección
  "Integraciones" en Opciones. Web, solo si `GET /integrations` responde
  `available: true`.
- R31. Los clientes DEBERÁN mostrar el botón "Enviar al chat" junto a
  Copiar solo si hay una integración activa en algún modo.
- R32. Ningún cliente DEBERÁ guardar en sus archivos de datos ni en el
  almacenamiento normal la URL del webhook. En modo cuenta, NO DEBERÁ
  guardarla en absoluto después de enviarla al servidor.
