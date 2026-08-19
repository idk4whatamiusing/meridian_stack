from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="ai", version="0.1.0")


class PredictRequest(BaseModel):
    text: str


class TrainRequest(BaseModel):
    dataset: str = "todo"


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/predict")
def predict(req: PredictRequest):
    # stub - replace with your model inference
    return {"prediction": None, "text_len": len(req.text), "note": "implement your model"}


@app.post("/train")
def train(req: TrainRequest):
    # stub - training runs as a batch job, not inside this service
    return {"job_id": None, "dataset": req.dataset, "note": "implement your training job"}