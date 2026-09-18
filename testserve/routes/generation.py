import time

from fastapi import APIRouter, Request

from .. import media
from ..config import BASE_URL
from .common import capture, preflight, tracked

router = APIRouter()


def _prompt(body):
    if not isinstance(body, dict):
        return ""
    if body.get("prompt"):
        return str(body["prompt"])
    instances = body.get("instances") or []
    if instances and isinstance(instances[0], dict):
        return str(instances[0].get("prompt", ""))
    return ""


def _payload(entry, name):
    scenario = entry["scenario"]
    if scenario == "badjson":
        return {"broken": True}
    if scenario == "empty":
        return ""
    return media.b64(name)


@router.post("/openai/v1/images/generations")
async def image_base64(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    async with tracked():
        return {
            "created": int(time.time()),
            "data": [{"b64_json": _payload(entry, "dummy.png"), "revised_prompt": _prompt(entry["body"])}],
        }


@router.post("/openai/v1/images/urls")
async def image_url(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    async with tracked():
        return {"created": int(time.time()), "data": [{"url": f"{BASE_URL}/files/dummy.png"}]}


@router.post("/openai/v1/images/batch")
async def image_batch(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    count = int((entry["body"].get("n") if isinstance(entry["body"], dict) else 2) or 2)
    async with tracked():
        return {"created": int(time.time()),
                "outputs": [media.b64("dummy.png") for _ in range(max(1, count))]}


@router.post("/v1/videos/generations")
async def video_url(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    async with tracked():
        return {"id": f"vid_{entry['seq']}", "status": "succeeded",
                "video": {"url": f"{BASE_URL}/files/dummy.mp4", "duration_seconds": 2}}


@router.post("/v1/videos/inline")
async def video_inline(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    async with tracked():
        return {"id": f"vid_{entry['seq']}", "video": {"b64": _payload(entry, "dummy.mp4")}}


@router.post("/gemini/v1beta/models/{model}:predict")
async def gemini_predict(model: str, request: Request):
    entry = await capture(request, model)
    denied = await preflight(entry)
    if denied:
        return denied
    name = "dummy.mp4" if "video" in model else "dummy.png"
    async with tracked():
        return {"predictions": [{"bytesBase64Encoded": _payload(entry, name),
                                 "mimeType": media.media_type(name)}]}
