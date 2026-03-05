#!/bin/bash
set -e

MINIO_ENDPOINT=${MINIO_ENDPOINT:-"http://localhost:9000"}
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY:-"minio_admin"}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY:-"a_third_strong_secret_password"}
MINIO_BUCKET="agent-artifacts"

make build

echo "Configuring MinIO client..."
mc alias set myminio ${MINIO_ENDPOINT} ${MINIO_ACCESS_KEY} ${MINIO_SECRET_KEY}

mc mb myminio/${MINIO_BUCKET} --ignore-existing

echo "Uploading artifacts to MinIO bucket: ${MINIO_BUCKET}..."
mc cp --recursive dist/ myminio/${MINIO_BUCKET}/

echo "Upload complete."