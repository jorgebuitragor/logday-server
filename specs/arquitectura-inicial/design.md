# Arquitectura inicial — Diseño

Estado: en diseño (decisiones de stack y de resolución de conflictos
tomadas; protocolo de sync pendiente — ver `requirements.md`)

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
  `sqlc` en vez de un ORM pesado. Driver: `mattn/go-sqlite3` (CGO) en
  vez de `modernc.org/sqlite` (puro Go) — dado que CGO ya es
  obligatorio en el build por `yrs` (ver más abajo), no hay motivo
  para pagar el costo de rendimiento del driver puro Go solo para
  evitar una dependencia CGO que de todas formas está presente.
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

### Librería e implementación: `yrs` vía CGO

- **Librería**: [`yrs`](https://github.com/y-crdt/y-crdt) — puerto en
  Rust de Yjs, mantenido por el mismo equipo, con bindings C oficiales
  (`yffi`/`libyrs`) pensados para consumirse desde otros lenguajes.
- **Rol del servidor**: no es un relay 100% opaco. El servidor Go
  consume `libyrs` vía CGO para poder compactar el log de updates en
  un snapshot periódicamente (sin compactación, el log crece sin
  límite — cada edición de texto genera un update). El merge de
  negocio del contenido sigue ocurriendo con la misma librería CRDT,
  no con lógica propia del servidor.
- **Por qué CGO y no WASM**: se evaluó correr `yrs` compilado a WASM
  vía `wazero` (runtime WASM puro Go, sin CGO) para mantener
  cross-compilación trivial. Se descartó porque no existe un target
  WASM oficial para `yffi` pensado para consumirse desde Go — tocaría
  compilarlo y mantenerlo a mano, con más riesgo que beneficio. CGO
  usa el artefacto oficial, con llamadas nativas y sin el overhead de
  marshaling a través de memoria lineal WASM.
- **Costo aceptado**: CGO complica la cross-compilación para
  Raspberry Pi/ARM (un target explícito del proyecto) porque ya no
  alcanza con `GOOS`/`GOARCH` — hace falta un toolchain C por
  arquitectura. Se resuelve con `docker buildx` + el helper
  [`tonistiigi/xx`](https://github.com/tonistiigi/xx) en el Dockerfile
  multi-stage, un patrón estándar para cross-compilar binarios Go con
  CGO por plataforma.

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
