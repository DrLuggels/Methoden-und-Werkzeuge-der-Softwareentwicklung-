"""FastAPI-Backend von PixelWise (Block 5+6 des Kursskripts).

Endpunkte:
  GET  /health    -- Liveness + Modellversion
  GET  /results   -- letzte 20 Vorhersagen aus der Datenbank
  POST /classify  -- Ziffer klassifizieren (API-Key-Auth + Rate-Limit), speichern
"""
import os

import numpy as np
from fastapi import Depends, FastAPI, Header, HTTPException, Request
from pydantic import BaseModel
from slowapi import Limiter, _rate_limit_exceeded_handler
from slowapi.errors import RateLimitExceeded
from slowapi.middleware import SlowAPIMiddleware
from slowapi.util import get_remote_address

from app.classifier import classify_batch
from app.models import Prediction, SessionLocal


class ClassifyRequest(BaseModel):
    pixels: list[list[int]]


class ClassifyResponse(BaseModel):
    prediction: str
    confidence: float
    scores: dict[str, float]


def verify_api_key(x_api_key: str = Header(...)):
    if x_api_key != os.getenv("SECRET_API_KEY"):
        raise HTTPException(status_code=401, detail="Invalid API key")


app = FastAPI(title="PixelWise")
limiter = Limiter(key_func=get_remote_address)
app.state.limiter = limiter
app.add_exception_handler(RateLimitExceeded, _rate_limit_exceeded_handler)
app.add_middleware(SlowAPIMiddleware)


@app.get("/health")
def health():
    return {"status": "ok", "model_version": "v1"}


@app.get("/results")
def results():
    db = SessionLocal()
    try:
        rows = (
            db.query(Prediction)
            .order_by(Prediction.created_at.desc())
            .limit(20)
            .all()
        )
        return {
            "results": [
                {
                    "id": r.id,
                    "prediction": r.prediction,
                    "confidence": r.confidence,
                    "model_version": r.model_version,
                    "created_at": r.created_at.isoformat(),
                }
                for r in rows
            ]
        }
    finally:
        db.close()


@app.post(
    "/classify",
    response_model=ClassifyResponse,
    dependencies=[Depends(verify_api_key)],
)
@limiter.limit("30/minute")
def classify(request: Request, req: ClassifyRequest):
    arr = np.array(req.pixels, dtype=np.uint8)[np.newaxis]
    result = classify_batch(arr)[0]
    db = SessionLocal()
    try:
        db.add(
            Prediction(
                prediction=result["prediction"],
                confidence=result["confidence"],
                client_ip=request.client.host,
                model_version="v1",
            )
        )
        db.commit()
    finally:
        db.close()
    return result
