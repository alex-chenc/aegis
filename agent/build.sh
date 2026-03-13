#!/bin/bash
set -e

echo "Building Aegis Agent..."

# Cross-compile for amd64
echo "Building linux/amd64..."
GOOS=linux GOARCH=amd64 go build -o ./dist/aegis-agent-linux-amd64 ./cmd/agent

# Cross-compile for arm64
echo "Building linux/arm64..."
GOOS=linux GOARCH=arm64 go build -o ./dist/aegis-agent-linux-arm64 ./cmd/agent

echo "Build complete!"
ls -lh dist/
