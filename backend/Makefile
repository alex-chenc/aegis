.PHONY: build test clean install

# 静态编译，禁用CGO
build:
	CGO_ENABLED=0 GOOS=linux go build -o backend ./cmd/server/main.go

test:
	go test ./...

clean:
	rm -f backend

install:
	@echo "Backend binary built successfully"
