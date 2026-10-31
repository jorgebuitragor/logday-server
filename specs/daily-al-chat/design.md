# Daily al chat del equipo — Diseño

Estado: en diseño.

## Vista general

| | Modo local | Modo cuenta |
|---|---|---|
| Clientes | Desktop, Mobile | Desktop, Mobile, Web |
| Requiere sync | No | Sí |
| Dónde vive la URL | Desktop: llavero del SO. Mobile: `expo-secure-store` | Servidor, cifrada |
| Quién envía | El equipo (Desktop: Rust con `reqwest`. Mobile: `fetch`) | logday-server |
| Envío programado | Desktop con la app abierta; Mobile no | Sí, con las apps cerradas |
| Se ve en otros equipos | No | Sí |

Los dos modos comparten:

- la lista de proveedores y la validación de la URL;
- el cuerpo de cada proveedor;
- el texto del daily;
- los límites: 4000 caracteres, 20 envíos por hora, 10 s de timeout, sin
  redirecciones;
- las reglas del envío programado.

La lista de proveedores y los casos de prueba están en este spec, y
cada implementación los copia. Los casos dorados del final evitan que
se separen.

## Proveedores (contrato común)

| Proveedor | Host permitido | Ruta | Cuerpo |
|---|---|---|---|
| `google_chat` | `chat.googleapis.com` | `/v1/spaces/…/messages` | `{"text": …}` |
| `slack` | `hooks.slack.com` | `/services/…` o `/triggers/…` | `{"text": …}` |
| `teams` | `*.logic.azure.com`, `*.environment.api.powerplatform.com` | cualquiera | tarjeta adaptativa con un `TextBlock` (`wrap: true`) |

- **Teams:** los conectores de Office 365 (`*.webhook.office.com`)
  están retirados, así que solo se aceptan webhooks de Workflows. **Hay
  que verificar los hosts de Workflows al implementar**, con una URL
  real.
- **Hosts fijos:** el host se compara exacto, o como sufijo con punto
  en los `*.`. En el servidor, esta lista evita que se use para alcanzar
  direcciones internas (SSRF). En los equipos, evita mandar el daily a
  un destino que no es un chat.
- **Respuesta:** un 2xx es éxito; cualquier otra cosa es error, y se
  guarda solo el código.
- **Formato del texto:** se envía tal cual. Google Chat y Slack
  interpretan `*negrita*` y `_cursiva_` a su manera, y un `**negrita**`
  de Markdown se verá con asteriscos. Es una limitación aceptada en v1.
- **Texto de prueba:** "Mensaje de prueba de Logday: la integración con
  el chat funciona." (en inglés, según el idioma de la app).

## Modo cuenta: servidor

### Paquete `internal/integration`

Paquete vertical nuevo, con la misma forma que `absence` o `dailyentry`:
`models.go`, `store.go`, `handlers.go` y `helpers.go`. Se agregan dos
archivos propios:

- **`providers.go`:** validación de URL y armado del cuerpo.
- **`scheduler.go`:** el envío programado.

El cifrado va en `internal/security/secretbox.go`. Es una primitiva sin
conocimiento del dominio, igual que `password.go` y `token.go`.

### Esquema (migración `00021_add_chat_integrations.sql`)

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

### Cifrado

- `INTEGRATIONS_KEY`: 32 bytes en base64, generada con
  `openssl rand -base64 32`. Es una clave aparte de `JWT_SECRET`, para
  que rotar una no invalide la otra.
- **Formato:** `nonce || seal`, con un nonce aleatorio de 12 bytes por
  cada escritura. Se usa `user_id` como dato adicional autenticado, así
  que un blob copiado a otro usuario no se descifra.
- Si falla el descifrado, se guarda `last_status = needs_reconfig` y no
  se envía (R14). No hay rotación de clave en v1: cambiar la clave obliga
  a volver a pegar la URL.

### API

Todas las rutas usan `RequireAuth` y JSON.

| Método y ruta | Uso | Respuestas |
|---|---|---|
| `GET /integrations` | Estado | 200 `{available, chat: null \| ChatIntegration}` |
| `PUT /integrations/chat` | Crear o actualizar | 200 `ChatIntegration`; 400 |
| `DELETE /integrations/chat` | Borrar | 204 |
| `POST /integrations/chat/test` | Enviar el texto de prueba | 204; 409; 429; 502 |
| `POST /integrations/chat/send` | Enviar `{date, text}` | 204; 400; 409; 429; 502 |

- **503:** todas menos `GET` responden `503 integrations_unavailable`
  si el modo cuenta no está disponible (R8).
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
- **`consent`:** obligatorio al crear (R12). En una actualización se
  ignora. Al pasar de local a cuenta (R4), el cliente lo manda en
  `true`, porque el usuario ya aceptó el aviso en el equipo.
- **`schedule`:** se valida `HH:MM` y que la zona exista
  (`time.LoadLocation`).
- **Envío:** `send` registra `last_sent_date = date` cuando sale bien,
  y así el envío programado de ese día se omite (R26).

`openapi.yaml` se actualiza con estos esquemas en el mismo PR que el
código.

### Cliente HTTP y límite

- **Cliente HTTP:** timeout de 10 s, y `CheckRedirect` devuelve
  `http.ErrUseLastResponse` (R19).
- **Límite:** un limitador en memoria por `user_id` de 20 envíos por
  hora, con la misma ventana deslizante que `loginLimiter`
  (`internal/auth/ratelimit.go`). Al reiniciar el servidor se pierde la
  cuenta, y se acepta.
- **Logs:** nunca incluyen la URL ni el texto. Solo el usuario, el
  proveedor y el resultado.

### Panel y Configuración

- `settings.Settings` gana `ChatIntegrationsEnabled bool`.
- En el formulario de Configuración del panel aparece un checkbox.
- Si falta `INTEGRATIONS_KEY`, el panel muestra el checkbox deshabilitado
  y explica por qué.

### Envío programado del servidor

- **Ticker:** una goroutine con un `time.Ticker` de 1 minuto, arrancada
  en `main.go` como la purga de tombstones.
- **Cada minuto,** para cada integración con `enabled` y
  `schedule_enabled`, aplica las reglas comunes de abajo con los datos
  del servidor:
  - los dailys de `daily_entries`;
  - las ausencias de `absence_days` (sin borrar).
- **Port a Go:**
  - `buildDailyText(prevDate, prev, today, todayText)`, de
    `buildDailyCopyText` de Desktop (`src/lib/colombianHolidays.ts`);
  - los festivos de Colombia (fijos, Ley Emiliani y los que dependen de
    Pascua).
  - Las pruebas usan casos dorados generados con las funciones de
    Desktop: los festivos de 2025 a 2030 y un puñado de textos.

## Modo local: Desktop y Mobile

Cada cliente lo detalla en su spec. Lo común:

- **Configuración no secreta** (proveedor, pista, consentimiento,
  programación, último resultado) en las preferencias locales de la
  extensión (`AppConfig.extensions['team-chat']` en Desktop), que no se
  sincronizan.
- **La URL, en el almacén seguro:**
  - Desktop, con los comandos de secretos de la base de extensiones
    (`ext_secret_set('team-chat', 'webhook', …)`), que usan el llavero
    del SO.
  - Mobile, con `expo-secure-store`.
- **Desktop envía desde Rust.** Un comando `team_chat_send(provider,
  text)`, en `src-tauri/src/extensions/team_chat.rs`, lee la URL del
  llavero, valida el host y envía con `reqwest` (timeout de 10 s,
  `redirect::Policy::none()`). La URL nunca cruza al webview.
- **Mobile envía con `fetch`** (React Native no aplica CORS), con
  `AbortController` para el timeout y `redirect: 'manual'`.
- **Programado en Desktop:** un intervalo de 1 minuto mientras la app
  está abierta, con las reglas comunes. Usa `getPreviousWorkingDay`,
  `isWorkingDay` y `buildDailyCopyText`, que ya existen. Si la app
  estaba cerrada a la hora programada, aplica la ventana de 30 minutos
  (R28) al abrirse.
- **Límite:** 20 envíos por hora en memoria, por equipo.

## Elegir el modo (R2–R4)

```
¿hay sesión de sync y GET /integrations → available?
├─ no → modo local (Web: no se muestra la integración)
└─ sí → al configurar: "En tu cuenta" (por defecto) o "Solo en este equipo"
         al enviar: cuenta si existe; si no, local
```

- **Pasar de local a cuenta** ("Guardar en tu cuenta"): el cliente lee
  la URL del almacén seguro, hace `PUT /integrations/chat` y, si
  responde 200, borra la URL y la configuración locales.
- **Cerrar la sesión de sync** no toca la integración local. La de la
  cuenta deja de usarse en ese equipo.

## Reglas comunes del envío programado

Cada minuto, para una integración con programación activa:

1. Calcula la fecha y la hora local en la zona configurada.
2. Sigue solo si pasó la hora programada, si no pasaron más de 30 min
   (R28) y si el último daily enviado con éxito no es el de hoy (R26).
   Si pasaron más de 30 min, guarda `skipped_late` una sola vez por día.
3. Sigue solo si hoy es hábil (R24): ni fin de semana, ni festivo de
   Colombia, ni día de ausencia del usuario.
4. Lee los dailys de hoy y del día hábil anterior. Si el de hoy está
   vacío, guarda `skipped_empty` (R25).
5. Arma el texto como Copiar y lo envía.

**Riesgo de doble envío:** si el mismo usuario tiene configurados los
dos modos con programación, el servidor y Desktop enviarían cada uno.
Por eso el modo local no programa mientras exista una integración de la
cuenta (R29).

## Clientes (UI)

- **La tarjeta "Chat del equipo" en Opciones → Extensiones** (R30):
  - proveedor (Google Chat, Slack, Teams);
  - dónde se guarda, si aplica (R2);
  - campo de URL de tipo contraseña, que se vacía después de guardar y
    muestra la pista;
  - el aviso de consentimiento la primera vez;
  - botones "Enviar prueba" y "Quitar integración";
  - la hora programada, con el aviso "solo mientras la app esté
    abierta" en modo local de Desktop;
  - el último resultado.
- **Daily:** "Enviar al chat" junto a Copiar (R31). Envía el texto de
  `buildDailyCopyText` y la fecha del daily.
  - Si el envío falla, muestra un aviso con el código del proveedor.
  - Si se alcanzó el límite, lo avisa.
  - Si ese daily ya se envió hoy, pide confirmar antes de mandarlo de
    nuevo.

## Alternativas descartadas

- **Solo modo cuenta:** obliga a tener sync, y Logday es local-first.
  Jorge lo pidió explícitamente: las integraciones no deben depender
  del sync.
- **Solo modo local:** Web no puede enviar, la URL quedaría en cada
  equipo, y no habría envío programado con las apps cerradas.
- **Que Desktop envíe desde el webview con `fetch`:** la URL pasaría por
  JavaScript, y el CORS del proveedor podría bloquearlo. Desde Rust no
  hay CORS y la URL no sale del llavero.
- **Guardar la URL local en las preferencias:** quedaría en texto plano
  en `config.json` y en los backups.
- **Plugins de código de terceros:** ver la nota del vault. Las
  integraciones incluidas cubren el caso sin ejecutar código ajeno.
- **Que el servidor arme también el texto del envío manual:** duplicaría
  la lógica de los clientes en el camino que más se usa.
- **Derivar la clave de `JWT_SECRET`:** rotar el secreto de sesión
  rompería todas las integraciones.
- **Filtrar IPs privadas en vez de una lista de hosts:** es más frágil
  (DNS rebinding, IPv6) y no hace falta, porque los proveedores son
  pocos y fijos.
