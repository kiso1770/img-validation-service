FROM python:3.12-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libglib2.0-0 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir -r requirements.txt

COPY main.py .

# Pre-download the buffalo_l model pack so first requests don't pay cold-start cost.
RUN python -c "from insightface.app import FaceAnalysis; fa = FaceAnalysis(name='buffalo_l', providers=['CPUExecutionProvider']); fa.prepare(ctx_id=-1, det_size=(640, 640))" \
    || echo "Model download failed, will retry at runtime"

EXPOSE 8082

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8082"]
