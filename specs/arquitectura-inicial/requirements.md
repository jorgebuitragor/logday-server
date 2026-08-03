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

### Resolución de conflictos — PENDIENTE DE DECISIÓN

- El sistema DEBERÁ definir una estrategia de resolución de conflictos
  antes de implementar el protocolo de sync. Opciones a evaluar (ninguna
  descartada ni elegida todavía):
  - **Last-write-wins** (por registro o por campo): simple de
    implementar, pero con riesgo real de pérdida silenciosa de cambios
    cuando dos dispositivos editan lo mismo mientras ambos están
    offline.
  - **Alternativas más precisas y de menor riesgo**: p. ej. versionado
    con detección de conflicto explícita (el cliente sabe que su cambio
    pisa uno más reciente y puede decidir), merge por campo en vez de
    por registro completo, o algún esquema tipo vector clocks / CRDT
    acotado a los campos que realmente lo justifiquen.
- Esta decisión condiciona el diseño del protocolo de sync (siguiente
  spec) y no debe asumirse implícitamente al implementar.

## Fuera de este spec (para specs futuros)

- Protocolo de sync detallado (formato de cursor, forma de los
  endpoints, payloads de WebSocket).
- Estrategia de resolución de conflictos (ver arriba — pendiente).
- Esquema de datos completo tabla por tabla.
- Esquema de auth (usuarios, dispositivos, tokens).
- Migración de usuarios existentes del sync por git hacia esta API.
- Orden de integración de clientes (cuál consume la API primero).
