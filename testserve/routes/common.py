import asyncio
import json
import random
import time
from contextlib import asynccontextmanager

from fastapi import Request
from fastapi.responses import JSONResponse, StreamingResponse

from .. import state
from ..config import API_KEY, CHAOS_RATE, FLAKY_FAILURES, HANG_SECONDS, REQUIRE_KEY, SLOW_DELAY_MS
from ..scenarios import resolve

KEY_HEADERS = ("authorization", "x-api-key", "x-goog-api-key", "api-key")


def presented_key(headers):
    for name in KEY_HEADERS:
        value = headers.get(name)
        if value:
            return value.split()[-1]
    return ""


async def capture(request: Request, model=""):
    raw = await request.body()
    text = raw.decode("utf-8", "replace")
    try:
        body = json.loads(raw)
    except Exception:
        body = {"_unparsed": text[:2000]}
    headers = {k.lower(): v for k, v in request.headers.items()}
    key = presented_key(headers)
    resolved = model or (body.get("model") if isinstance(body, dict) else "") or ""
    entry = {
        "ts": time.time(),
        "path": request.url.path,
        "method": request.method,
        "query": dict(request.query_params),
        "headers": headers,
        "body": body,
        "body_bytes": len(raw),
        "model": resolved,
        "scenario": resolve(resolved),
        "key_presented": bool(key),
        "key_leaked_in_body": bool(key) and key in text,
        "placeholders_left": [p for p in ("{{content_json}}", "{{max_tokens_json}}",
                                          "{{input_json}}", "{{prompt_json}}", "{{query}}",
                                          "{{model}}", "{{api_key}}") if p in text],
    }
    return state.record(entry)


def error(status, message, code, headers=None):
    return JSONResponse(
        {"error": {"message": message, "type": code, "code": code}},
        status_code=status,
        headers=headers,
    )


async def preflight(entry):
    if REQUIRE_KEY and presented_key(entry["headers"]) != API_KEY:
        return error(401, "invalid or missing api key", "authentication_error")
    if CHAOS_RATE and random.random() < CHAOS_RATE:
        return error(500, "chaos injected failure", "chaos_error")
    scenario = entry["scenario"]
    if scenario == "error-400":
        return error(400, "malformed request reached the provider", "invalid_request_error")
    if scenario == "error-429":
        return error(429, "rate limit exceeded", "rate_limit_error", {"Retry-After": "2"})
    if scenario == "error-500":
        return error(500, "internal provider error", "api_error")
    if scenario == "flaky" and state.flaky_count(entry["model"]) <= FLAKY_FAILURES:
        return error(503, "provider temporarily overloaded", "overloaded_error")
    if scenario == "slow":
        await asyncio.sleep(SLOW_DELAY_MS / 1000)
    if scenario == "hang":
        await asyncio.sleep(HANG_SECONDS)
    return None


@asynccontextmanager
async def tracked():
    state.enter()
    try:
        yield
    finally:
        state.leave()


def sse(generator):
    async def body():
        async with tracked():
            async for chunk in generator():
                yield chunk

    return StreamingResponse(body(), media_type="text/event-stream",
                             headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})
