# Arquitectura inicial — Logday Server

Estado: en diseño

## Contexto

Logday Desktop (repo `task-manager`) es hoy 100% local: cada entidad
(`Task`, `Note`, entradas diarias, `OvertimeEntry`, `CalendarEvent`,
`AbsenceDay`) vive como archivo en `{basePath}/...`, gestionado desde
Tauri. El único sync existente es un wrapper client-side sobre el CLI de
`git` (sin servidor, sin resolución de conflictos propia).

Este proyecto es una API self-hosted independiente, desplegable con
Docker al estilo Vaultwarden, que sirve como backend de sync opcional
para múltiples clientes de Logday (desktop, web, móvil, extensión).
**Reemplaza** el sync por git — no coexiste con él a largo plazo.

## Requisitos (EARS)

### Filosofía local-first (no negociable)

- El sistema DEBERÁ permitir que cualquier cliente de Logday funcione de
  forma 100% local y completamente funcional sin configurar jamás un
  servidor de sync.
- Ninguna funcionalidad del cliente DEBERÁ quedar bloqueada o degradada
  por la ausencia de un servidor de sync configurado.
- Cuando un cliente realice una escritura, el sistema DEBERÁ persistir el
  cambio localmente de inmediato, sin esperar confirmación del servidor.
- Los identificadores de entidades DEBERÁN generarse del lado del
  cliente, nunca asignados por el servidor, para permitir crear/editar
  sin conectividad.
- El servidor DEBERÁ tratarse, en el diseño del protocolo de sync, como
  un nodo de sincronización más — no como la autoridad única de los
  datos del usuario.

### Despliegue self-hosted

- El sistema DEBERÁ poder desplegarse mediante un único contenedor
  Docker, sin dependencias externas obligatorias (base de datos embebida
  por defecto).
- El sistema DEBERÁ funcionar en hardware modesto (Raspberry Pi, NAS, VPS
  de bajos recursos).

### Multi-usuario

- El sistema DEBERÁ soportar múltiples usuarios por instancia desde la
  v1.
- El sistema DEBERÁ aislar los datos de forma que ningún usuario pueda
  leer o escribir datos de otro usuario.
- El sistema DEBERÁ tratar cada cliente/dispositivo conectado como una
  sesión independiente (no una única sesión global por usuario).

### Sync en tiempo real

- Cuando un cliente conectado modifique una entidad y el usuario tenga
  otros dispositivos conectados, el sistema DEBERÁ notificarles el
  cambio sin esperar a un intervalo de poll periódico.
- Un cliente que esté desconectado DEBERÁ poder reconciliar su estado al
  reconectar, usando el mismo mecanismo de sync incremental que usaría
  un cliente en tiempo real (no un sistema paralelo).

### Resolución de conflictos

- El sistema DEBERÁ resolver conflictos de campos simples (status,
  fechas, números, booleanos, y cualquier campo no listado como texto
  largo) mediante **last-write-wins por campo**, no por registro
  completo — dos ediciones concurrentes a campos distintos del mismo
  registro DEBERÁN sobrevivir ambas.
- El sistema DEBERÁ resolver conflictos en campos de texto largo
  (`Note.content`, `daily_entries.content`) mediante **CRDT**, para que
  ediciones concurrentes de texto en distintos dispositivos se
  mezclen en vez de que una pise a la otra. Ver `design.md` para la
  librería y el rol del servidor en este mecanismo.
- Ningún otro campo DEBERÁ usar CRDT salvo que se agregue
  explícitamente a la lista de campos de texto largo — el alcance se
  mantiene acotado a propósito (ver `design.md`, "Mapeo de datos").

## Fuera de este spec (para specs futuros)

- Protocolo de sync incremental detallado — ver
  [`sync-incremental/`](../sync-incremental/requirements.md).
- Payloads de WebSocket para tiempo real.
- Esquema de datos completo tabla por tabla.
- Esquema de auth (usuarios, dispositivos, tokens).
- Detalle de UI/flujo de "conectar servidor" en el cliente desktop.
