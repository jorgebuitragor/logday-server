# Daily al chat del equipo — Tareas

Estado: en diseño.

## Decisiones

- [x] Es la extensión `team-chat` (grupo Integraciones), incluida y apagada por defecto, no plugin de terceros.
- [x] No depende del sync: modo local en Desktop y Mobile, y modo
  cuenta en el servidor (el único de Web).
- [x] El envío manual usa el texto del cliente; el programado lo arma
  quien envía (Desktop o el servidor).
- [x] Proveedores: Google Chat, Slack y Teams (Workflows), por webhook
  entrante.
- [x] URL en almacén seguro: llavero del SO, `expo-secure-store` o
  cifrada con `INTEGRATIONS_KEY`. Lista fija de hosts por proveedor.
- [x] Un destino por modo en v1.
- [ ] Export de datos: qué se incluye (ver PENDIENTE en el design).
- [ ] Verificar con una URL real los hosts de Teams Workflows.

## Fase 1 — Desktop en modo local (no necesita servidor)

Va en `task-manager`, sobre la base de `specs/extensiones/`, que se
hace antes o en el mismo PR.

- [ ] Rust: comando `team_chat_send`, con validación de host, `reqwest`
  sin redirecciones y timeout de 10 s (R10, R11, R18, R19). La URL se
  guarda con los comandos de secretos de la base.
- [ ] Pruebas en Rust de la validación con la tabla de casos del design.
- [ ] Preferencias locales: proveedor, pista, consentimiento y último
  resultado (R12, R21).
- [ ] La tarjeta en Opciones → Extensiones y el botón "Enviar al chat" en
  `DailyEditor` (R15–R17, R20, R30–R32).

## Fase 2 — Servidor (modo cuenta) y Web

- [ ] `internal/security/secretbox.go`: `Seal`/`Open` con AES-256-GCM y
  dato adicional (R11, R14), con pruebas: ida y vuelta, otro `user_id`
  falla, blob corrupto falla.
- [ ] `cmd/server/main.go`: leer y validar `INTEGRATIONS_KEY` sin
  romper el arranque (R6), y documentarla en `.env.example`.
- [ ] Migración `00021_add_chat_integrations.sql` (tabla y columna de
  `instance_settings`).
- [ ] `settings`: `ChatIntegrationsEnabled`, más el checkbox del panel
  y su estado deshabilitado sin clave (R7).
- [ ] `internal/integration/providers.go`: validación (R10) y cuerpo por
  proveedor. Pruebas de tabla con URLs válidas e inválidas: `http`, otro
  host, host parecido (`chat.googleapis.com.evil.io`) y ruta
  equivocada. Son los mismos casos que en Desktop.
- [ ] `store.go`: CRUD de `chat_integrations` (R9, R13) y registro del
  último resultado (R21).
- [ ] `handlers.go`: las 5 rutas, con 503, 409, 400, 429 y 502 (R8,
  R12, R15–R20).
- [ ] Limitador de 20 envíos por hora por usuario (R17).
- [ ] Logs sin URL ni texto. Probar que el log de un envío fallido no
  incluye la URL (R11).
- [ ] Pruebas de handlers contra un `httptest.Server` como proveedor:
  - 2xx;
  - 500;
  - timeout;
  - redirección no seguida (R19);
  - que la respuesta nunca incluye la URL.
- [ ] `openapi.yaml`.
- [ ] Web: la extensión en Opciones → Extensiones y el botón, solo en modo cuenta (R5,
  R30, R31).
- [ ] Desktop: elegir el modo, enviar por la cuenta y pasar de local a
  cuenta (R2–R4).

## Fase 3 — Mobile

- [ ] Modo local con `expo-secure-store` y `fetch`, solo el envío
  manual (R1, R23).
- [ ] Modo cuenta y elección del modo (R2–R4).

## Fase 4 — Envío programado

- [ ] Desktop en modo local: intervalo de 1 minuto con las reglas
  comunes, la ventana de 30 min al abrir la app, y deshabilitado si hay
  integración en la cuenta (R22–R29).
- [ ] Servidor: port de `colombianHolidays.ts` y de `buildDailyCopyText`
  a Go, con casos dorados de 2025 a 2030 generados desde Desktop (R24,
  R27).
- [ ] Servidor: `scheduler.go`, con un ticker de 1 minuto. Pruebas con
  un reloj inyectado:
  - antes de la hora;
  - a la hora;
  - 31 min tarde;
  - ya enviado;
  - festivo;
  - ausencia;
  - daily vacío.
- [ ] Clientes: el control de la hora programada en la tarjeta de la extensión.

## Fuera de alcance (specs aparte)

- Google Calendar, para enviar antes de la reunión "Daily".
- OpenProject en Desktop: traer los work packages asignados como tareas.
