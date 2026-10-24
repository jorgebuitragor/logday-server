# syntax=docker/dockerfile:1.4

# --- Web frontend (logday-web, separate private repo) ---
# Built from a local sibling checkout, not cloned — logday-web is
# private, so an anonymous `git clone` during the build would fail.
# Requires the extra build context: see README "Levantar el servidor"
# for the exact `docker build`/`docker compose` invocation.
FROM node:22-alpine AS web-builder
WORKDIR /web-src
COPY --from=logday-web . .
RUN npm ci
RUN npm run build

FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /web-src/dist ./web/dist

ENV CGO_ENABLED=1
RUN go build -trimpath \
    -ldflags '-s -w -linkmode external -extldflags "-static"' \
    -o /out/server ./cmd/server

FROM alpine:3.20

RUN adduser -D -u 10001 logday && \
    mkdir -p /data && \
    chown logday:logday /data
USER logday

COPY --from=builder /out/server /usr/local/bin/server

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/server"]
