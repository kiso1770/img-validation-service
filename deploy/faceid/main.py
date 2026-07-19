"""InsightFace sidecar: face detection + face verification for img-validation-service.

Endpoints (contract with internal/validation/face.go HTTPFaceClient):
  POST /detect  raw image bytes            -> {"faces": [{"score": float}, ...]}
  POST /verify  {"source_image": b64, "target_image": b64} -> {"similarity", "source_face_count", "target_face_count"}
  GET  /health/model

NOTE: uses the "buffalo_l" model pack (detection + ArcFace recognition). InsightFace's
official model zoo packs are released for research / non-commercial use — confirm
licensing terms before enabling this in a commercial production deployment.
"""

import base64
import io

import cv2
import numpy as np
from fastapi import FastAPI, HTTPException, Request
from insightface.app import FaceAnalysis
from PIL import Image

# Guard against decompression bombs: a small-in-bytes file can decode into an
# enormous pixel buffer. ~30MP gives headroom for profile photos/selfies while
# bounding the worst case. Pillow raises Image.DecompressionBombError above this,
# which is caught by the same try/except as other decode failures below.
Image.MAX_IMAGE_PIXELS = 30_000_000

app = FastAPI()

_face_app: FaceAnalysis | None = None


def get_face_app() -> FaceAnalysis:
    assert _face_app is not None, "model not loaded"
    return _face_app


@app.on_event("startup")
def load_model() -> None:
    global _face_app
    fa = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
    fa.prepare(ctx_id=-1, det_size=(640, 640))
    _face_app = fa


@app.get("/health/")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/health/model")
def health_model() -> dict[str, object]:
    return {"status": "ok", "model_loaded": _face_app is not None}


def _decode_image(data: bytes) -> np.ndarray:
    img = Image.open(io.BytesIO(data)).convert("RGB")
    return cv2.cvtColor(np.array(img), cv2.COLOR_RGB2BGR)


def _cosine_similarity(a: np.ndarray, b: np.ndarray) -> float:
    a_norm = a / np.linalg.norm(a)
    b_norm = b / np.linalg.norm(b)
    # ArcFace embeddings: cosine similarity in [-1, 1]; rescale to [0, 1] so
    # thresholds read like a plain match confidence.
    cos = float(np.dot(a_norm, b_norm))
    return max(0.0, min(1.0, (cos + 1.0) / 2.0))


@app.post("/detect")
async def detect(request: Request) -> dict[str, list[dict[str, float]]]:
    data = await request.body()
    if not data:
        raise HTTPException(status_code=400, detail="empty image payload")
    try:
        img = _decode_image(data)
    except Exception as exc:  # noqa: BLE001 - surface as 400 to caller
        raise HTTPException(status_code=400, detail=f"invalid image: {exc}") from exc

    faces = get_face_app().get(img)
    return {"faces": [{"score": float(f.det_score)} for f in faces]}


@app.post("/verify")
async def verify(request: Request) -> dict[str, float | int]:
    payload = await request.json()
    source_b64 = payload.get("source_image")
    target_b64 = payload.get("target_image")
    if not source_b64 or not target_b64:
        raise HTTPException(status_code=400, detail="source_image and target_image are required")

    try:
        source_img = _decode_image(base64.b64decode(source_b64))
        target_img = _decode_image(base64.b64decode(target_b64))
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=400, detail=f"invalid image: {exc}") from exc

    fa = get_face_app()
    source_faces = fa.get(source_img)
    target_faces = fa.get(target_img)

    if not source_faces or not target_faces:
        return {
            "similarity": 0.0,
            "source_face_count": len(source_faces),
            "target_face_count": len(target_faces),
        }

    # Largest detected face in each image (by bbox area) is used for comparison.
    def largest(faces):
        return max(faces, key=lambda f: (f.bbox[2] - f.bbox[0]) * (f.bbox[3] - f.bbox[1]))

    similarity = _cosine_similarity(largest(source_faces).embedding, largest(target_faces).embedding)

    return {
        "similarity": similarity,
        "source_face_count": len(source_faces),
        "target_face_count": len(target_faces),
    }
