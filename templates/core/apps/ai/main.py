import os

import httpx
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel

app = FastAPI(title="ai", version="0.2.0")


class PredictRequest(BaseModel):
    text: str


class TrainRequest(BaseModel):
    dataset: str = "todo"


class ChatRequest(BaseModel):
    message: str
    system: str = "You are a concise assistant."


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/predict")
def predict(req: PredictRequest):
    # baseline classifier so the endpoint works out of the box.
    # ponytail: swap for your model; train() is where real fitting happens.
    text = req.text.lower()
    if any(w in text for w in ["bad", "terrible", "hate", "awful"]):
        label = "negative"
    elif any(w in text for w in ["great", "love", "good", "nice"]):
        label = "positive"
    else:
        label = "neutral"
    return {"label": label, "text_len": len(req.text), "model": "baseline-keywords"}


@app.post("/train")
def train(req: TrainRequest):
    # training runs as a batch job, not inside this service
    return {"job_id": None, "dataset": req.dataset, "note": "run training as a separate job"}


@app.post("/chat")
async def chat(req: ChatRequest):
    # real inference via any OpenAI-compatible API (OpenAI, Groq, Ollama, ...)
    key = os.getenv("OPENAI_API_KEY")
    if not key:
        return JSONResponse(
            {"error": "OPENAI_API_KEY not set - configure it to enable chat"},
            status_code=503,
        )
    base = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
    model = os.getenv("OPENAI_MODEL", "gpt-4o-mini")
    try:
        async with httpx.AsyncClient(timeout=60) as client:
            r = await client.post(
                f"{base}/chat/completions",
                headers={"Authorization": f"Bearer {key}"},
                json={
                    "model": model,
                    "messages": [
                        {"role": "system", "content": req.system},
                        {"role": "user", "content": req.message},
                    ],
                },
            )
            r.raise_for_status()
            return {"reply": r.json()["choices"][0]["message"]["content"], "model": model}
    except httpx.HTTPError as e:
        return JSONResponse({"error": f"provider unreachable: {e}"}, status_code=502)