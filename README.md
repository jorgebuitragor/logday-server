# Logday Server

![local-first](./.github/badge-local-first.svg)

English · [Español](README.es.md)

Self-hosted sync server for [Logday](https://logday.dev/en/), in the spirit of Vaultwarden/Bitwarden: you deploy it as a single Docker container and point each client at your own host.

Sync is **optional**: every Logday client is fully usable without a server. This project is for people who want to sync their data across devices or with their team while keeping full control over where that data lives. There is no hosted Logday.

## Clients

- [Logday Desktop](https://github.com/jorgebuitragor/logday) (Tauri; macOS, Windows and Linux): local-first, sync is turned on in its options.
- Logday Web (React): this server serves it at `/app`. It's the only client that always needs the server.
- [Logday Mobile](https://github.com/jorgebuitragor/logday-mobile) (Expo; Android and iOS): local-first, with optional sync. In development.

## Status

Implemented and in daily use. All 7 entities (tasks, notes, dailies, overtime and its monthly summary, calendar events and days off) sync with per-field last-writer-wins. Note and daily text uses a CRDT (Yjs, via `ygo`). It has multi-user support with per-account isolation, real-time sync over WebSocket, a shared trash, and a privacy policy with consent. See [`specs/`](./specs/README.md) for the design of each feature.

## Stack

- Go (`net/http` + `chi`), a single static binary.
- SQLite by default (one file, zero configuration), with embedded `goose` migrations.
- WebSocket for real-time sync.
- Web admin panel embedded in the same binary (`html/template`, no SPA).
- Multi-stage Docker build (~25 MB, runs as a non-root user).

## Quick start with the published image

The image is on GitHub Container Registry, for `linux/amd64` and `linux/arm64` (Raspberry Pi 4/5 with a 64-bit OS, ARM VPS, Apple Silicon):

```bash
docker run -d --name logday -p 8080:8080 \
  -e JWT_SECRET="$(openssl rand -base64 32)" \
  -e DATABASE_PATH=/data/logday.db \
  -v logday-data:/data \
  ghcr.io/jorgebuitragor/logday-server:latest
```

Check that it started: `curl http://localhost:8080/health` should answer `ok`. Then open `http://localhost:8080/setup` to create the first admin (see [First boot](#first-boot-two-ways-to-create-the-first-admin)). The web client is at `http://localhost:8080/app`. In production, put it behind HTTPS (see [Reverse proxy and TLS](#reverse-proxy-and-tls)).

Or with Docker Compose:

```yaml
services:
  logday:
    image: ghcr.io/jorgebuitragor/logday-server:latest
    ports:
      - "8080:8080"
    environment:
      DATABASE_PATH: /data/logday.db
      JWT_SECRET: ${JWT_SECRET:?set JWT_SECRET in your .env}
    volumes:
      - logday-data:/data
    restart: unless-stopped

volumes:
  logday-data:
```

## Building the image

You need Docker and Docker Compose. The build also compiles `logday-web`, the web client, and embeds it to serve it at `/app`. Its source lives in a separate repository that is currently private, so the build takes it from a local checkout next to this repo (`../logday-web`) instead of cloning it.

```bash
git clone https://github.com/jorgebuitragor/logday-web.git ../logday-web
cp .env.example .env
# edit .env (see "Environment variables" below)
docker compose --env-file .env up -d --build
```

By default the image is built for your own machine. To build it for another architecture, pass `--platform` (for example `--platform linux/amd64` on an Apple Silicon Mac for an x86_64 server, or `--platform linux/amd64,linux/arm64` for both). The Dockerfile cross-compiles, so this needs no emulation.

### Environment variables

| Variable | Required | Description |
|---|---|---|
| `JWT_SECRET` | Yes | Signs access tokens (JWT). Generate one with `openssl rand -base64 32`. |
| `ADMIN_EMAIL` | No | Email of the first admin, created on first boot. See "First boot" below; it's the alternative to `/setup`. |
| `ADMIN_PASSWORD` | No | Password of the first admin. Goes together with `ADMIN_EMAIL`: if one is missing, neither is used. |
| `DATABASE_PATH` | No | Path of the SQLite file. Defaults to `./data/logday.db`; `docker-compose.yml` sets it to `/data/logday.db`, and the `logday-data` volume persists that directory. |
| `PORT` | No | Internal HTTP port of the binary. Defaults to `8080`. |
| `CORS_ALLOWED_ORIGINS` | No | Exact origins, comma-separated, allowed to call the API from another domain (for example, a Logday Web deployed separately). Empty means CORS is off. No wildcards. |
| `COOKIE_SECURE` | No | `always` always marks Logday Web's session cookie as `Secure`. Use it if your proxy chain doesn't forward `X-Forwarded-Proto` correctly (for example, a CDN with "flexible" TLS). |
| `TRUSTED_PROXIES` | No | IPs or CIDRs of your reverse proxies, comma-separated. Only requests coming from them can say who the real client is (`X-Forwarded-For`, or `CLIENT_IP_HEADER`). Empty, the server uses the connection's IP: behind a proxy, every client then shares one login rate limit. See "Reverse proxy and TLS". |
| `CLIENT_IP_HEADER` | No | Header with the client IP, read only from `TRUSTED_PROXIES`. Empty means `X-Forwarded-For`. Behind Cloudflare, if your proxy doesn't trust Cloudflare's addresses, use `CF-Connecting-IP`. |
| `MAX_BODY_MB` | No | Body limit for authenticated requests (notes, sync), in MB. Default 16. `/auth/*` is capped at 64 KiB and the panel at 1 MiB. |

### First boot: two ways to create the first admin

**Option A: environment variables** (non-interactive, good for automated deployments). Set `ADMIN_EMAIL`/`ADMIN_PASSWORD` before the first start. The server creates that user as admin the first time it boots with an empty database. On later boots these variables are ignored, because at least one user already exists.

**Option B: `/setup` in the browser.** If you don't set those variables, the server still starts (it doesn't fail) and serves a form at `http://<your-host>:8080/setup` to create the first admin there. Once created, that page is locked: it can't be used again to create a second "first admin".

Either way you end up with an admin account that can sign in at `http://<your-host>:8080/admin/panel/login`.

## Client API

The full sync API contract (auth, CRUD per resource, the `/sync/changes` feed and the `/ws` protocol) is documented in [`openapi.yaml`](./openapi.yaml) (OpenAPI 3.0.3). Open it in [Swagger Editor](https://editor.swagger.io) or any OpenAPI viewer to browse it.

## Admin panel

Once at least one admin exists, `http://<your-host>:8080/admin/panel` lets you, without touching the API by hand:

- See every user of the instance (active and deactivated).
- Create users and promote them to admin or demote them.
- Deactivate (reversibly) or restore a user.
- Reset another user's password. An admin does it, so it needs no SMTP; there's no self-service recovery yet.
- See and revoke the devices and sessions of **any** user of the instance, not just your own.

See [`specs/panel-admin/`](./specs/panel-admin/requirements.md) for the design details: why the panel session is its own cookie instead of reusing the sync clients' token model, CSRF, and so on.

**Production recommendation:** the panel (`/admin/panel/*`) is the only part of the server meant to be used by a person in a browser. The sync API (`/auth`, `/tasks`, `/sync/changes`, `/ws`, etc.) is used by Logday clients and can't sit behind an interactive login. If your proxy or CDN supports per-path access rules (for example Cloudflare Access, Traefik's `forward_auth`, or nginx's `auth_request` with an SSO provider), add that layer **only on `/admin/panel/*`** as a second factor in front of the panel's own login. Leave the rest of the domain without it, so sync clients keep working. It's the same defense in depth recommended for any self-hosted admin dashboard (Coolify, Vaultwarden, etc.): the app's authentication plus the edge's, as two independent layers.

## Reverse proxy and TLS

The server speaks plain HTTP on purpose: it doesn't manage certificates or terminate TLS itself, just like Vaultwarden. To expose it over HTTPS, put a reverse proxy in front (Caddy, nginx, Traefik, whatever you already use). A minimal example with Caddy, which handles Let's Encrypt automatically:

```caddyfile
logday.yourdomain.com {
    reverse_proxy localhost:8080
}
```

Or with nginx, assuming TLS termination is already set up in the surrounding `server` block:

```nginx
location / {
    proxy_pass http://localhost:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

The cookies behave differently:

- **Logday Web's session** is marked `Secure` when the proxy sends `X-Forwarded-Proto: https`. That's why you should forward that header, or use `COOKIE_SECURE=always`.
- **The panel cookie** is only marked `Secure` if the connection to the container is TLS. Usually the proxy terminates TLS and talks to the container over internal HTTP, and then the cookie has no `Secure` flag. So that this doesn't weaken real security, the traffic between the proxy and the container must stay inside a network the proxy controls (Docker's internal network, or `localhost`), not exposed directly to the internet without going through the proxy.

**The client's real IP.** Behind a proxy, every request reaches the server from the proxy's address. The login rate limit counts failed attempts per IP, so until you set `TRUSTED_PROXIES` all clients share one counter, and anyone could slow down everyone's logins. Set it to the proxy's address (`127.0.0.1` if it runs on the same host) or to its Docker network (`docker network inspect <network>`). The server then reads `X-Forwarded-For`, which Caddy, nginx (with the line above) and Traefik send. If Cloudflare sits in front of your proxy and the proxy doesn't trust Cloudflare's addresses, the real IP only arrives in `CF-Connecting-IP`: set `CLIENT_IP_HEADER=CF-Connecting-IP`, and make sure the proxy can only be reached through Cloudflare. The panel's device list shows the IP the server sees, which is a quick way to check the setting.

The server also sends its own security headers, including a strict CSP for `/app` and the panel (see [`specs/cabeceras-seguridad/`](./specs/cabeceras-seguridad/requirements.md)).

## Development

You need Go (the version is in `go.mod`) and a C compiler, because the SQLite driver uses CGO.

```bash
make build   # go build -o bin/server ./cmd/server
make run     # go run ./cmd/server
make test    # go test ./...
make lint    # golangci-lint run ./...
make fmt     # golangci-lint fmt ./...
```

`make run` needs `JWT_SECRET` in the environment.

## Documentation

The specs, the project docs and the code comments are written in Spanish.

- [`specs/`](./specs/README.md): one folder per feature with requirements, design and tasks (Spec-Driven Development). [`specs/esquema-datos/`](./specs/esquema-datos/requirements.md) is the source of truth for the data model shared by the clients.
- [`specs/convenciones-codigo/`](./specs/convenciones-codigo/requirements.md): package structure and code conventions.
- [`AGENTS.md`](./AGENTS.md): commands, the contract with the clients, and what isn't done without asking. Written for people and for AI agents.
- [`CONTRIBUTING.md`](./CONTRIBUTING.md): branching flow (Gitflow).

## License

[AGPL-3.0-or-later](./LICENSE). Free software: anyone, person or organization, can run their own instance. If you offer a modified version as a service to others, the AGPL requires you to publish that modified code too (network copyleft).
