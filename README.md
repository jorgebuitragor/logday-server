# Logday Server

API de sync self-hosted para [Logday](https://github.com/jorgebuitragor/logday),
al estilo Vaultwarden/Bitwarden: se despliega con un único contenedor
Docker, y cada cliente (desktop, web, móvil, extensión) se configura para
apuntar a su propio host.

El sync es **opcional**: cualquier cliente de Logday sigue siendo
100% funcional sin conexión a un servidor. Este proyecto existe para
quienes eligen sincronizar sus datos entre dispositivos o con su equipo,
manteniendo el control total sobre dónde viven esos datos.

## Estado del proyecto

Implementado. Ver [`specs/`](./specs/README.md) para el proceso de
spec-driven development y el detalle de diseño de cada feature.

## Stack

- Go (`net/http` + `chi`), un único binario estático
- SQLite por defecto (archivo único, cero configuración), Postgres
  intercambiable en instalaciones más grandes
- WebSocket para sync en tiempo real
- Panel de administración web embebido en el mismo binario (sin
  frontend/SPA separado)
- Docker multi-stage build (~25MB, corre como usuario no-root)

## Levantar el servidor

Requiere Docker y Docker Compose.

```bash
cp .env.example .env
# editá .env — ver "Variables de entorno" abajo
docker compose --env-file .env up -d --build
```

Confirmá que arrancó: `curl http://localhost:8080/health` debería
responder `ok`.

### Variables de entorno

| Variable | Requerida | Descripción |
|---|---|---|
| `JWT_SECRET` | Sí | Firma los access tokens (JWT). Generá una con `openssl rand -base64 32`. |
| `ADMIN_EMAIL` | No | Email del primer admin, creado en el primer arranque. Ver "Primer arranque" abajo — alternativa a `/setup`. |
| `ADMIN_PASSWORD` | No | Password del primer admin. Va junto con `ADMIN_EMAIL` — si falta una de las dos, no se usa ninguna. |
| `DATABASE_PATH` | No | Ruta del archivo SQLite. Ya viene fijada a `/data/logday.db` en `docker-compose.yml` (el volumen `logday-data` persiste ese directorio). |
| `PORT` | No | Puerto HTTP interno del binario. Por defecto `8080`. |

### Primer arranque: dos formas de crear el primer admin

**Opción A — variables de entorno** (sin interacción, ideal para deploys
automatizados): seteá `ADMIN_EMAIL`/`ADMIN_PASSWORD` en `.env` antes del
primer `docker compose up`. El servidor crea ese usuario como admin la
primera vez que arranca con la base de datos vacía. En arranques
posteriores estas variables se ignoran (ya existe al menos un usuario).

**Opción B — `/setup` desde el navegador**: si no seteás esas variables,
el servidor arranca igual (no falla) y sirve un formulario en
`http://<tu-host>:8080/setup` para crear el primer admin ahí. Una vez
creado, esa pantalla queda bloqueada — no se puede volver a usar para
crear un segundo "primer admin".

Cualquiera de las dos formas te deja con una cuenta admin que puede
loguearse en `http://<tu-host>:8080/admin/panel/login`.

## API para clientes

El contrato completo de la API de sync (auth, CRUD por recurso, feed de
`/sync/changes` y el protocolo de `/ws`) está documentado en
[`openapi.yaml`](./openapi.yaml) (OpenAPI 3.0.3) — abrilo en
[Swagger Editor](https://editor.swagger.io) o con cualquier visor de OpenAPI
para navegarlo.

## Panel de administración

Una vez que existe al menos un admin, `http://<tu-host>:8080/admin/panel`
permite, sin tocar la API a mano:

- Ver todos los usuarios de la instancia (activos y dados de baja).
- Crear usuarios nuevos, promoverlos/degradarlos a admin.
- Dar de baja (reversible) o restaurar un usuario.
- Resetear la contraseña de otro usuario (admin-asistido — no requiere
  SMTP, no hay recuperación self-service todavía).
- Ver y revocar los dispositivos/sesiones de **cualquier** usuario de la
  instancia (no solo los propios).

Ver [`specs/panel-admin/`](./specs/panel-admin/requirements.md) para el
detalle de diseño (por qué la sesión del panel es una cookie propia y no
reusa el modelo de tokens de los clientes de sync, CSRF, etc.).

## Reverse proxy y TLS

El servidor sirve HTTP plano a propósito — no maneja certificados ni
termina TLS él mismo, mismo criterio que Vaultwarden. Para exponerlo con
HTTPS, poné un reverse proxy delante (Caddy, nginx, Traefik, lo que ya
uses). Ejemplo mínimo con Caddy (maneja Let's Encrypt automáticamente):

```caddyfile
logday.tudominio.com {
    reverse_proxy localhost:8080
}
```

O con nginx, asumiendo que la terminación TLS ya está resuelta en el
`server` block de arriba:

```nginx
location / {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
```

Si tu proxy termina TLS y le habla al contenedor por HTTP interno (el
caso normal), el panel de administración detecta la conexión como no-TLS
y no marca su cookie de sesión como `Secure` — para que eso no debilite
la seguridad real, asegurate de que el tráfico entre el proxy y el
contenedor quede dentro de una red que el proxy controle (la red interna
de Docker, o `localhost`), no expuesto directamente a internet sin pasar
por el proxy.

## Desarrollo

```bash
make build   # go build -o bin/server ./cmd/server
make run     # go run ./cmd/server
make test    # go test ./...
make lint    # golangci-lint run ./...
make fmt     # golangci-lint fmt ./...
```

Ver [`specs/convenciones-codigo/`](./specs/convenciones-codigo/requirements.md)
para la estructura de paquetes y convenciones de código.
