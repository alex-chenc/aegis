.PHONY: all build clean test bpf package upload

BINARY_NAME=aegis-agent
BPF_DIR=internal/ebpf/bpf
BPF_OBJ_DIR=internal/ebpf/bpf/obj
ARCH_INCLUDE_DIR?=/usr/include/$(shell uname -m)-linux-gnu

BPF_PROGRAMS=execve fork file tcp_connect accept

all: bpf build package

bpf: $(BPF_OBJ_DIR)
	@echo "Building eBPF programs (ringbuf + perf)..."
	@for prog in $(BPF_PROGRAMS); do \
		echo "  Compiling $$prog (ringbuf)..."; \
		clang -g -O2 -c -target bpf -DAEGIS_EVENT_RINGBUF=1 \
			-mllvm -bpf-stack-size=1024 \
			-D__TARGET_ARCH_$$(uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm/') \
			-I/usr/include -I$(ARCH_INCLUDE_DIR) -I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.ringbuf.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c 2>/dev/null || \
		clang -g -O2 -c -target bpf -DAEGIS_EVENT_RINGBUF=1 \
			-mllvm -bpf-stack-size=1024 \
			-I/usr/include -I$(ARCH_INCLUDE_DIR) -I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.ringbuf.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c; \
		echo "  Compiling $$prog (perf)..."; \
		clang -g -O2 -c -target bpf -DAEGIS_EVENT_PERF=1 \
			-mllvm -bpf-stack-size=1024 \
			-D__TARGET_ARCH_$$(uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm/') \
			-I/usr/include -I$(ARCH_INCLUDE_DIR) -I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.perf.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c 2>/dev/null || \
		clang -g -O2 -c -target bpf -DAEGIS_EVENT_PERF=1 \
			-mllvm -bpf-stack-size=1024 \
			-I/usr/include -I$(ARCH_INCLUDE_DIR) -I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.perf.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c; \
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
