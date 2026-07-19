"""Anti-spoof (presentation attack detection) sidecar for img-validation-service.

Wraps the upstream minivision-ai/Silent-Face-Anti-Spoofing inference code
(vendored into the image at /app/sfas by the Dockerfile, Apache-2.0 licensed)
behind a single-endpoint HTTP contract matching internal/validation/liveness.go
HTTPAntiSpoofChecker:

  POST /classify  raw image bytes  -> {"live": bool, "score": float}
  GET  /health/model

The upstream reference implementation (test.py) reloads both MiniFASNet
checkpoints from disk on every call; here they are loaded once and cached,
since this runs as a long-lived service rather than a one-shot script.
"""

import io
import os
import sys

# The vendored repo's modules are hardcoded to resolve resources relative to
# its own root ("./resources/..."), so run from there.
SFAS_ROOT = "/app/sfas"
os.chdir(SFAS_ROOT)
sys.path.insert(0, SFAS_ROOT)

import cv2  # noqa: E402
import numpy as np  # noqa: E402
import torch  # noqa: E402
import torch.nn.functional as F  # noqa: E402
from fastapi import FastAPI, HTTPException, Request  # noqa: E402
from PIL import Image  # noqa: E402

from src.anti_spoof_predict import MODEL_MAPPING, AntiSpoofPredict  # noqa: E402
from src.data_io import transform as trans  # noqa: E402
from src.generate_patches import CropImage  # noqa: E402
from src.utility import get_kernel, parse_model_name  # noqa: E402

# Guard against decompression bombs: a small-in-bytes file can decode into an
# enormous pixel buffer. ~30MP gives headroom for profile photos/selfies while
# bounding the worst case. Pillow raises Image.DecompressionBombError above this,
# which is caught by the same try/except as other decode failures below.
Image.MAX_IMAGE_PIXELS = 30_000_000

MODEL_DIR = os.path.join(SFAS_ROOT, "resources", "anti_spoof_models")

app = FastAPI()


class CachedAntiSpoofPredict(AntiSpoofPredict):
    """AntiSpoofPredict variant that keeps loaded model weights in memory."""

    def __init__(self, device_id: int) -> None:
        super().__init__(device_id)
        self._model_cache: dict[str, torch.nn.Module] = {}

    def _load_cached_model(self, model_path: str) -> torch.nn.Module:
        model = self._model_cache.get(model_path)
        if model is not None:
            return model

        model_name = os.path.basename(model_path)
        h_input, w_input, model_type, _ = parse_model_name(model_name)
        kernel_size = get_kernel(h_input, w_input)
        model = MODEL_MAPPING[model_type](conv6_kernel=kernel_size).to(self.device)

        state_dict = torch.load(model_path, map_location=self.device)
        first_key = next(iter(state_dict))
        if first_key.startswith("module."):
            state_dict = {k[len("module."):]: v for k, v in state_dict.items()}
        model.load_state_dict(state_dict)
        model.eval()

        self._model_cache[model_path] = model
        return model

    def predict(self, img: np.ndarray, model_path: str) -> np.ndarray:
        test_transform = trans.Compose([trans.ToTensor()])
        img_t = test_transform(img).unsqueeze(0).to(self.device)
        model = self._load_cached_model(model_path)
        with torch.no_grad():
            result = model.forward(img_t)
            result = F.softmax(result, dim=1).cpu().numpy()
        return result


_predictor: CachedAntiSpoofPredict | None = None
_cropper = CropImage()


@app.on_event("startup")
def load_models() -> None:
    global _predictor
    predictor = CachedAntiSpoofPredict(device_id=0)
    # Warm the cache for both checkpoints so the first real request isn't slow.
    for model_name in os.listdir(MODEL_DIR):
        predictor._load_cached_model(os.path.join(MODEL_DIR, model_name))  # noqa: SLF001
    _predictor = predictor


@app.get("/health/")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/health/model")
def health_model() -> dict[str, object]:
    return {"status": "ok", "model_loaded": _predictor is not None}


def _decode_image(data: bytes) -> np.ndarray:
    img = Image.open(io.BytesIO(data)).convert("RGB")
    return cv2.cvtColor(np.array(img), cv2.COLOR_RGB2BGR)


@app.post("/classify")
async def classify(request: Request) -> dict[str, object]:
    data = await request.body()
    if not data:
        raise HTTPException(status_code=400, detail="empty image payload")

    try:
        image = _decode_image(data)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=400, detail=f"invalid image: {exc}") from exc

    assert _predictor is not None, "model not loaded"

    bbox = _predictor.get_bbox(image)
    prediction = np.zeros((1, 3))
    for model_name in os.listdir(MODEL_DIR):
        h_input, w_input, model_type, scale = parse_model_name(model_name)
        patch = _cropper.crop(
            org_img=image,
            bbox=bbox,
            scale=scale,
            out_w=w_input,
            out_h=h_input,
            crop=scale is not None,
        )
        prediction += _predictor.predict(patch, os.path.join(MODEL_DIR, model_name))

    label = int(np.argmax(prediction))
    score = float(prediction[0][label] / 2)
    live = label == 1

    return {"live": live, "score": score}
