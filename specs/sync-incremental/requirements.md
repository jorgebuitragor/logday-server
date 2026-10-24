# Sync incremental — Requirements

Estado: implementado — pull (`GET /sync/changes`, con paginación
opcional vía `limit`), tiempo real (WS) y purga de tombstones con
invalidación de cursor implementados. Ver `design.md` para el detalle
de paginación y de cómo viaja el contenido CRDT.

## Contexto

Depende de las decisiones de [`arquitectura-inicial`](../arquitectura-inicial/requirements.md)
(stack Go, filosofía local-first, resolución de conflictos: LWW por
campo + CRDT acotado a texto largo). Este spec define el protocolo
concreto que un cliente usa para reconciliar su estado con el servidor,
tanto al reconectar tras estar offline como al recibir una notificación
en tiempo real.

## Requisitos (EARS)

### Escritura de cambios

- El sistema DEBERÁ exponer endpoints REST por entidad para que un
  cliente escriba cambios (`POST /<entidad>` para crear,
  `PATCH /<entidad>/:id` para editar campos LWW,
  `DELETE /<entidad>/:id` para soft-delete) — no un endpoint genérico
  de push por batch. Cada paquete de dominio (`internal/task`,
  `internal/note`, ...) expone los suyos siguiendo este mismo patrón.
  El `PATCH` parcial (en reemplazo del `PUT` de fila completa de v1) y
  la resolución por campo individual se especifican en
  [`lww-por-campo/`](../lww-por-campo/requirements.md) — este spec
  asume ese protocolo, no lo repite.
- El servidor DEBERÁ asignar `seq` en cada escritura — el cliente
  nunca lo envía ni lo controla, es un concepto puramente del servidor
  (cursor de sync).
- El cliente DEBERÁ enviar su propio `updated_at` (el momento real en
  que hizo la edición localmente, no cuándo llega al servidor) en cada
  escritura. Sin esto, "last-write-wins" no compara nada real — sería
  simplemente "gana quien llega último al servidor", que es
  exactamente el riesgo de pérdida silenciosa que se descartó al
  elegir LWW sobre "último que escribe gana" (ver
  `arquitectura-inicial/requirements.md`).
- El riesgo de reloj de dispositivo desincronizado en el cliente que
  determina `updated_at` es un riesgo aceptado explícitamente desde la
  decisión original de usar LWW (ver
  `arquitectura-inicial/requirements.md`) — no es nuevo aquí.

### Cursor

- El sistema DEBERÁ asignar a cada escritura un número de secuencia
  (`seq`) monótono creciente, único por usuario (no por tabla) — es la
  fuente de verdad de "qué tan al día" está un cliente.
- Un cliente DEBERÁ persistir localmente el último `seq` que procesó, y
  usarlo como cursor en la siguiente solicitud de cambios.

### Endpoint de cambios

- El sistema DEBERÁ exponer un único endpoint que devuelva todas las
  entidades modificadas con `seq` mayor al cursor solicitado, sin
  importar el tipo de entidad.
- Cada registro devuelto DEBERÁ incluir el tipo de entidad al que
  pertenece, para que el cliente pueda enrutarlo a su almacenamiento
  local correspondiente.
- El sistema DEBERÁ aceptar un `limit` opcional que acote la cantidad
  de cambios devueltos en una respuesta — sin él (o `0`), DEBERÁ
  comportarse exactamente igual que hoy (sin paginar). Un cliente que
  pida `limit` y reciba una página completa DEBERÁ poder seguir
  pidiendo la página siguiente con `since` = el `seq` del último
  elemento recibido, hasta agotar el delta.

### Deletes / tombstones

- Cuando una entidad se elimine, el sistema DEBERÁ marcarla como
  borrada (soft-delete) en vez de eliminarla físicamente, y DEBERÁ
  devolverla en el endpoint de cambios como cualquier otro cambio,
  señalizando que fue borrada.
- El sistema DEBERÁ purgar físicamente los tombstones con más de 90
  días de antigüedad.
- Cuando un cliente solicite cambios con un cursor anterior al punto de
  purga vigente, el sistema DEBERÁ rechazar la solicitud con una señal
  explícita de "cursor inválido" en vez de devolver un resultado
  incompleto.
- Al recibir la señal de "cursor inválido", un cliente DEBERÁ descartar
  su cursor local y solicitar el estado completo desde cero (full
  resync), preservando sus datos locales no sincronizados.

### Notificación en tiempo real

- Cada dispositivo conectado DEBERÁ suscribirse a un único canal
  WebSocket por usuario (no uno por tipo de entidad).
- Cuando el servidor procese una escritura, DEBERÁ notificar por ese
  canal a los demás dispositivos conectados del mismo usuario con un
  aviso liviano (tipo de entidad, id, `seq`), sin el registro completo.
- Al recibir un aviso, un cliente DEBERÁ reconciliar su estado
  solicitando `/sync/changes` desde su propio cursor — el mismo
  mecanismo que usaría al reconectar, no un camino separado para
  aplicar el payload del WebSocket directamente.
- El sistema NO DEBERÁ requerir el header `Authorization` en el
  handshake del WebSocket (los navegadores no permiten configurarlo en
  una conexión WS nativa). El cliente DEBERÁ autenticarse enviando un
  primer mensaje `{"type":"auth","token":"..."}` con su access token
  vigente; el servidor DEBERÁ cerrar la conexión si ese mensaje no
  llega a tiempo o el token no es válido.
- El servidor DEBERÁ enviar pings periódicos a cada conexión para
  detectar clientes caídos sin cierre limpio, y cerrar/liberar la
  conexión si no responde.

## Fuera de este spec (para esta misma feature más adelante)

- Estrategia de reconexión del lado del cliente (backoff) — es una
  decisión de cada cliente, no de este servidor; el servidor solo
  garantiza que el mismo mecanismo de reconciliación (`/sync/changes`)
  sirve tanto para reconexión como para el caso en tiempo real.

La forma en que viajan los updates CRDT dentro de `data` (snapshot
resuelto en `content_state`, no el update binario crudo) ya está
implementada y documentada en `design.md` — no quedó fuera de este
spec, solo faltaba escribirlo.
