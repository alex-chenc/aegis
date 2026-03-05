#!/bin/bash
set -e

IMAGE_NAME=${1:-"baseline-system/frontend"}
IMAGE_TAG=${2:-"latest"}
BASE_IMAGE="node:18-alpine"

echo "Building Docker image ${IMAGE_NAME}:${IMAGE_TAG}..."

docker build -t ${IMAGE_NAME}:${IMAGE_TAG} \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    -f Dockerfile .

echo "Build complete: ${IMAGE_NAME}:${IMAGE_TAG}"