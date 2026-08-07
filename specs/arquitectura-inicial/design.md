# Arquitectura inicial — Diseño

Estado: implementado — stack, resolución de conflictos (incluido CRDT
real para texto largo) y protocolo de sync completos. Ver
`sync-incremental/`, `esquema-datos/`, `auth-multiusuario/` para el
detalle de cada pieza.

## Stack: Go

De los lenguajes que maneja el autor (Python, Go, Java, Node), Go es la
elección por el criterio priorizado — ligereza para self-host, no
velocidad de desarrollo ni ecosistema:

- Compila a un único binario estático → imagen Docker mínima (`scratch`
  o `alpine`, ~15–30MB), sin runtime/VM que cargar.
- Bajo consumo de RAM en reposo y arranque en frío — el mismo perfil que
  hace viable a Vaultwarden (Rust) en hardware casero, con una curva de
  aprendizaje mucho más accesible.
- Concurrencia nativa simple (goroutines/channels) — encaja bien con el
  requisito de sync en tiempo real vía WebSocket.
- Precedente: Gitea, Miniflux y buena parte del ecosistema self-hosted
  "un binario + Docker" está en Go por esta misma razón.

### Componentes propuestos

- **HTTP**: `net/http` + `chi` como router liviano. Evitar frameworks
  pesados (Gin/Echo) si no aportan algo concreto.
- **Base de datos**: SQLite por defecto (archivo único, cero
  configuración), con la capa de acceso escrita para que Postgres sea
  intercambiable en instalaciones más grandes — `database/sql` +
  `sqlc` en vez de un ORM pesado. Driver: `mattn/go-sqlite3` (CGO).
  Nota histórica: se eligió cuando CGO parecía obligatorio de todas
  formas por `yrs` (ver "Resolución de conflictos" — decisión luego
  revertida a favor de una librería CRDT puro Go). La razón original
  ("no hay costo adicional, CGO ya está presente") ya no aplica, pero
  `mattn/go-sqlite3` se queda: sigue siendo el driver SQLite más
  maduro/rápido en Go, y no hay urgencia de migrar a
  `modernc.org/sqlite` solo para volver el binario 100% CGO-free sin
  un motivo concreto que lo justifique.
- **Migraciones**: `golang-migrate` o `goose`.
- **WebSocket**: `nhooyr.io/websocket` o `gorilla/websocket`, para el
  requisito de sync en tiempo real.
- **Auth**: tabla de usuarios (password hasheado con `bcrypt` o
  `argon2`), tokens de sesión por dispositivo (JWT de vida corta +
  refresh token de vida larga) — cada cliente es un "device" separado,
  no una sesión global. Aislamiento por `user_id` en cada tabla desde el
  esquema inicial.
- **Empaquetado**: Dockerfile multi-stage, `docker-compose.yml` de
  ejemplo con volumen para el SQLite/DB.

## Arquitectura local-first: el servidor como nodo, no autoridad

Principio que atraviesa todo el diseño (ver `requirements.md`): el
servidor de sync cumple el mismo rol que un remote de git — un punto de
encuentro opcional entre dispositivos, nunca la única copia válida de
los datos.

Implicaciones concretas para el diseño de endpoints y protocolo (a
resolver en el spec de sync):

- Cada cliente mantiene su propia copia local persistente completa
  (desktop: archivos/DB local ya existentes; web/móvil/extensión:
  almacenamiento local propio — IndexedDB, SQLite embebido, etc.).
- El servidor nunca es un requisito para leer o escribir localmente —
  solo para propagar cambios entre dispositivos del mismo usuario.
- El login inicial requiere red; después, el token se cachea localmente
  y la app sigue funcionando offline indefinidamente (solo se pausa el
  sync, no la app).

## Tiempo real: capa sobre el mismo protocolo de sync, no un sistema aparte

- Cada cliente conectado mantiene un WebSocket abierto mientras la app
  está activa.
- Al recibir una escritura, el servidor notifica a los demás
  dispositivos conectados del mismo usuario — un aviso liviano ("algo
  cambió: tipo, id, updated_at"), no necesariamente el payload completo.
- El cliente que recibe el aviso dispara el mismo endpoint de "cambios
  desde mi cursor" que usaría al reconectar tras estar offline — un solo
  mecanismo de reconciliación, no dos sistemas paralelos.
- Esto no cambia el modelo de conflictos elegido (pendiente) — el
  tiempo real solo reduce la latencia con la que los dispositivos ven el
  mismo estado ya resuelto.

## Mapeo de datos (punto de partida, no cerrado)

Repo de referencia (Logday Desktop): `/Users/jorgebuitrago/Developer/task-manager`
(`https://github.com/jorgebuitragor/logday.git`). Al diseñar el esquema
de datos, leer los tipos directamente de ahí en vez de asumir que el
resumen de abajo sigue vigente — pueden haber cambiado desde que se
escribió este spec.

Los tipos TS ya existentes en `task-manager` (`src/types/task.ts`,
`note.ts`, `overtime.ts`, `calendar.ts`, `absence.ts`) son el contrato de
referencia para las tablas del lado servidor. Cada tabla necesita como
mínimo `user_id`, `updated_at` y soft-delete (`deleted_at`) — es lo
mínimo que un sync incremental real necesita, y que el sync por git
actual no tiene (hoy "sync" = todo el repo o nada).

Casos particulares al migrar del formato archivo a tabla:

- **Daily entries**: hoy `Record<date, string>` con formato custom en
  disco (`## YYYY-MM-DD` + texto) — se mapea a
  `daily_entries(user_id, date, content, updated_at)`.
- **OvertimeEntry**: hoy un blob JSON dentro de un fence `---` (no
  frontmatter real) — pasa a ser una tabla normal.
- **Projects/Folders**: hoy son solo rutas de string sin metadata propia
  — pendiente decidir si en la API siguen siendo un campo string en cada
  Task/Note, o si pasan a ser una entidad propia con `id` (facilita
  renombrar/mover sin reescribir hijos).

## Resolución de conflictos

Estrategia mixta (ver `requirements.md`):

- **LWW por campo** para todo campo que no sea texto largo (status,
  fechas, números, booleanos, etc.) — cada campo compara su propio
  `updated_at`, no el del registro completo, para que ediciones
  concurrentes a campos distintos no se pisen entre sí.
- **CRDT acotado a texto largo**: solo `Note.content` y
  `daily_entries.content`. Es el único caso donde perder texto escrito
  offline en dos dispositivos es un riesgo real y donde un merge
  automático (en vez de "gana el más reciente") tiene sentido. No se
  extiende a otros campos sin decisión explícita.

### Librería e implementación: `Deln0r/ygo` (Go puro) — implementado

**Historial de la decisión** (documentado porque cambió dos veces, no
porque haga falta para entender el estado final, pero para que quede
registro de por qué):

1. Diseño original: [`yrs`](https://github.com/y-crdt/y-crdt) (puerto
   en Rust de Yjs) vía CGO, usando los bindings C oficiales
   (`yffi`/`libyrs`). Nunca se implementó — el riesgo de escribir
   bindings CGO/Rust a mano contra una API no verificada se evaluó dos
   veces como demasiado alto para resolver de paso al construir
   `note`/`daily_entries` con la simplificación LWW por fila mientras
   tanto (ver `esquema-datos/design.md`, histórico).
2. Antes de escribir esos bindings, se investigó la API real de
   `yffi` (verificada contra el código fuente actual, no de memoria):
   sí es viable — `yrs`/`yffi` v0.27.3, MIT, activamente mantenida,
   firmas de función confirmadas. Pero la misma investigación encontró
   que **ya existen librerías CRDT de texto en Go puro**,
   wire-compatibles con Yjs, que evitan CGO/Rust por completo:
   `Deln0r/ygo` y `reearth/ygo`.
3. Se eligió **`github.com/Deln0r/ygo`** (MIT, v1.15.0 al momento de
   integrarlo): coherente con el criterio que ya guio el resto de
   decisiones de este proyecto (Go sobre Rust por curva de
   aprendizaje, evitar CGO donde no haga falta). Con esto, la
   cross-compilación para Raspberry Pi/ARM vuelve a ser el
   `GOOS`/`GOARCH` normal que ya usa el resto del binario — no hizo
   falta el toolchain cruzado ni `tonistiigi/xx` que la vía CGO habría
   requerido. `mattn/go-sqlite3` sigue siendo la única pieza CGO del
   proyecto (ver "Base de datos" arriba), sin relación con CRDT.
   `reearth/ygo` no se descartó por un defecto concreto encontrado —
   no llegó a evaluarse en profundidad (ver nota abajo).

**Nota sobre el proceso**: la implementación inicial de `note` con
`Deln0r/ygo` fue escrita por un agente que se saltó su instrucción de
solo investigar sin tocar el repo, sin completar la comparación contra
`reearth/ygo` que se le había pedido. El código resultante se revisó
a fondo (compila, tests propios pasan, incluyendo el caso real que
importa: dos ediciones concurrentes offline se mezclan sin perderse) y
se decidió conservarlo en vez de descartarlo, dado que reescribir con
otra librería no tenía una razón concreta en contra de `Deln0r/ygo` —
solo evaluación pendiente. `daily_entries` se completó después con el
mismo patrón, ya de forma directa.

- **`internal/crdt`**: envuelve `Deln0r/ygo` en dos operaciones
  (`ApplyTextUpdate`, `Text`), sin conocimiento de dominio — mismo
  criterio que `internal/security`. `ApplyTextUpdate` es idempotente
  (reaplicar el mismo update no duplica texto) y nunca rechaza por
  staleness (los updates CRDT conmutan, a diferencia de LWW).
- **Rol del servidor**: no es un relay 100% opaco — persiste el estado
  CRDT compactado (`EncodeStateAsUpdate` tras cada merge) en
  `content_crdt BLOB`, no el log completo de updates sin límite.
- Endpoint dedicado por entidad (`POST /notes/:id/content`,
  `PUT /daily-entries/:date` — este último ya no tiene un endpoint de
  metadata separado, ver `esquema-datos/design.md`), separado de los
  campos LWW-por-fila, con el `content_update` viajando en base64.

## Integración de clientes y transición desde git

- **Orden**: desktop (`task-manager`) primero — es el único cliente
  que existe hoy con usuarios reales, y es el que necesita la
  transición desde el sync por git. Validar el protocolo completo
  contra un cliente y datos reales antes de invertir en clientes
  nuevos (web/móvil/extensión).
- **Transición**: reemplazo directo, no convivencia. El sync por git
  sigue intacto para quien no configure un servidor; configurar uno lo
  desactiva para ese usuario — coherente con que la API reemplaza el
  sync por git a largo plazo (`requirements.md`), no un sistema
  paralelo indefinido.
- No hace falta un import especial del historial de git: por la
  filosofía local-first, el cliente ya tiene su estado actual completo
  en disco. Configurar un servidor por primera vez es simplemente el
  primer push de ese estado actual vía el protocolo de
  [`sync-incremental`](../sync-incremental/design.md) — no un caso
  especial de migración.

## Explícitamente pendiente (specs futuros)

- Protocolo de sync incremental — ver
  [`sync-incremental/`](../sync-incremental/requirements.md).
- Payloads de WebSocket para tiempo real.
- Esquema de auth completo (tablas `users`, `devices`/`sessions`).
- Esquema de datos completo tabla por tabla.
- Migración de usuarios existentes del sync por git.
- Orden de integración de clientes.
