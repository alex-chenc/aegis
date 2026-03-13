.PHONY: all build clean test

BINARY_NAME=aegis-agent

all: build

build:
	@echo "Building agent..."
	GOOS=linux GOARCH=amd64 go build -o ./dist/$(BINARY_NAME)-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -o ./dist/$(BINARY_NAME)-linux-arm64 ./cmd/agent

clean:
	rm -rf dist

test:
	go test ./...
