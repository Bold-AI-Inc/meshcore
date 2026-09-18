import time

from fastapi import APIRouter, Request

from .. import media, state
from ..scenarios import SCENARIOS

router = APIRouter(prefix="/_debug")


@router.get("/requests")
async def requests(limit: int = 20, path: str = None, model: str = None):
    return {"count": state.stats["total"], "entries": state.entries(limit, path, model)}


@router.get("/last")
async def last():
    return state.last() or {}


@router.delete("/requests")
async def clear():
    state.clear()
    return {"cleared": True}


@router.get("/stats")
async def stats():
    uptime = time.time() - state.stats["started_at"]
    return dict(state.stats, uptime_seconds=round(uptime, 2),
                requests_per_second=round(state.stats["total"] / uptime, 2) if uptime else 0)


@router.post("/scenario")
async def set_scenario(request: Request):
    body = await request.json()
    state.override["scenario"] = body.get("scenario", "")
    return {"scenario": state.override["scenario"], "available": list(SCENARIOS)}


@router.get("/scenarios")
async def scenarios():
    return {"override": state.override["scenario"], "available": list(SCENARIOS)}


@router.get("/files")
async def files():
    return {name: {"media_type": media.media_type(name), "bytes": len(media.read(name))}
            for name in media.FILES}
