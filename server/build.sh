#!/bin/bash
set -e

IMAGE_NAME=${1:-"aegis-system/server"}
IMAGE_TAG=${2:-"latest"}
BASE_IMAGE="golang:1.25-alpine"

echo "Building Docker image ${IMAGE_NAME}:${IMAGE_TAG}..."

docker build -t ${IMAGE_NAME}:${IMAGE_TAG} \
    --build-arg BASE_IMAGE=${BASE_IMAGE} \
    -f Dockerfile .

echo "Build complete: ${IMAGE_NAME}:${IMAGE_TAG}"