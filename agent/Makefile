# agent/Makefile

.PHONY: all build clean test upload

BINARY_NAME=baseline-agent
TARGETS=linux/amd64 linux/arm64

all: build

build:
	@echo "Cross-compiling Agent..."
	docker run --rm -v "$(PWD)":/usr/src/myapp -w /usr/src/myapp golang:1.20-alpine sh -c ' \
	for target in $(TARGETS); do \
		GOOS=$$(echo $$target | cut -d'/' -f1) GOARCH=$$(echo $$target | cut -d'/' -f2) go build -v -o ./dist/$(BINARY_NAME)-$$GOOS-$$GOARCH ./cmd/agent; \
	done'

clean:
	@echo "Cleaning..."
	rm -rf dist

test:
	@echo "Running tests..."
	go test ./...

upload:
	@./build.sh