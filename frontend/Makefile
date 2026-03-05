# frontend/Makefile

.PHONY: all install build clean test docker

NPM_CMD=npm
IMAGE_NAME=baseline-system/frontend
IMAGE_TAG?=latest

all: build

install:
	@echo "Installing dependencies..."
	$(NPM_CMD) install

build: install
	@echo "Building frontend assets..."
	$(NPM_CMD) run build

clean:
	@echo "Cleaning..."
	rm -rf dist node_modules

test:
	@echo "Running tests..."
	$(NPM_CMD) run test

docker:
	@./build.sh $(IMAGE_NAME) $(IMAGE_TAG)