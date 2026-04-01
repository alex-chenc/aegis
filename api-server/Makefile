.PHONY: build test clean

# 静态编译，禁用CGO
build:
	CGO_ENABLED=0 GOOS=linux go build -o bin/api-server ./cmd/...

test:
	go test ./...

clean:
	rm -rf bin/