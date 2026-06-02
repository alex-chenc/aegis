.PHONY: all build clean test bpf package upload docker-base-image docker-build

BINARY_NAME=aegis-agent
BPF_DIR=internal/ebpf/bpf
BPF_OBJ_DIR=internal/ebpf/bpf/obj
ROOT_DIR?=$(abspath ..)
EBPF_BASE_IMAGE?=aegis-agent-builder-ubi8:5.8.0
AGENT_ARTIFACT_IMAGE?=aegis-agent-artifacts:local
ARCH_INCLUDE_DIR?=/usr/include/$(shell uname -m)-linux-gnu
BPF_TARGET_ARCH?=$(shell uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm/')
BPF_TRANSPORT?=all
BPF_INCLUDE_DIR?=/opt/aegis/ebpf/include

BPF_PROGRAMS=execve fork file tcp_connect accept

all: bpf build package

bpf: $(BPF_OBJ_DIR)
	@case "$(BPF_TARGET_ARCH)" in x86|arm) ;; *) echo "BPF_TARGET_ARCH must be x86 or arm, got $(BPF_TARGET_ARCH)"; exit 1 ;; esac
	@case "$(BPF_TRANSPORT)" in all|ringbuf|perf) ;; *) echo "BPF_TRANSPORT must be all, ringbuf, or perf, got $(BPF_TRANSPORT)"; exit 1 ;; esac
	@echo "Building eBPF programs (arch=$(BPF_TARGET_ARCH), transport=$(BPF_TRANSPORT))..."
	@set -e; \
	for prog in $(BPF_PROGRAMS); do \
		for transport in ringbuf perf; do \
			if [ "$(BPF_TRANSPORT)" != "all" ] && [ "$(BPF_TRANSPORT)" != "$$transport" ]; then \
				continue; \
			fi; \
			if [ "$$transport" = "ringbuf" ]; then macro=AEGIS_EVENT_RINGBUF; else macro=AEGIS_EVENT_PERF; fi; \
			echo "  Compiling $$prog ($$transport)..."; \
			clang -g -O2 -c -target bpf -D$$macro=1 \
				-mllvm -bpf-stack-size=1024 \
				-D__TARGET_ARCH_$(BPF_TARGET_ARCH) \
				-I/usr/include -I$(ARCH_INCLUDE_DIR) -I$(BPF_INCLUDE_DIR) -I$(BPF_DIR) \
				-o $(BPF_OBJ_DIR)/$$prog.$$transport.bpf.o \
				$(BPF_DIR)/$$prog.bpf.c; \
		done; \
	done
	@echo "eBPF programs built successfully"

$(BPF_OBJ_DIR):
	@mkdir -p $(BPF_OBJ_DIR)

build:
	@echo "Building agent..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist/$(BINARY_NAME)-linux-amd64 ./cmd/agent
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ./dist/$(BINARY_NAME)-linux-arm64 ./cmd/agent

package:
	@echo "Packaging agent..."
	@mkdir -p dist/bpf
	@cp $(BPF_OBJ_DIR)/*.ringbuf.bpf.o dist/bpf/ 2>/dev/null || true
	@cp $(BPF_OBJ_DIR)/*.perf.bpf.o dist/bpf/ 2>/dev/null || true
	cd dist && tar -czf $(BINARY_NAME).tar.gz $(BINARY_NAME)-linux-amd64 bpf/*.bpf.o
	@echo "Package created: dist/$(BINARY_NAME).tar.gz"

upload:
	@echo "Uploading to MinIO..."
	@if [ -z "$(MINIO_ENDPOINT)" ]; then \
		echo "Error: MINIO_ENDPOINT not set"; \
		exit 1; \
	fi
	mc alias set local $(MINIO_ENDPOINT) $(MINIO_ACCESS_KEY) $(MINIO_SECRET_KEY) 2>/dev/null || true
	mc cp dist/$(BINARY_NAME).tar.gz local/agent-artifacts/$(BINARY_NAME).tar.gz
	@echo "Upload complete: agent-artifacts/$(BINARY_NAME).tar.gz"

clean:
	rm -rf dist
	rm -rf $(BPF_OBJ_DIR)

test:
	go test ./...

docker-base-image:
	docker build -f $(ROOT_DIR)/docker/ebpf-builder-base/Dockerfile -t $(EBPF_BASE_IMAGE) $(ROOT_DIR)

docker-build: docker-base-image
	docker build -f Dockerfile --build-arg EBPF_BASE_IMAGE=$(EBPF_BASE_IMAGE) -t $(AGENT_ARTIFACT_IMAGE) .
