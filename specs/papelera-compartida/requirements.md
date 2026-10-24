# Papelera compartida entre servicios — Requirements

Estado: implementado (sin cambios de código) — desktop (`task-manager`)
y web (`logday-web`) ya implementaron cada uno su propia papelera
local (ver `specs/papelera-reciclaje/` y `specs/papelera-compartida/`
de esos repos respectivamente, commit `b8d5b57` para desktop). Las
preguntas que quedaban abiertas del lado servidor ya se decidieron
(ver "Decisiones" abajo): ninguna requiere tocar código acá, el
comportamiento actual (retención indefinida, sin hard-delete) queda
confirmado como el diseño final, no un accidente. Mobile sigue sin
implementar — no es una decisión pendiente, es trabajo de cliente sin
hacer.

## Contexto

`task-manager` ya implementó una papelera de reciclaje local para
Task, Note, OvertimeEntry y entradas de Daily: en vez de borrar el
archivo/registro al toque, lo guarda como snapshot en un directorio
`.trash/` local, con purga automática opcional a 60 días y vaciado
manual siempre disponible. Restaurar vuelve a mandar un `POST` de
creación con el mismo `id` — y el servidor ya lo revive correctamente
porque `upsertNote`/equivalentes de Task/Overtime hacen
`ON CONFLICT(id) DO UPDATE ... deleted_at = NULL` (confirmado leyendo
`internal/note/store.go`, `internal/task/store.go`,
`internal/overtime/store.go` antes de construir nada del lado
cliente).

Además, `task-manager` ya extendió esa papelera para quedar
**compartida entre instalaciones de Logday Desktop del mismo
usuario**: cuando un delete llega por `/sync/changes` desde OTRO
dispositivo (no un delete propio), también se captura un snapshot
local antes de purgar el archivo — sin ningún cambio de este lado, ya
que `/sync/changes` manda la fila completa de cada delete a todos los
dispositivos, y el historial completo está disponible desde
`since=0`.

`logday-web` ya implementó el mismo mecanismo (capturar snapshot al
borrar y al recibir un delete remoto, UI de listar/restaurar/purgar) —
ver `specs/papelera-compartida/requirements.md` de ese repo. Con eso,
cada servicio tiene su propia papelera local funcionando — no una
vista unificada entre servicios (ningún cliente pidió eso, y este
servidor no expone hoy ningún endpoint que lo permitiría sin volver a
diseñar el modelo). Lo que faltaba decidir del lado servidor (ver
"Decisiones" abajo) ya está resuelto: no hace falta tocar código acá.
Mobile sigue sin implementar nada de esto.

## Decisiones (ya tomadas)

- [x] **El servidor NO purga soft-deletes automáticamente — retención
  indefinida, a propósito.** Antes era así por ausencia de un job de
  limpieza, no por decisión; ahora es el diseño confirmado. Motivos:
  (1) el volumen es metadata/texto a escala de un usuario o equipo
  chico — no hay problema real de espacio que justifique la
  complejidad de un job de purga; (2) cualquier retención finita
  tendría que ser mayor o igual a la de **todos** los clientes,
  incluidos los que desactiven su purga automática local para guardar
  "para siempre" — coordinar eso entre clientes sin un canal para
  comunicar la config no es confiable, así que no purgar nunca es la
  única forma de no arriesgar perder la posibilidad de restaurar algo
  que un cliente todavía muestra en su papelera; (3) coherente con el
  principio local-first ya documentado en `arquitectura-inicial`: el
  servidor es un nodo de sync, no la fuente de verdad única, así que
  retener más historia de la estrictamente necesaria no compromete
  nada. Si en el futuro el volumen real de una instancia se vuelve un
  problema, esto se revisita con datos concretos, no especulando.
- [x] **No se agrega un endpoint de hard-delete.** "Vaciar papelera"
  sigue siendo 100% client-local (deja de mostrarse, nunca toca el
  servidor) — ningún cliente lo prometió como borrado real, y no hay
  un caso de uso concreto (espacio, cumplimiento) que lo pida hoy.
  Agregar un `DELETE` irreversible de verdad es superficie de riesgo
  real (borrado permanente) que no se justifica sin una necesidad
  puntual — si aparece una (por ejemplo, un pedido de "derecho al
  olvido" de un usuario real), se diseña como spec aparte en ese
  momento, no preventivamente.
- [x] **`daily_entry` no necesita nada nuevo acá.** Ya sincroniza
  normal del lado servidor y `logday-web` ya lo incluye en su
  papelera — la limitación que existía era específica de
  `task-manager` (ver `specs/sync-primer-sincronizacion` de ese repo,
  "mismatch de modelo local"), no del servidor ni de web. Sigue
  siendo prerequisito resolver eso en Desktop antes de que su
  papelera lo incluya ahí también, pero eso no bloquea nada de este
  lado.
