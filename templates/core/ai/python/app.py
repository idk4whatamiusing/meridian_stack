"""Meridian ai - Python RAG sidecar.

Owns everything embedding/vector:
  /retrieve      query -> pgvector top-k sources
  /ingest        documents -> chunk(512/64) -> embed -> upsert
  /cache_lookup  semantic prompt cache (per user, 7d, Redis)
  /cache_store   store prompt+answer+embedding after generation

The Go binary (:8002, gRPC Ai service) calls these over localhost:8003.
Embeddings: Cloudflare Workers AI bge-base-en-v1.5 (default, 768 dims,
matches db/migrations/0002_rag.sql) or any OpenAI-compatible endpoint
with dimensions=768.
"""

import hashlib
import json
import os
import time

import asyncpg
import httpx
import numpy as np
from fastapi import FastAPI
from pydantic import BaseModel

app = FastAPI(title="ai-rag")

TTL = 7 * 24 * 3600  # every cache = 7 days
CHUNK = 512
OVERLAP = 64
DIM = 768

_pool: asyncpool = None  # type: ignore[name-defined]


async def pool() -> asyncpg.Pool:
    global _pool
    if _pool is None:
        _pool = await asyncpg.create_pool(os.environ["DATABASE_URL"], min_size=1, max_size=5)
    return _pool


def rdb():
    import redis.asyncio as aioredis

    return aioredis.Redis.from_url(os.getenv("REDIS_ADDR", "redis://localhost:6379"), decode_responses=True)


# ---------- embeddings ----------

async def embed(texts: list[str]) -> list[list[float]]:
    provider = os.getenv("EMBED_PROVIDER", os.getenv("AI_PROVIDER", "cloudflare"))
    if provider == "cloudflare":
        account = os.environ["CF_ACCOUNT_ID"]
        token = os.environ["CF_API_TOKEN"]
        model = os.getenv("CF_EMBED_MODEL", "@cf/baai/bge-base-en-v1.5")
        url = f"https://api.cloudflare.com/client/v4/accounts/{account}/ai/run/{model}"
        async with httpx.AsyncClient(timeout=60) as c:
            r = await c.post(url, headers={"Authorization": f"Bearer {token}"},
                             json={"text": texts})
            r.raise_for_status()
            data = r.json()
            return data["result"]["data"]
    # openai-compatible fallback (OpenAI, ollama-openai, vLLM, ...)
    base = os.getenv("OPENAI_BASE_URL", "https://api.openai.com/v1").rstrip("/")
    model = os.getenv("OPENAI_EMBED_MODEL", "text-embedding-3-small")
    async with httpx.AsyncClient(timeout=60) as c:
        r = await c.post(f"{base}/embeddings",
                         headers={"Authorization": f"Bearer {os.getenv('OPENAI_API_KEY', '')}"},
                         json={"model": model, "input": texts, "dimensions": DIM})
        r.raise_for_status()
        return [d["embedding"] for d in r.json()["data"]]


# ---------- chunking ----------

def chunks(text: str) -> list[str]:
    words = text.split()
    step = CHUNK - OVERLAP
    return [" ".join(words[i:i + CHUNK]) for i in range(0, len(words), step)] or [text]


# ---------- models ----------

class Retrieve(BaseModel):
    query: str
    user_id: str = ""
    collection: str = "support"
    k: int = 5


class Ingest(BaseModel):
    documents: list[str]
    collection: str = "support"


class CacheLookup(BaseModel):
    kind: str            # chat | support
    user_id: str
    text: str
    threshold: float = 0.92


class CacheStore(BaseModel):
    kind: str
    user_id: str
    text: str
    answer: str
    sources_hash: str = ""


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/retrieve")
async def retrieve(body: Retrieve):
    [vec] = await embed([body.query])
    p = await pool()
    rows = await p.fetch(
        """SELECT id, title, content, 1 - (embedding <=> $1::vector) AS score
           FROM documents WHERE collection = $2
           ORDER BY embedding <=> $1::vector LIMIT $3""",
        "[" + ",".join(f"{x:.7f}" for x in vec) + "]", body.collection, body.k,
    )
    return {"sources": [
        {"id": str(r["id"]), "title": r["title"], "text": r["content"], "score": float(r["score"])}
        for r in rows
    ]}


@app.post("/ingest")
async def ingest(body: Ingest):
    pieces: list[str] = []
    for doc in body.documents:
        pieces.extend(chunks(doc))
    vecs = await embed(pieces)
    p = await pool()
    async with p.acquire() as conn:
        await conn.executemany(
            "INSERT INTO documents (collection, title, content, embedding) VALUES ($1,$2,$3,$4::vector)",
            [(body.collection, piece[:80], piece, "[" + ",".join(f"{x:.7f}" for x in v) + "]")
             for piece, v in zip(pieces, vecs)],
        )
    return {"chunks": len(pieces)}


def _key(kind: str, user_id: str) -> str:
    return f"cache:semantic:{kind}:{user_id}"


@app.post("/cache_lookup")
async def cache_lookup(body: CacheLookup):
    r = rdb()
    raw_entries = await r.lrange(_key(body.kind, body.user_id), 0, -1)
    if not raw_entries:
        return {"hit": False}

    exact = hashlib.sha256(body.text.encode()).hexdigest()
    entries = [json.loads(e) for e in raw_entries]
    for e in entries:
        if e["prompt_hash"] == exact:
            return {"hit": True, "answer": e["answer"], "sources_hash": e.get("sources_hash", "")}

    q = np.array(await embed([body.text])[0], dtype=np.float32)
    best, best_score = None, -1.0
    for e in entries:
        v = np.array(e["embedding"], dtype=np.float32)
        denom = (np.linalg.norm(q) * np.linalg.norm(v)) or 1.0
        score = float(np.dot(q, v) / denom)
        if score > best_score:
            best, best_score = e, score
    if best and best_score >= body.threshold:
        return {"hit": True, "answer": best["answer"],
                "sources_hash": best.get("sources_hash", ""), "score": best_score}
    return {"hit": False}


@app.post("/cache_store")
async def cache_store(body: CacheStore):
    [vec] = await embed([body.text])
    entry = {
        "prompt_hash": hashlib.sha256(body.text.encode()).hexdigest(),
        "embedding": vec,
        "answer": body.answer,
        "sources_hash": body.sources_hash,
        "ts": time.time(),
    }
    r = rdb()
    key = _key(body.kind, body.user_id)
    pipe = r.pipeline()
    pipe.rpush(key, json.dumps(entry))
    pipe.ltrim(key, -200, -1)          # cap entries per user
    pipe.expire(key, TTL)              # 7 days, refreshed on write
    await pipe.execute()
    return {"ok": True}
