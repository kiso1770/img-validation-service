FROM python:3.11-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    libgl1 \
    libglib2.0-0 \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Vendor the upstream Apache-2.0 licensed Silent-Face-Anti-Spoofing repo:
# model code + both MiniFASNet checkpoints ship inside the repo itself.
RUN git clone --depth 1 https://github.com/minivision-ai/Silent-Face-Anti-Spoofing.git /app/sfas \
    && rm -rf /app/sfas/.git /app/sfas/datasets /app/sfas/images /app/sfas/saved_logs

COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu torch torchvision \
    && pip install --no-cache-dir -r requirements.txt

COPY main.py .

EXPOSE 8083

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8083"]
