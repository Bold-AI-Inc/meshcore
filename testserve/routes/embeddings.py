import hashlib

from fastapi import APIRouter, Request

from ..config import EMBEDDING_DIM
from .common import capture, preflight, tracked

router = APIRouter()


def _text(value):
    if isinstance(value, list):
        return " ".join(str(v) for v in value)
    if isinstance(value, dict):
        return " ".join(str(p.get("text", "")) for p in value.get("parts") or [])
    return str(value or "")


def vector(text, dim=None):
    dim = dim or EMBEDDING_DIM
    digest = hashlib.sha256(text.encode()).digest()
    raw = [(digest[i % len(digest)] / 127.5) - 1 for i in range(dim)]
    norm = sum(v * v for v in raw) ** 0.5 or 1.0
    return [round(v / norm, 6) for v in raw]


@router.post("/openai/v1/embeddings")
async def openai_embeddings(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    text = _text(entry["body"].get("input"))
    async with tracked():
        if entry["scenario"] == "badjson":
            return {"object": "list", "data": [{"embedding": "not-a-vector"}]}
        if entry["scenario"] == "empty":
            return {"object": "list", "data": []}
        return {
            "object": "list",
            "model": entry["model"],
            "data": [{"object": "embedding", "index": 0, "embedding": vector(text)}],
            "usage": {"prompt_tokens": max(1, len(text) // 4), "total_tokens": max(1, len(text) // 4)},
        }


@router.post("/gemini/v1beta/models/{model}:embedContent")
async def gemini_embeddings(model: str, request: Request):
    entry = await capture(request, model)
    denied = await preflight(entry)
    if denied:
        return denied
    text = _text(entry["body"].get("content") or entry["body"].get("input"))
    async with tracked():
        return {"embedding": {"values": vector(text)}}
