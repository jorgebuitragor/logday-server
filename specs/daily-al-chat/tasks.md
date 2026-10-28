# Daily al chat del equipo — Tareas

Estado: en diseño.

## Decisiones

- [x] Integración incluida y apagada por defecto, no plugin de terceros.
- [x] El servidor hace el envío. El manual usa el texto del cliente; el
  programado lo arma el servidor.
- [x] Proveedores: Google Chat, Slack y Teams (Workflows), por webhook
  entrante.
- [x] URL cifrada con `INTEGRATIONS_KEY` y una lista fija de hosts por
  proveedor.
- [x] Un destino por usuario en v1.
- [ ] Export de datos: qué se incluye (ver PENDIENTE en el design).
- [ ] Verificar con una URL real los hosts de Teams Workflows.

## Fase 1 — Servidor: configuración y envío manual

- [ ] `internal/security/secretbox.go`: `Seal`/`Open` con AES-256-GCM y
  dato adicional (R6, R9), con pruebas: ida y vuelta, otro `user_id`
  falla, blob corrupto falla.
- [ ] `cmd/server/main.go`: leer y validar `INTEGRATIONS_KEY` sin
  romper el arranque (R1), y documentarla en `.env.example`.
- [ ] Migración `00021_add_chat_integrations.sql` (tabla y columna de
  `instance_settings`).
- [ ] `settings`: `ChatIntegrationsEnabled`, más el checkbox del panel
  y su estado deshabilitado sin clave (R2).
- [ ] `internal/integration/providers.go`: validación de URL por
  proveedor (R5) y cuerpo por proveedor. Pruebas de tabla con URLs
  válidas e inválidas: `http`, otro host, host parecido
  (`chat.googleapis.com.evil.io`) y ruta equivocada.
- [ ] `store.go`: CRUD de `chat_integrations` (R4, R8) y registro del
  último resultado (R16).
- [ ] `handlers.go`: las 5 rutas, con 503, 409, 400, 429 y 502 (R3,
  R7, R10–R15).
- [ ] Limitador de 20 envíos por hora por usuario (R12).
- [ ] Logs sin URL ni texto. Probar que el log de un envío fallido no
  incluye la URL (R6).
- [ ] Pruebas de handlers contra un `httptest.Server` como proveedor:
  - 2xx;
  - 500;
  - timeout;
  - redirección no seguida (R14);
  - que la respuesta nunca incluye la URL.
- [ ] `openapi.yaml` y `specs/README.md` (índice).

## Fase 2 — Clientes: Integraciones y "Enviar al chat"

Cada repo con su spec y su PR, en paralelo contra el contrato de la
fase 1.

- [ ] Desktop: Opciones → Integraciones y el botón en `DailyEditor`
  (R23–R25).
- [ ] Web: lo mismo.
- [ ] Mobile: lo mismo, con el texto de `dailyCopyText.ts`.

## Fase 3 — Envío programado

- [ ] Port de `colombianHolidays.ts` a Go, con casos dorados de 2025 a
  2030 generados desde Desktop (R18).
- [ ] `buildDailyText`, con casos dorados de texto (R21).
- [ ] `scheduler.go`: ticker de 1 minuto con las reglas de R17–R22.
  Pruebas con un reloj inyectado:
  - antes de la hora;
  - a la hora;
  - 31 min tarde;
  - ya enviado;
  - festivo;
  - ausencia;
  - daily vacío.
- [ ] Clientes: el control de la hora programada en Integraciones.

## Fuera de alcance (specs aparte)

- Google Calendar, para enviar antes de la reunión "Daily".
- OpenProject en Desktop: traer los work packages asignados como tareas.
