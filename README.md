# Logday Server

![local-first](./.github/badge-local-first.svg)

API de sync self-hosted para Logday, al estilo Vaultwarden/Bitwarden: se despliega con un único contenedor Docker, y cada cliente se configura para apuntar a su propio host.

El sync es **opcional**: cualquier cliente de Logday sigue siendo 100% funcional sin conexión a un servidor. Este proyecto existe para quienes eligen sincronizar sus datos entre dispositivos o con su equipo, manteniendo el control total sobre dónde viven esos datos.

## Clientes

- [Logday Desktop](https://github.com/jorgebuitragor/logday) (Tauri; macOS, Windows y Linux): local-first, el sync se activa en sus opciones.
- [Logday Web](https://github.com/jorgebuitragor/logday-web) (React): este servidor la sirve embebida en `/app`. Es el único cliente que necesita el servidor siempre.
- [Logday Mobile](https://github.com/jorgebuitragor/logday-mobile) (Expo; Android e iOS): local-first, con sync opcional.

## Estado del proyecto

Implementado y en uso. Las 7 entidades (tareas, notas, dailys, horas extra con su resumen mensual, eventos de calendario y ausencias) se sincronizan con LWW por campo. El texto de notas y dailys usa CRDT (Yjs, vía `ygo`). Hay multi-usuario con aislamiento por cuenta, sync en tiempo real por WebSocket, papelera compartida y política de privacidad con consentimiento. Ver [`specs/`](./specs/README.md) para el detalle de diseño de cada feature.

## Stack

- Go (`net/http` + `chi`), un único binario estático.
- SQLite por defecto (archivo único, cero configuración), con migraciones `goose` embebidas.
- WebSocket para sync en tiempo real.
- Panel de administración web embebido en el mismo binario (`html/template`, sin SPA).
- Docker multi-stage build (~25MB, corre como usuario no-root).

## Levantar el servidor

Requiere Docker y Docker Compose. El build también compila [`logday-web`](https://github.com/jorgebuitragor/logday-web) (repo privado, aparte) y lo embebe, sirviéndolo en `/app`. Como es privado, el build lo toma de un checkout local en vez de clonarlo, así que necesitas tenerlo clonado como hermano de este repo (`../logday-web` relativo a acá).

```bash
git clone https://github.com/jorgebuitragor/logday-web.git ../logday-web
cp .env.example .env
# edita .env (ver "Variables de entorno" abajo)
docker compose --env-file .env up -d --build
```

Confirma que arrancó: `curl http://localhost:8080/health` debería responder `ok`. El cliente web queda en `http://localhost:8080/app`.

Si construyes la imagen en un Mac con Apple Silicon para un servidor x86_64, agrega `--platform linux/amd64`: si no, la imagen sale para ARM.

### Variables de entorno

| Variable | Requerida | Descripción |
|---|---|---|
| `JWT_SECRET` | Sí | Firma los access tokens (JWT). Genera una con `openssl rand -base64 32`. |
| `ADMIN_EMAIL` | No | Email del primer admin, creado en el primer arranque. Ver "Primer arranque" abajo; es la alternativa a `/setup`. |
| `ADMIN_PASSWORD` | No | Contraseña del primer admin. Va junto con `ADMIN_EMAIL`: si falta una de las dos, no se usa ninguna. |
| `DATABASE_PATH` | No | Ruta del archivo SQLite. Por defecto `./data/logday.db`; `docker-compose.yml` la fija en `/data/logday.db`, y el volumen `logday-data` persiste ese directorio. |
| `PORT` | No | Puerto HTTP interno del binario. Por defecto `8080`. |
| `CORS_ALLOWED_ORIGINS` | No | Orígenes exactos, separados por comas, que pueden llamar a la API desde otro dominio (por ejemplo, un Logday Web desplegado aparte). Vacía, CORS queda apagado. No acepta comodines. |
| `COOKIE_SECURE` | No | `always` marca siempre como `Secure` la cookie de sesión de Logday Web. Úsala si tu cadena de proxies no reenvía bien `X-Forwarded-Proto` (por ejemplo, un CDN con TLS "flexible"). |

### Primer arranque: dos formas de crear el primer admin

**Opción A: variables de entorno** (sin interacción, ideal para despliegues automatizados). Define `ADMIN_EMAIL`/`ADMIN_PASSWORD` en `.env` antes del primer `docker compose up`. El servidor crea ese usuario como admin la primera vez que arranca con la base de datos vacía. En arranques posteriores estas variables se ignoran, porque ya existe al menos un usuario.

**Opción B: `/setup` desde el navegador**. Si no defines esas variables, el servidor arranca igual (no falla) y sirve un formulario en `http://<tu-host>:8080/setup` para crear ahí el primer admin. Una vez creado, esa pantalla queda bloqueada: no se puede volver a usar para crear un segundo "primer admin".

Cualquiera de las dos formas te deja con una cuenta admin que puede iniciar sesión en `http://<tu-host>:8080/admin/panel/login`.

## API para clientes

El contrato completo de la API de sync (auth, CRUD por recurso, feed de `/sync/changes` y el protocolo de `/ws`) está documentado en [`openapi.yaml`](./openapi.yaml) (OpenAPI 3.0.3). Ábrelo en [Swagger Editor](https://editor.swagger.io) o en cualquier visor de OpenAPI para navegarlo.

## Panel de administración

Una vez que existe al menos un admin, `http://<tu-host>:8080/admin/panel` permite, sin tocar la API a mano:

- Ver todos los usuarios de la instancia (activos y dados de baja).
- Crear usuarios nuevos y promoverlos o degradarlos a admin.
- Dar de baja (reversible) o restaurar un usuario.
- Resetear la contraseña de otro usuario. Lo hace un admin, así que no requiere SMTP; todavía no hay recuperación self-service.
- Ver y revocar los dispositivos y sesiones de **cualquier** usuario de la instancia, no solo los propios.

Ver [`specs/panel-admin/`](./specs/panel-admin/requirements.md) para el detalle de diseño: por qué la sesión del panel es una cookie propia y no reusa el modelo de tokens de los clientes de sync, el CSRF, etc.

**Recomendación para producción**: el panel (`/admin/panel/*`) es la única parte del servidor pensada para que una persona la use desde el navegador. La API de sync (`/auth`, `/tasks`, `/sync/changes`, `/ws`, etc.) la consumen los clientes de Logday y no puede quedar detrás de un login interactivo. Si tu proxy o CDN soporta reglas de acceso por path (por ejemplo Cloudflare Access, un `forward_auth` de Traefik, o `auth_request` de nginx con un proveedor SSO), agrega esa capa **solo sobre `/admin/panel/*`** como segundo factor delante del login propio del panel. Deja el resto del dominio sin esa capa, para no romper a los clientes de sync. Es la misma seguridad en profundidad que se recomienda para cualquier dashboard administrativo self-hosted (Coolify, Vaultwarden, etc.): la autenticación de la app más la del borde, en dos capas independientes.

## Reverse proxy y TLS

El servidor sirve HTTP plano a propósito: no maneja certificados ni termina TLS él mismo, igual que Vaultwarden. Para exponerlo con HTTPS, pon un reverse proxy delante (Caddy, nginx, Traefik, el que ya uses). Ejemplo mínimo con Caddy, que maneja Let's Encrypt automáticamente:

```caddyfile
logday.tudominio.com {
    reverse_proxy localhost:8080
}
```

O con nginx, asumiendo que la terminación TLS ya está resuelta en el bloque `server` de arriba:

```nginx
location / {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Las cookies se comportan distinto según cuál sea:

- **La sesión de Logday Web** se marca `Secure` cuando el proxy avisa con `X-Forwarded-Proto: https`. Por eso conviene reenviar esa cabecera, o usar `COOKIE_SECURE=always`.
- **La cookie del panel** solo se marca `Secure` si la conexión con el contenedor es TLS. Lo normal es que el proxy termine TLS y le hable por HTTP interno, y en ese caso la cookie no lleva `Secure`. Para que eso no debilite la seguridad real, el tráfico entre el proxy y el contenedor tiene que quedar dentro de una red que el proxy controle (la red interna de Docker, o `localhost`), no expuesto directamente a internet sin pasar por el proxy.

El servidor también manda sus propias cabeceras de seguridad, incluida una CSP estricta para `/app` y el panel (ver [`specs/cabeceras-seguridad/`](./specs/cabeceras-seguridad/requirements.md)).

## Desarrollo

Requiere Go (la versión está en `go.mod`) y un compilador de C, porque el driver de SQLite usa CGO.

```bash
make build   # go build -o bin/server ./cmd/server
make run     # go run ./cmd/server
make test    # go test ./...
make lint    # golangci-lint run ./...
make fmt     # golangci-lint fmt ./...
```

`make run` necesita `JWT_SECRET` en el entorno.

## Documentación

- [`specs/`](./specs/README.md): una carpeta por feature con requisitos, diseño y tareas (Spec-Driven Development). [`specs/esquema-datos/`](./specs/esquema-datos/requirements.md) es la fuente de verdad del modelo de datos que comparten los clientes.
- [`specs/convenciones-codigo/`](./specs/convenciones-codigo/requirements.md): estructura de paquetes y convenciones de código.
- [`AGENTS.md`](./AGENTS.md): comandos, contrato con los clientes y lo que no se hace sin preguntar. Sirve para personas y para agentes de IA.
- [`CONTRIBUTING.md`](./CONTRIBUTING.md): flujo de ramas (Gitflow).

## Licencia

[AGPL-3.0-or-later](./LICENSE). Software libre: cualquier persona u organización puede desplegar su propia instancia. Si ofreces una versión modificada como servicio a terceros, el AGPL exige publicar también ese código modificado (copyleft de red).
