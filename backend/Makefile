# backend/Makefile

.PHONY: all build clean test docker

BINARY_NAME=backend
GO_CMD=go
IMAGE_NAME=baseline-system/backend
IMAGE_TAG?=latest

all: build

build:
	@echo "Building Go binary..."
	$(GO_CMD) build -o $(BINARY_NAME) ./cmd/server/main.go

clean:
	@echo "Cleaning..."
	$(GO_CMD) clean
	rm -f $(BINARY_NAME)

test:
	@echo "Running tests..."
	$(GO_CMD) test ./...

docker:
	@./build.sh $(IMAGE_NAME) $(IMAGE_TAG)