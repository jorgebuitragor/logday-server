# LWW por campo — Requirements

Estado: implementado (`feature/lww-por-campo`, no mergeado a `develop`/`main` todavía)

## Contexto

`esquema-datos/requirements.md` ("Resolución de conflictos por campo")
ya asume que todo campo no-CRDT se resuelve por LWW individual — pero
`arquitectura-inicial/requirements.md` ("Resolución de conflictos") deja
constancia de que la v1 implementada es LWW por **fila completa**,
porque ni el esquema (un solo `updated_at` por fila) ni el cliente de
referencia (`task-manager`, que manda la fila entera en cada
escritura) soportan hoy timestamps por campo. Este spec cierra esa
brecha: completa lo que `esquema-datos` ya daba por sentado.

Motivación directa: sin esto, dos ediciones concurrentes a campos
*distintos* de la misma fila (ej. título en un dispositivo, estado en
otro, ambos offline) hacen que una pise a la otra entera, aunque no
haya colisión real de intención. El objetivo explícito es que el
cliente **no tenga que involucrar al usuario** para resolver esto —
mismo principio que Google Docs/Notion: mergear al nivel más chico con
significado propio, no comparar el objeto entero. Ver
`arquitectura-inicial/requirements.md` para por qué texto largo
(`Note.content`, `daily_entries.content`) ya resuelve esto vía CRDT y
queda fuera de este spec.

## Requisitos (EARS)

### Escritura parcial

- El sistema DEBERÁ exponer `PATCH /<entidad>/:id` como el único medio
  de editar campos LWW existentes, en reemplazo de `PUT` con fila
  completa. `POST` (creación) sigue mandando la fila completa — un
  recurso nuevo no tiene versiones previas de campos con las que
  compararse.
- El cliente DEBERÁ enviar únicamente los campos que efectivamente
  cambiaron en esa acción del usuario, junto con un único `updated_at`
  para esa escritura (el momento real de la edición local). No
  DEBERÁ reenviar campos sin cambios "por las dudas".
- Entidades sin campos LWW propios (`daily_entries`, cuyo único
  contenido es CRDT) NO DEBERÁN verse afectadas por este spec — su
  único endpoint de escritura sigue siendo el existente.

### Resolución por campo

- El sistema DEBERÁ almacenar un timestamp de última escritura **por
  campo LWW**, no solo por fila. Ver `design.md` para la forma de
  almacenamiento.
- Al recibir un `PATCH`, el sistema DEBERÁ evaluar cada campo del
  payload de forma independiente: si el `updated_at` de la escritura
  entrante es más reciente que el timestamp almacenado de ese campo
  puntual, DEBERÁ aplicarlo y actualizar el timestamp de ese campo; si
  no, DEBERÁ descartar ese campo específico y conservar el valor
  almacenado — sin afectar el resultado de los demás campos del mismo
  payload.
- El sistema NO DEBERÁ rechazar la escritura completa con un código de
  error cuando solo algunos campos pierdan el LWW — un `PATCH` con al
  menos un campo aplicado DEBERÁ responder 200, nunca 409. El 409 por
  fila completa (v1) queda obsoleto para estos endpoints.
- Cuando ningún campo del payload logre aplicarse (todos más viejos
  que lo almacenado), el sistema DEBERÁ igualmente responder 200 con
  el estado actual — desde la perspectiva del cliente, una escritura
  totalmente perdida y una parcialmente perdida se comunican de la
  misma forma (ver "Respuesta", el estado completo es lo único que
  importa).
- `tags` (campo lista) sigue el mismo mecanismo que cualquier otro
  campo LWW: se resuelve como un valor atómico (todo el array), no hay
  merge elemento por elemento — esto ya estaba decidido en
  `esquema-datos/requirements.md` y no cambia acá.

### Respuesta: el servidor manda, el cliente nunca decide

- Todo `PATCH` DEBERÁ responder con la fila completa resultante
  (todos los campos LWW, con los valores ya resueltos), no un diff de
  "qué se aplicó y qué no".
- El cliente DEBERÁ, sin excepción, sobreescribir su copia local de la
  entidad con la fila que devuelve el servidor tras un `PATCH` —
  incluidos los campos que el propio cliente no mandó en ese payload.
  No DEBERÁ existir una rama de código cliente-side distinta para
  "mi escritura fue rechazada" vs. "mi escritura se aplicó": el mismo
  manejo (reemplazar estado local por la respuesta) cubre ambos casos,
  y por eso el cliente no necesita mostrarle nada al usuario ni pedirle
  que elija.
- El sistema DEBERÁ avanzar `seq` (y por lo tanto notificar por
  `/sync/changes` y WebSocket a otros dispositivos) únicamente cuando
  al menos un campo cambió de verdad — un `PATCH` donde todos los
  campos pierden el LWW NO DEBERÁ generar un evento de sync espurio.

### Riesgo aceptado (no se resuelve en este spec)

- Dos escrituras concurrentes al **mismo campo** (no a campos
  distintos) siguen resolviéndose por LWW normal: una gana, la otra se
  descarta sin aviso ni historial de recuperación. Es el mismo riesgo
  ya aceptado para LWW por fila en v1, ahora acotado a un campo en vez
  de a la fila entera — no una regresión, una mejora del alcance.
  Guardar una copia recuperable de la escritura perdida queda
  explícitamente fuera de alcance: no lo hacen Notion ni Google Docs
  tampoco, y agregarlo contradice el objetivo de que el cliente no
  tenga que pensar en esto.

## Fuera de este spec

- Forma de almacenamiento del timestamp por campo (JSON vs. tabla
  aparte) — ver `design.md`.
- Cambios en el cliente de referencia (`task-manager`) para adoptar
  `PATCH` — ver spec `sync-servidor` en ese repo.
- Migración de datos existentes creados bajo el esquema de un solo
  `updated_at` por fila.
