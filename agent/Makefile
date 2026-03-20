.PHONY: all build clean test bpf

BINARY_NAME=aegis-agent
BPF_DIR=internal/ebpf/bpf
BPF_OBJ_DIR=internal/ebpf/bpf/obj

# BPF programs
BPF_PROGRAMS=execve fork exit openat connect setuid setgid capset

all: bpf build

# Build BPF objects
bpf: $(BPF_OBJ_DIR)
	@echo "Building eBPF programs..."
	@for prog in $(BPF_PROGRAMS); do \
		clang -g -O2 -c -target bpf -D__TARGET_ARCH_$$(uname -m | sed 's/x86_64/x86/' | sed 's/aarch64/arm/') \
			-I/usr/include \
			-I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c 2>/dev/null || \
		clang -g -O2 -c -target bpf \
			-I/usr/include \
			-I$(BPF_DIR) \
			-o $(BPF_OBJ_DIR)/$$prog.bpf.o \
			$(BPF_DIR)/$$prog.bpf.c; \
	done
	@echo "eBPF programs built successfully"

$(BPF_OBJ_DIR):
	@mkdir -p $(BPF_OBJ_DIR)

# Build Go binary
build:
	@echo "Building agent..."
	GOOS=linux GOARCH=amd64 go build -o ./dist/$(BINARY_NAME)-linux-amd64 ./cmd/agent
	GOOS=linux GOARCH=arm64 go build -o ./dist/$(BINARY_NAME)-linux-arm64 ./cmd/agent

clean:
	rm -rf dist
	rm -rf $(BPF_OBJ_DIR)

test:
	go test ./...
