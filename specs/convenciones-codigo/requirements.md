# Convenciones de código — Requirements

Estado: implementado — logging y manejo de errores quedaban
"diferidos a la primera feature real"; con 9 paquetes de dominio ya
construidos, la convención de facto es consistente en todos y nunca se
había escrito. Ver `design.md`.

## Contexto

Antes de implementar cualquier feature de negocio (esquema de datos,
sync, auth), se define cómo se organiza el código dentro del repo y
qué garantías automáticas de calidad corren en cada cambio — para no
improvisar la estructura entidad por entidad a medida que se escribe.

## Requisitos (EARS)

### Organización de paquetes

- El código de cada dominio/entidad (task, note, overtime,
  calendar_event, absence_day, daily_entry, auth, sync) DEBERÁ vivir en
  su propio paquete bajo `internal/`, agrupando ahí su handler HTTP,
  lógica y acceso a datos — no dividido por capa transversal
  (`handlers/`, `services/`, `repository/`) ni con capas tipo
  hexagonal/ports & adapters.
- Infraestructura compartida entre dominios (conexión a base de datos,
  wiring del router, middlewares transversales) DEBERÁ vivir en
  paquetes propios (p. ej. `internal/db`), separada de los paquetes de
  dominio.
- Primitivas genéricas sin conocimiento del dominio (hash de password,
  firma/verificación de JWT, generación de tokens opacos) DEBERÁN vivir
  en `internal/security`, no dentro del paquete de dominio que las usa
  primero — para que otro dominio futuro las reutilice sin depender de
  ese paquete completo.
- Dentro de un paquete de dominio, los archivos DEBERÁN seguir la
  convención de nombres documentada en `design.md` (`models.go`,
  `store.go`, `handlers.go`, etc.) cuando el contenido correspondiente
  exista — no es obligatorio tener todos los archivos, sí usar el
  nombre correcto para el que aplique.
- La superficie exportada de un paquete de dominio DEBERÁ limitarse a
  lo que otro paquete realmente necesita llamar; todo lo demás
  permanece sin exportar.

### Linting

- El sistema DEBERÁ correr `golangci-lint` sobre todo el código,
  con un preset reforzado sobre el default (incluye `gosec` para
  detectar patrones inseguros, dado que el proyecto maneja passwords y
  tokens).
- El repositorio DEBERÁ exponer un target `make lint` para correr el
  linter localmente.
- El sistema DEBERÁ correr el linter en CI (GitHub Actions) en cada
  push y pull request contra `main`, fallando el build si hay errores.

### Logging y manejo de errores

- El sistema DEBERÁ usar el paquete `log` de la librería estándar para
  todo logging — sin `slog` ni una librería estructurada de terceros.
- Un error de infraestructura (fallo de base de datos, I/O) DEBERÁ
  envolverse con `fmt.Errorf("<acción en minúscula>: %w", err)` al
  propagarse, para que `errors.Is`/`errors.As` sigan la cadena hasta
  el handler.
- Un caso de negocio esperable (no encontrado, prohibido para este
  usuario, conflicto de versión) DEBERÁ señalizarse con un error
  centinela propio del paquete (`errNotFound`, `errForbidden`,
  `errConflict`, sin exportar) en vez de comparar el mensaje de texto
  del error.
- El handler HTTP de cada dominio DEBERÁ mapear esos centinelas a su
  código HTTP específico vía `errors.Is`, con un `default` genérico
  (`500`, sin exponer el error interno) para cualquier otro error no
  anticipado.
- Un error de validación de entrada (campo requerido, formato
  inválido) DEBERÁ devolverse como `errors.New("<mensaje>")` plano
  desde una función `validate<X>Request`, distinto de los centinelas
  de negocio — se traduce a `400` con ese mensaje tal cual, no a
  través del mapeo de centinelas.

## Fuera de este spec

- Convenciones de config/env vars — no decidido, se define si hace
  falta.
- Cobertura de tests obligatoria o mínima — no decidido.
