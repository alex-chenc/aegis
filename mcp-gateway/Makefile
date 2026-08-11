.PHONY: build test

build:
	CGO_ENABLED=0 GOOS=linux go build -o bin/mcp-gateway ./cmd/mcp-gateway

test:
	go test ./...
