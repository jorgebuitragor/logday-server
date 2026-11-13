# Versión de protocolo — Design

## Servidor

Paquete `internal/compat`:

- `Protocol` y `MinClientProtocol`, constantes. Hoy los dos valen `1`.
- `ServerVersion`, variable fijada al compilar con
  `-ldflags "-X .../internal/compat.ServerVersion=<versión>"`. El
  `Makefile` usa `git describe`; el `Dockerfile` recibe `--build-arg
  VERSION=...` y, sin él, queda `dev`. Es solo para mostrar: ningún
  cliente decide nada con ella.
- `VersionHandler` para `GET /version`.
- `RequireClientProtocol(min, exempt...)`, middleware.

En `cmd/server/main.go` las rutas de la API de los dominios (`auth`,
`task`, `note`, `overtime`, `calendar`, `absence`, `dailyentry`, `sync`)
se registran dentro de un `r.Group` con el middleware. Quedan fuera
`/health`, `/version`, el panel (`PanelRoutes`), la web embebida y `/ws`.

`/ws` queda fuera porque el navegador no deja poner cabeceras en el
handshake de un WebSocket, y porque solo manda avisos: el cliente viejo
recibe el `426` en el pull que sigue al aviso.

### Cómo se sube el protocolo

1. Un cambio que un cliente viejo no entendería sube `Protocol`.
2. Los clientes nuevos aprenden ese protocolo y lo mandan.
3. Cuando los clientes viejos ya no deben escribir, se sube
   `MinClientProtocol`. A partir de ahí reciben `426`.

Si un cambio no rompe a los clientes viejos, `MinClientProtocol` puede
quedarse atrás de `Protocol` todo el tiempo que haga falta.

### Logday Web

Web va embebida en el mismo binario, así que siempre habla el mismo
protocolo que su servidor. Aun así tiene que mandar la cabecera antes de
que `MinClientProtocol` pase de `1`; si no, el servidor la trataría como
protocolo 1 y la rechazaría.

## Clientes

Cada cliente define dos constantes:

- su protocolo (`1` hoy), que manda en `X-Logday-Protocol`;
- el protocolo mínimo de servidor que necesita (`1` hoy).

Al conectar y al comprobar la conexión pide `GET /version`:

| Resultado | Qué hace |
|---|---|
| `protocol` < mínimo de servidor del cliente | Aviso "el servidor está desactualizado", no sincroniza. |
| `min_client_protocol` > protocolo del cliente | Aviso "actualiza la app", no sincroniza. |
| `404` | Servidor de protocolo 1. |
| Error de red | No bloquea; se repite la próxima vez. |

Un `426` en cualquier pedido produce el mismo aviso de "actualiza la
app". En los dos avisos el cliente conserva sus datos y su sesión: al
actualizar la app o el servidor, el sync sigue donde estaba.

## Alternativas descartadas

- **Versión semántica de cada app** (el servidor exige "Mobile ≥ 1.2.0").
  El servidor tendría que llevar un mínimo por cliente, y las versiones
  de las apps cambian por motivos que no tienen que ver con el sync.
- **Ampliar `/health`** con la versión. `/health` responde `text/plain`
  "ok" y lo usan los chequeos de Docker y Coolify; cambiarlo a JSON los
  rompería.
- **Mandar la versión en la respuesta del login.** Solo llega al
  conectar, y el cliente necesita saberlo también antes de pedir la
  contraseña y al comprobar la conexión.
