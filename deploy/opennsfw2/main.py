"""OpenNSFW2 sidecar: POST /classify for raw image bytes."""

import io

import opennsfw2
from fastapi import FastAPI, Request
from PIL import Image

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
    img = Image.open(io.BytesIO(data)).convert("RGB")
    score = float(opennsfw2.predict_image(img))
    return {"score": score}
