from fastapi import APIRouter, Request

from ..content import gemini_blocks, input_tokens, reply_text
from ..sse import DONE_SIGNAL, chunks, frame, pace
from .common import capture, preflight, sse, tracked

router = APIRouter()


def _chunk(text, tokens_in, produced, include_usage=True):
    payload = {
        "candidates": [{
            "content": {"role": "model", "parts": [{"text": text}]},
            "index": 0,
        }],
        "modelVersion": "testserve",
    }
    if include_usage:
        payload["usageMetadata"] = {
            "promptTokenCount": tokens_in,
            "candidatesTokenCount": produced,
            "totalTokenCount": tokens_in + produced,
        }
    return payload


@router.post("/gemini/v1beta/models/{model}:streamGenerateContent")
async def stream_generate(model: str, request: Request):
    entry = await capture(request, model)
    denied = await preflight(entry)
    if denied:
        return denied

    blocks = gemini_blocks(entry["body"])
    scenario = entry["scenario"]
    tokens_in = input_tokens(blocks)
    pieces = chunks(reply_text(blocks, "gemini"), scenario)

    async def gen():
        if scenario == "badjson":
            yield "data: {this is not json\n\n"
            return
        for i, piece in enumerate(pieces):
            if scenario == "conn-reset" and i == 2:
                raise RuntimeError("testserve forced connection reset")
            if scenario == "keepalive" and i % 3 == 0:
                yield "data:\n\n"
            await pace()
            yield frame(_chunk(piece, tokens_in, i + 1, scenario != "nousage"))
        if not pieces:
            yield frame(_chunk("", tokens_in, 0, scenario != "nousage"))
        if scenario == "customdone":
            yield frame(DONE_SIGNAL)

    return sse(gen)


@router.post("/gemini/v1beta/models/{model}:generateContent")
async def generate(model: str, request: Request):
    entry = await capture(request, model)
    denied = await preflight(entry)
    if denied:
        return denied
    blocks = gemini_blocks(entry["body"])
    pieces = chunks(reply_text(blocks, "gemini"), entry["scenario"])
    async with tracked():
        return _chunk("".join(pieces), input_tokens(blocks), len(pieces))
