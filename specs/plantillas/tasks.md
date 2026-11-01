# Plantillas por empresa — Tasks

Estado: en diseño. Nada implementado todavía.

Orden: primero lo que fija el contrato y los archivos (T1–T5), después el
servidor y los tres clientes en paralelo, cada uno en su repo.

## Antes de implementar

- [ ] T1. Verificar que Desktop, Web y Mobile, en sus versiones publicadas,
  ignoran un `entity_type` desconocido en `/sync/changes` y en `/ws` (P25).
  Si alguno falla, agregar `?types=` al contrato (design §1,
  "Compatibilidad").
- [ ] T2. Jorge decide los pendientes de `requirements.md`: a qué usuarios
  activa la FCV la migración del servidor y si la plantilla por instancia
  queda para v2.
- [ ] T3. `logday-design`: el texto de la plantilla genérica del Daily (es y
  en), el `.xlsx` genérico y la sección "Plantillas" de Opciones. Lo
  aprueba Jorge.
- [ ] T4. Hacer el `.xlsx` FCV con marcadores a partir de la salida de
  `generateOvertimeXlsx`, y elegir la librería de zip (JSZip o fflate) que
  sirva en los tres clientes. Guardar los dos `.xlsx` en
  `assets/templates/` y anotar sus hashes en `design.md`.
- [ ] T5. Escribir los casos de prueba compartidos (entradas y salidas
  esperadas del Daily y del Excel) que los tres clientes y Go usan igual,
  en `specs/plantillas/casos/`.

## logday-server

- [ ] T6. Migración: tabla `templates` (P23, design §1).
- [ ] T7. `internal/template/`: modelo, store, `GET`, `PUT` y `DELETE`, con
  las validaciones de P26 y la exigencia de todos los campos. Tests.
- [ ] T8. Sumar `template` a `/sync/changes`, `/ws`, `/account/export` y al
  borrado de la cuenta (P29). Tests.
- [ ] T9. Migración de datos: las filas FCV para los usuarios existentes
  (P8), con `ON CONFLICT DO NOTHING`. Test con un usuario que ya tiene una
  fila.
- [ ] T10. `openapi.yaml`: `Template`, `TemplateWrite`, las rutas y el
  `entity_type` nuevo.

## Motores (cada cliente, mismo comportamiento)

- [ ] T11. `renderDailyTemplate` (design §2) y su test contra los casos de
  T5. La FCV tiene que dar lo mismo que `buildDailyCopyText` (P4).
- [ ] T12. `fillOvertimeTemplate` y `validateOvertimeTemplate` (design §3)
  con los tests de T5:
  - fórmulas, celdas combinadas y formato condicional corridos;
  - 0, 1 y 31 filas;
  - un valor que empieza con `=`;
  - un zip bomba, y archivos `.xlsm` y con tablas.
- [ ] T13. Test de equivalencia: la FCV con el motor nuevo contra
  `generateOvertimeXlsx` (P4). Cuando pase, borrar `generateOvertimeXlsx`.

## Logday Desktop (`task-manager`)

- [ ] T14. Store: la plantilla activa por tipo, el archivo en
  `<basePath>/.logday/plantillas/` y el sync de `template`, con la cola
  "pendiente de subir" para el 404 (P24).
- [ ] T15. Migración con `templatesMigrated` (P6, P7, P10). Tests: una
  config vieja (FCV), la bienvenida (Logday), una fila que llegó por sync
  (no se pisa) y un segundo arranque (no se repite).
- [ ] T16. Copiar el Daily, "Enviar al chat" (`team-chat`) y el webhook
  saliente con `renderDailyTemplate` (P13).
- [ ] T17. El Excel de horas extra con `fillOvertimeTemplate`.
- [ ] T18. Opciones → Plantillas (design §4, P14, P18–P22).

## Logday Web

- [ ] T19. API y store de `template`, y el texto del Daily y el Excel con
  los motores.
- [ ] T20. Opciones → Plantillas.

## Logday Mobile

- [ ] T21. Tabla `templates` en SQLite, sync y migración con
  `templatesMigrated`.
- [ ] T22. Copiar y exportar con los motores. El `.xlsx` se elige con el
  selector de documentos.
- [ ] T23. Pantalla de Plantillas en Ajustes.

## Cierre

- [ ] T24. Prueba de punta a punta:
  - un usuario existente actualiza el servidor y Desktop y sigue con la FCV;
  - uno nuevo arranca con la de Logday;
  - un usuario carga una plantilla en Desktop y la ve en Web y Mobile;
  - "Usar la de Logday" en un equipo llega a los otros.
- [ ] T25. Vault: nota de la feature y decisión "Plantillas por empresa,
  sincronizadas", con los hubs y `Pendientes de Logday.md`.
