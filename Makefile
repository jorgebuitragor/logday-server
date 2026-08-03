.PHONY: build run test lint fmt

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...
