#!/usr/bin/env bash
# Smoke test img-validation-service gRPC ValidateImage (local or dev).
set -euo pipefail

GRPC_ADDR="${GRPC_ADDR:-localhost:9090}"

if ! command -v grpcurl >/dev/null 2>&1; then
  echo "grpcurl required: go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest" >&2
  exit 1
fi

SAFE_B64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='

echo "=== ValidateImage (safe PNG) on ${GRPC_ADDR} ==="
grpcurl -plaintext \
  -d "{\"image_data\":\"${SAFE_B64}\",\"content_type_hint\":\"image/png\",\"purpose\":\"profile_photo\",\"reference_id\":\"e2e-safe\"}" \
  "${GRPC_ADDR}" imgvalidation.v1.ImageValidationService/ValidateImage

echo
echo "OK: grpc ValidateImage pass smoke completed"
