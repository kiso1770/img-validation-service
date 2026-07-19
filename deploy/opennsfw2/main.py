"""OpenNSFW2 sidecar: POST /classify for raw image bytes."""

import io

import opennsfw2
from fastapi import FastAPI, HTTPException, Request
from PIL import Image

# Guard against decompression bombs: a small-in-bytes file can decode into an
# enormous pixel buffer. ~30MP gives headroom for profile photos/selfies while
# bounding the worst case. Pillow raises Image.DecompressionBombError above this.
Image.MAX_IMAGE_PIXELS = 30_000_000

app = FastAPI()


@app.on_event("startup")
def preload_model() -> None:
    opennsfw2.make_open_nsfw_model()


@app.get("/health/")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/health/model")
def health_model() -> dict[str, object]:
    return {"status": "ok", "model_loaded": True}


@app.post("/classify")
async def classify(request: Request) -> dict[str, float]:
    data = await request.body()
    if not data:
        raise HTTPException(status_code=400, detail="empty image payload")

    try:
        img = Image.open(io.BytesIO(data)).convert("RGB")
    except Exception as exc:  # noqa: BLE001 - surface as 400 to caller
        raise HTTPException(status_code=400, detail=f"invalid image: {exc}") from exc

    score = float(opennsfw2.predict_image(img))
    return {"score": score}
