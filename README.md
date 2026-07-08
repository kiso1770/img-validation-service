# img-validation-service

gRPC image validation service for y-crave: NSFW moderation (OpenNSFW2 sidecar), face
detection (InsightFace/`faceid` sidecar), face match and anti-spoof liveness
(Silent-Face-Anti-Spoofing/`antispoof` sidecar), format and size checks.

## API

See [proto/imgvalidation/v1/img_validation.proto](proto/imgvalidation/v1/img_validation.proto).

- `ValidateImage` — format/size/NSFW/face-presence checks for an upload. `purpose=profile_photo`
  and `purpose=selfie` also run face detection (`face_count`, `face_confidence` in the response);
  `selfie` additionally rejects more than one detected face.
- `MatchFaces` — compares a selfie against a reference photo, returns cosine `similarity` and
  `matched` (similarity >= `FACE_MATCH_THRESHOLD`).
- `CheckLiveness` — anti-spoof (presentation attack detection) on a single frame.

Business rejections return `passed=false` with `rejection_reasons` (`nsfw`, `unsupported_format`,
`too_large`, `no_face`, `multiple_faces`). Infrastructure failures use gRPC `UNAVAILABLE` / `INVALID_ARGUMENT`.

⚠️ **Licensing note**: the `faceid` sidecar uses InsightFace's `buffalo_l` model pack, which
InsightFace distributes for research/non-commercial use. Confirm licensing terms (or swap in a
permissively licensed model) before relying on face match in a commercial production path. The
`antispoof` sidecar vendors minivision-ai/Silent-Face-Anti-Spoofing, which is Apache-2.0 licensed.

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
| `FACE_ENABLED` | `false` | Use faceid sidecar for detect/match (else stub, always passes) |
| `FACE_ENDPOINT` | `http://localhost:8082` | faceid sidecar base URL |
| `FACE_MATCH_THRESHOLD` | `0.40` | Min cosine similarity to consider two faces the same person |
| `ANTISPOOF_ENABLED` | `false` | Use antispoof sidecar for liveness (else stub, always live) |
| `ANTISPOOF_ENDPOINT` | `http://localhost:8083` | antispoof sidecar base URL |
| `ANTISPOOF_MIN_SCORE` | `0.70` | Min live-score to consider a frame genuine |

## Consumers

- **storage-service** — calls `ValidateImage` on confirm-upload for `profile_photo` / `selfie`
- **chat-service** (planned) — image messages before publish

## Proto codegen

```bash
make proto-gen
```
