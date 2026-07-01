# img-validation-service

gRPC image validation service for y-crave: NSFW moderation (OpenNSFW2 sidecar), format and size checks.

## API

`ImageValidationService.ValidateImage` — see [proto/imgvalidation/v1/img_validation.proto](proto/imgvalidation/v1/img_validation.proto).

Business rejections return `passed=false` with `rejection_reasons`. Infrastructure failures use gRPC `UNAVAILABLE` / `INVALID_ARGUMENT`.

## Local development

```bash
cp .env.example .env
docker compose -f deploy/docker-compose.yml up --build
```

- gRPC: `localhost:9090`
- HTTP health: `http://localhost:8080/api/v1/img-validation-service/healthz`

## Config

| Variable | Default | Description |
|----------|---------|-------------|
| `GRPC_PORT` | `9090` | gRPC listen port |
| `HTTP_PORT` | `8080` | Health endpoints |
| `NSFW_ENABLED` | `false` | Use OpenNSFW2 sidecar (else stub) |
| `NSFW_ENDPOINT` | `http://localhost:8081` | Sidecar base URL |
| `NSFW_SCORE_THRESHOLD` | `0.85` | Reject when score >= threshold |
| `MAX_IMAGE_SIZE_BYTES` | `10485760` | Max upload size (10 MB) |

## Consumers

- **storage-service** — calls `ValidateImage` on confirm-upload for `profile_photo` / `selfie`
- **chat-service** (planned) — image messages before publish

## Proto codegen

```bash
make proto-gen
```
