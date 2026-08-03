# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache build-base

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

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
