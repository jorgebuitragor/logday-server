# Convenciones de código — Requirements

Estado: en diseño

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

### Linting

- El sistema DEBERÁ correr `golangci-lint` sobre todo el código,
  con un preset reforzado sobre el default (incluye `gosec` para
  detectar patrones inseguros, dado que el proyecto maneja passwords y
  tokens).
- El repositorio DEBERÁ exponer un target `make lint` para correr el
  linter localmente.
- El sistema DEBERÁ correr el linter en CI (GitHub Actions) en cada
  push y pull request contra `main`, fallando el build si hay errores.

## Fuera de este spec

- Convenciones de logging, manejo de errores, o config/env vars —
  se definen cuando se implemente la primera feature real, no aquí.
- Cobertura de tests obligatoria o mínima — no decidido.
