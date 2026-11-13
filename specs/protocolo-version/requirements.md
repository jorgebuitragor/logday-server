# Versión de protocolo — Requirements

Estado: en progreso (servidor implementado; clientes pendientes).

## Contexto

Ningún cliente sabía si el servidor al que se conecta es compatible.
Cuando llegue el cifrado de extremo a extremo (vault: "Cifrado de
extremo a extremo (plan)"), una app vieja escribiría texto plano en una
cuenta cifrada sin enterarse. Mobile 1.0 se publica antes del cifrado,
así que tiene que traer ya este aviso (vault: "Publicación de Logday
Mobile (plan)").

Decisión de Jorge (13 nov 2026): la compatibilidad se mide con un
**número de protocolo** entero y propio, no con la versión de las apps.
Desktop, Web y Mobile tienen números de versión distintos, y subir la
versión de una app por un arreglo visual no dice nada sobre el sync.

## Requisitos (EARS)

### Servidor

- El sistema DEBERÁ exponer `GET /version`, sin autenticación y con
  `Cache-Control: no-store`, que responde `server_version` (informativa),
  `protocol` (el protocolo que habla) y `min_client_protocol` (el más
  viejo que acepta).
- `min_client_protocol` NO DEBERÁ ser mayor que `protocol`.
- Cuando un pedido a la API trae `X-Logday-Protocol` con un entero menor
  que `min_client_protocol`, el sistema DEBERÁ responder `426` con
  `X-Logday-Min-Protocol: <mínimo>` y no atender el pedido.
- Cuando un pedido a la API no trae `X-Logday-Protocol`, el sistema
  DEBERÁ tratarlo como protocolo `1`.
- Cuando `X-Logday-Protocol` no es un entero positivo, el sistema DEBERÁ
  responder `400`.
- La regla NO DEBERÁ aplicarse a `/health`, `/version`, `/ws`,
  `POST /auth/logout`, el panel de administración ni la web embebida en
  `/app`. Cerrar sesión siempre funciona para que el refresh token de un
  cliente viejo no quede vivo.
- Cuando se haga un preflight CORS, el sistema DEBERÁ aprobar la
  cabecera `X-Logday-Protocol`.

### Clientes

- Cada cliente DEBERÁ mandar `X-Logday-Protocol` en todos sus pedidos a
  la API.
- Cuando la API responde `426`, el cliente DEBERÁ dejar de sincronizar,
  conservar sus datos locales y pedir al usuario que actualice la app.
- Cuando `GET /version` dice un `protocol` menor que el mínimo de
  servidor que necesita el cliente, el cliente DEBERÁ avisar que el
  servidor está desactualizado y no sincronizar.
- Cuando `GET /version` responde `404`, el cliente DEBERÁ tratar al
  servidor como protocolo `1`.
- Cuando `GET /version` falla por red, el cliente NO DEBERÁ bloquear el
  sync por eso: la comprobación se repite en la siguiente conexión.

## Fuera de alcance

- Subir el protocolo a 2. Lo hará el cifrado de extremo a extremo.
- Un mínimo por cuenta (solo las cuentas cifradas exigiendo protocolo 2).
  Se decide con el diseño del cifrado.
