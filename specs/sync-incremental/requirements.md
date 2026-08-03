# Sync incremental — Requirements

Estado: en diseño

## Contexto

Depende de las decisiones de [`arquitectura-inicial`](../arquitectura-inicial/requirements.md)
(stack Go, filosofía local-first, resolución de conflictos: LWW por
campo + CRDT acotado a texto largo). Este spec define el protocolo
concreto que un cliente usa para reconciliar su estado con el servidor,
tanto al reconectar tras estar offline como al recibir una notificación
en tiempo real.

## Requisitos (EARS)

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

## Fuera de este spec (para esta misma feature más adelante)

- Paginación del endpoint de cambios para usuarios con historiales
  grandes.
- Forma exacta en la que viajan los updates CRDT (`yrs`) dentro de este
  protocolo.
