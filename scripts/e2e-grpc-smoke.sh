#!/usr/bin/env bash
# Smoke test img-validation-service gRPC ValidateImage (local or dev).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRPC_ADDR="${GRPC_ADDR:-localhost:9090}"
HTTP_ADDR="${HTTP_ADDR:-localhost:8080}"
APP_NAME="${APP_NAME:-img-validation-service}"

if ! command -v grpcurl >/dev/null 2>&1; then
  echo "grpcurl required: GOBIN=\$HOME/.local/bin go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest" >&2
  exit 1
fi

SAFE_B64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
PROTO="${ROOT}/proto/imgvalidation/v1/img_validation.proto"

echo "=== HTTP health on ${HTTP_ADDR} ==="
curl -sf "http://${HTTP_ADDR}/api/v1/${APP_NAME}/healthz" >/dev/null
curl -sf "http://${HTTP_ADDR}/api/v1/${APP_NAME}/ready" >/dev/null
echo "OK: healthz + ready"

echo "=== ValidateImage (safe PNG) on ${GRPC_ADDR} ==="
grpcurl_args=(-plaintext -d "{\"image_data\":\"${SAFE_B64}\",\"content_type_hint\":\"image/png\",\"purpose\":\"profile_photo\",\"reference_id\":\"e2e-safe\"}")
if grpcurl "${GRPC_ADDR}" list imgvalidation.v1.ImageValidationService >/dev/null 2>&1; then
  grpcurl "${grpcurl_args[@]}" "${GRPC_ADDR}" imgvalidation.v1.ImageValidationService/ValidateImage
else
  grpcurl "${grpcurl_args[@]}" \
    -import-path "${ROOT}/proto" \
    -proto imgvalidation/v1/img_validation.proto \
    "${GRPC_ADDR}" imgvalidation.v1.ImageValidationService/ValidateImage
fi

echo
echo "OK: grpc ValidateImage pass smoke completed"
