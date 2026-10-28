# Daily al chat del equipo — Diseño

Estado: en diseño.

## Paquete `internal/integration`

Paquete vertical nuevo, con la misma forma que `absence` o `dailyentry`:
`models.go`, `store.go`, `handlers.go` y `helpers.go`. Se agregan dos
archivos propios:

- **`providers.go`:** validación de URL y armado del cuerpo por
  proveedor.
- **`scheduler.go`:** el envío programado, que es la fase 3.

El cifrado va en `internal/security/secretbox.go`. Es una primitiva sin
conocimiento del dominio, igual que `password.go` y `token.go`.

## Esquema (migración `00021_add_chat_integrations.sql`)

```sql
CREATE TABLE chat_integrations (
    user_id          TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,          -- google_chat | slack | teams
    webhook_enc      BLOB NOT NULL,          -- nonce(12) || AES-256-GCM(url)
    webhook_hint     TEXT NOT NULL,          -- "chat.googleapis.com …a1b2"
    enabled          INTEGER NOT NULL DEFAULT 1,
    consent_at       TEXT NOT NULL,          -- instante ISO
    schedule_enabled INTEGER NOT NULL DEFAULT 0,
    schedule_time    TEXT,                   -- "HH:MM"
    schedule_tz      TEXT,                   -- IANA, p. ej. America/Bogota
    last_sent_at     TEXT,                   -- instante del último intento
    last_sent_date   TEXT,                   -- YYYY-MM-DD del daily enviado con éxito
    last_status      TEXT,                   -- ok | error | skipped_empty | skipped_late | needs_reconfig
    last_error       TEXT,                   -- código del proveedor o motivo, sin la URL
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
ALTER TABLE instance_settings ADD COLUMN chat_integrations_enabled INTEGER NOT NULL DEFAULT 0;
```

No es una entidad sincronizada: no tiene `seq`, no aparece en
`/sync/changes` y no pasa por LWW. Es configuración de la cuenta, como
los dispositivos.

**PENDIENTE DE DECISIÓN:** el export de datos personales podría
incluir el proveedor, la programación y la fecha del consentimiento,
sin la URL. Por ahora no se exporta nada de la integración.

## Cifrado

- `INTEGRATIONS_KEY`: 32 bytes en base64, generada con
  `openssl rand -base64 32`. Es una clave aparte de `JWT_SECRET`, para
  que rotar una no invalide la otra.
- **Formato:** `nonce || seal`, con un nonce aleatorio de 12 bytes por
  cada escritura. Se usa `user_id` como dato adicional autenticado, así
  que un blob copiado a otro usuario no se descifra.
- Si falla el descifrado, se guarda `last_status = needs_reconfig` y no
  se envía (R9). No hay rotación de clave en v1: cambiar la clave obliga
  a volver a pegar la URL.

## Proveedores

| Proveedor | Host permitido | Ruta | Cuerpo |
|---|---|---|---|
| `google_chat` | `chat.googleapis.com` | `/v1/spaces/…/messages` | `{"text": …}` |
| `slack` | `hooks.slack.com` | `/services/…` o `/triggers/…` | `{"text": …}` |
| `teams` | `*.logic.azure.com`, `*.environment.api.powerplatform.com` | cualquiera | tarjeta adaptativa con un `TextBlock` (`wrap: true`) |

- **Teams:** los conectores de Office 365 (`*.webhook.office.com`)
  están retirados, así que solo se aceptan webhooks de Workflows. **Hay
  que verificar los hosts de Workflows al implementar**, con una URL
  real.
- **Solo hosts fijos:** la lista evita que el servidor sirva para hacer
  peticiones a direcciones internas (SSRF), sin tener que filtrar IPs.
  En las pruebas se reemplaza con un `httptest.Server`.
- **Cliente HTTP:**
  - timeout de 10 s;
  - `CheckRedirect` devuelve `http.ErrUseLastResponse` (R14);
  - una respuesta 2xx es éxito; cualquier otra cosa es error, y se
    guarda solo el código.
- **Formato del texto:** se envía tal cual. Google Chat y Slack
  interpretan `*negrita*` y `_cursiva_` a su manera, y un `**negrita**`
  de Markdown se verá con asteriscos. Es una limitación aceptada en v1.

## API

Todas las rutas usan `RequireAuth` y JSON.

| Método y ruta | Uso | Respuestas |
|---|---|---|
| `GET /integrations` | Estado | 200 `{available, chat: null \| ChatIntegration}` |
| `PUT /integrations/chat` | Crear o actualizar | 200 `ChatIntegration`; 400 |
| `DELETE /integrations/chat` | Borrar | 204 |
| `POST /integrations/chat/test` | Enviar el texto de prueba | 204; 409; 429; 502 |
| `POST /integrations/chat/send` | Enviar `{date, text}` | 204; 400; 409; 429; 502 |

- **503:** todas menos `GET` responden `503 integrations_unavailable`
  si la integración no está disponible (R3).
- **409:** el usuario no configuró la integración, o hay que volver a
  configurarla.

```jsonc
// ChatIntegration (respuesta)
{
  "provider": "google_chat",
  "webhook_hint": "chat.googleapis.com …a1b2",
  "enabled": true,
  "consent_at": "2026-10-28T13:00:00.000Z",
  "schedule": { "enabled": false, "time": "08:50", "tz": "America/Bogota" },
  "last": { "sent_at": "…", "date": "2026-10-28", "status": "ok", "error": null }
}

// PUT /integrations/chat (petición)
{
  "provider": "google_chat",
  "webhook_url": "https://chat.googleapis.com/v1/spaces/…",
  "enabled": true,
  "consent": true,
  "schedule": { "enabled": true, "time": "08:50", "tz": "America/Bogota" }
}
```

- **`webhook_url`:** obligatoria al crear. Si se omite al actualizar,
  se conserva la URL guardada. Si cambia el `provider`, hay que mandar
  la URL de nuevo.
- **`consent`:** obligatorio al crear (R7). En una actualización se
  ignora.
- **`schedule`:** se valida `HH:MM` y que la zona exista
  (`time.LoadLocation`). Hasta la fase 3, un `schedule.enabled: true` se
  guarda pero no hace nada, y los clientes no muestran ese control.
- **Envío:** `send` registra `last_sent_date = date` cuando sale bien,
  y así el envío programado de ese día se omite (R20).

`openapi.yaml` se actualiza con estos esquemas en el mismo PR que el
código.

## Límite de envíos

Un limitador en memoria por `user_id` de 20 envíos por hora, con la
misma ventana deslizante que `loginLimiter` (`internal/auth/ratelimit.go`).
Cuenta los envíos manuales, los de prueba y los programados. Al
reiniciar el servidor se pierde la cuenta, y se acepta: el límite es
para evitar abusos, no para contabilizar.

## Panel y Configuración

- `settings.Settings` gana `ChatIntegrationsEnabled bool`.
- En el formulario de Configuración del panel aparece un checkbox
  "Permitir enviar el daily a chats externos".
- Si falta `INTEGRATIONS_KEY`, el panel muestra el checkbox deshabilitado
  y explica por qué.

## Envío programado (fase 3)

- **Ticker:** una goroutine con un `time.Ticker` de 1 minuto, arrancada
  en `main.go` como la purga de tombstones.
- **Cada minuto,** para cada integración con `enabled` y
  `schedule_enabled`:
  1. calcula la fecha y la hora local en `schedule_tz`;
  2. sigue solo si pasó la hora programada, si no pasaron más de 30 min
     (R22) y si `last_sent_date` es distinta de hoy (R20);
  3. sigue solo si hoy es hábil (R18). La regla es la de `isWorkingDay`
     de Desktop: fin de semana, festivos de Colombia y los días de
     ausencia del usuario (tabla `absence_days`, sin borrar);
  4. lee los dailys de hoy y del día hábil anterior. Si el de hoy está
     vacío, guarda `skipped_empty` (R19);
  5. arma el texto con `buildDailyText` y lo envía.
- **`skipped_late`:** si pasaron más de 30 min, guarda `skipped_late`
  una sola vez por día.
- **`buildDailyText(prevDate, prev, today, todayText)`:** un port de
  `buildDailyCopyText` de Desktop (`src/lib/colombianHolidays.ts`).
  - Formato del día: `"<Día> <d> de <mes>"` (por ejemplo, "Lunes 12 de
    enero"), con los nombres tal como los escribe Desktop.
  - Si el anterior está vacío: `"- (sin actividades registradas)"`.
- **Festivos:** un port de `colombianHolidays.ts` (fijos, Ley Emiliani y
  los que dependen de Pascua). Las pruebas usan casos dorados generados
  con la función de Desktop: los festivos de 2025 a 2030 y un puñado de
  textos. Así un cambio en un lado sin el otro rompe la prueba.

## Clientes

Cada cliente agrega lo suyo en su propio spec. El contrato es este:

- **Opciones → Integraciones**, visible solo si `available` (R23):
  - proveedor (Google Chat, Slack, Teams);
  - campo de URL de tipo contraseña, que se vacía después de guardar y
    muestra la `webhook_hint`;
  - el aviso de consentimiento la primera vez;
  - botones "Enviar prueba" y "Quitar integración";
  - el último resultado.
  - La hora programada aparece en la fase 3.
- **Daily:** "Enviar al chat" junto a Copiar (R24). Envía el texto de
  `buildDailyCopyText` y la fecha del daily.
  - Si la respuesta es 502, muestra un aviso con el código del proveedor.
  - Si es 429, avisa que se alcanzó el límite.
  - Si ese daily ya se envió hoy, pide confirmar antes de mandarlo de
    nuevo.
- **Desktop sin sesión de sync:** la sección explica que la integración
  necesita una cuenta en un servidor.

## Alternativas descartadas

- **Enviar desde cada cliente:** Web no puede, por la CSP y la falta de
  CORS. Además habría que guardar la URL en cada dispositivo, y no habría
  envío con las apps cerradas.
- **Plugins de código de terceros:** ver la nota del vault. Las
  integraciones incluidas cubren el caso sin ejecutar código ajeno.
- **Que el servidor arme también el texto del envío manual:** duplicaría
  la lógica de los clientes en el camino que más se usa. El cliente ya
  tiene el texto que el usuario ve en pantalla.
- **Derivar la clave de `JWT_SECRET`:** rotar el secreto de sesión
  rompería todas las integraciones.
- **Filtrar IPs privadas en vez de una lista de hosts:** es más frágil
  (DNS rebinding, IPv6) y no hace falta, porque los proveedores son
  pocos y fijos.
