# syntax=docker/dockerfile:1.4

# Multi-platform: `docker build --platform linux/amd64,linux/arm64` builds
# both images from one host without emulation. Every RUN step runs on the
# build host ($BUILDPLATFORM): the web bundle is the same for every
# platform, Go cross-compiles with xx (CGO included, for go-sqlite3), and
# the final stage only COPYs. Hefesto, the CI runner, is x86_64 with no
# QEMU, so a RUN in a target-platform stage would fail for arm64.

# --- Web frontend (logday-web, separate private repo) ---
# Built from a local sibling checkout, not cloned — logday-web is
# private, so an anonymous `git clone` during the build would fail.
# Requires the extra build context: see README "Building the image"
# for the exact `docker build`/`docker compose` invocation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS web-builder
WORKDIR /web-src
COPY --from=logday-web . .
RUN npm ci
RUN npm run build

# xx provides cross-compilation wrappers (xx-apk, xx-go, xx-verify).
FROM --platform=$BUILDPLATFORM tonistiigi/xx:1.6.1 AS xx

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

COPY --from=xx / /
# clang + lld cross-compile the C side of go-sqlite3 for any target.
RUN apk add --no-cache clang lld

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-builder /web-src/dist ./web/dist

# From here on each target platform gets its own build. musl-dev and gcc
# (for its crt files and libgcc) are installed for the target's sysroot.
ARG TARGETPLATFORM
RUN xx-apk add --no-cache musl-dev gcc

ENV CGO_ENABLED=1
# Versión informativa que publica GET /version (specs/protocolo-version).
ARG VERSION=dev
RUN xx-go build -trimpath \
    -ldflags "-s -w -linkmode external -extldflags=-static -X github.com/jorgebuitragor/logday-server/internal/compat.ServerVersion=${VERSION}" \
    -o /out/server ./cmd/server && \
    xx-verify --static /out/server

# The non-root user and the data directory, prepared on the build host so
# the final stage needs no RUN. Same Alpine tag as the final stage, so its
# /etc/passwd and /etc/group are the final image's own plus the logday user.
FROM --platform=$BUILDPLATFORM alpine:3.20 AS rootfs
RUN adduser -D -u 10001 logday && \
    mkdir -p /data && \
    chown logday:logday /data

FROM alpine:3.20

COPY --from=rootfs /etc/passwd /etc/group /etc/
COPY --from=rootfs --chown=10001:10001 /data /data
USER logday

COPY --from=builder /out/server /usr/local/bin/server

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/server"]
