# Papelera compartida entre servicios — Requirements

Estado: pendiente — bloqueado a propósito hasta que desktop, web (y
mobile, si aplica) estén alineados en el mismo modelo de papelera. Ver
`task-manager` (Logday Desktop) `specs/papelera-reciclaje/` para la
implementación de referencia ya construida ahí (commit
`b8d5b57`, rama `feature/sync-servidor`).

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

Lo que falta para que la papelera sea de verdad compartida entre
**servicios** (no solo entre instalaciones desktop) es: que
`logday-web` implemente el mismo mecanismo (capturar snapshot al
borrar y al recibir un delete remoto, UI de listar/restaurar/purgar) —
ver spec equivalente en ese repo — y decidir si este servidor necesita
algo nuevo para soportarlo bien.

## Preguntas abiertas (PENDIENTE DE DECISIÓN)

- **¿El servidor debe seguir reteniendo soft-deletes para siempre?**
  Hoy lo hace, pero por ausencia de un job de limpieza, no por una
  decisión documentada — un cliente que reconstruye su papelera desde
  `/sync/changes since=0` depende implícitamente de que ningún soft-
  delete se purgue solo del lado servidor. Si en algún momento se
  agrega una purga automática acá, tiene que ser >= la retención más
  larga que cualquier cliente use para su propia papelera local (hoy,
  60 días en desktop) — o los clientes perderían la posibilidad de
  restaurar algo que el servidor ya tiró.
- **¿Hace falta un endpoint de hard-delete real?** Hoy "vaciar la
  papelera" en desktop solo deja de mostrar el ítem localmente — el
  registro sigue soft-deleted en la base de datos del servidor para
  siempre. Si se quiere que "vaciar papelera" purgue de verdad (por
  ejemplo, por espacio o cumplimiento), hace falta un endpoint nuevo
  (`DELETE` definitivo, distinto del soft-delete actual) que hoy no
  existe en ninguna entidad.
- **¿`daily_entry` entra en esto?** Hoy no tiene ningún sync con el
  servidor (ver `specs/sync-primer-sincronizacion` de `task-manager`,
  "mismatch de modelo local") — una papelera compartida para dailys
  necesitaría resolver eso primero, es un prerequisito de este spec,
  no parte de él.
