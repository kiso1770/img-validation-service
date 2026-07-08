#!/usr/bin/env bash
# Smoke test img-validation-service MatchFaces / CheckLiveness gRPC methods (local or dev).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRPC_ADDR="${GRPC_ADDR:-localhost:9090}"

if ! command -v grpcurl >/dev/null 2>&1; then
  echo "grpcurl required: GOBIN=\$HOME/.local/bin go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest" >&2
  exit 1
fi

SAFE_B64='iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
PROTO="${ROOT}/proto/imgvalidation/v1/img_validation.proto"

call() {
  local method="$1" data="$2"
  if grpcurl "${GRPC_ADDR}" list imgvalidation.v1.ImageValidationService >/dev/null 2>&1; then
    grpcurl -plaintext -d "${data}" "${GRPC_ADDR}" "imgvalidation.v1.ImageValidationService/${method}"
  else
    grpcurl -plaintext -d "${data}" \
      -import-path "${ROOT}/proto" \
      -proto imgvalidation/v1/img_validation.proto \
      "${GRPC_ADDR}" "imgvalidation.v1.ImageValidationService/${method}"
  fi
}

echo "=== MatchFaces (stub, same image both sides) ==="
call MatchFaces "{\"source_image\":\"${SAFE_B64}\",\"target_image\":\"${SAFE_B64}\",\"reference_id\":\"e2e-match\"}"

echo
echo "=== CheckLiveness (stub) ==="
call CheckLiveness "{\"image_data\":\"${SAFE_B64}\",\"reference_id\":\"e2e-liveness\"}"

echo
echo "=== ValidateImage purpose=selfie (face check) ==="
call ValidateImage "{\"image_data\":\"${SAFE_B64}\",\"content_type_hint\":\"image/png\",\"purpose\":\"selfie\",\"reference_id\":\"e2e-selfie\"}"

echo
echo "OK: grpc face/liveness smoke completed"
