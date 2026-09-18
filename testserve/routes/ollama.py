from fastapi import APIRouter, Request

from ..content import openai_blocks
from .common import capture, error, preflight
from .openai import chat

router = APIRouter()
ALLOWED = ("text", "image")


@router.post("/ollama/v1/chat/completions")
async def completions(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    blocks = openai_blocks(entry["body"])
    unsupported = [b["type"] for b in blocks if b["type"] not in ALLOWED]
    if unsupported:
        return error(400, f"ollama accepts text and image only, got {unsupported}",
                     "invalid_request_error")
    return await chat(entry, blocks, "ollama")
