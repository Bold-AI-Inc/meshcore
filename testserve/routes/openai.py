import time

from fastapi import APIRouter, Request

from ..content import input_tokens, openai_blocks, reply_text
from ..sse import DONE_SIGNAL, chunks, frame, pace
from .common import capture, preflight, sse, tracked

router = APIRouter()


async def chat(entry, blocks, family):
    text = reply_text(blocks, family)
    scenario = entry["scenario"]
    tokens_in = input_tokens(blocks)
    pieces = chunks(text, scenario)
    created = int(time.time())
    base = {"id": f"chatcmpl_{entry['seq']}", "created": created, "model": entry["model"]}
    usage = {"prompt_tokens": tokens_in, "completion_tokens": len(pieces),
             "total_tokens": tokens_in + len(pieces)}

    if not entry["body"].get("stream"):
        async with tracked():
            return dict(base, object="chat.completion", usage=usage, choices=[{
                "index": 0, "finish_reason": "stop",
                "message": {"role": "assistant", "content": "".join(pieces)},
            }])

    async def gen():
        if scenario == "badjson":
            yield "data: {this is not json\n\n"
            yield "data: [DONE]\n\n"
            return
        yield frame(dict(base, object="chat.completion.chunk", choices=[
            {"index": 0, "delta": {"role": "assistant"}, "finish_reason": None}]))
        for i, piece in enumerate(pieces):
            if scenario == "conn-reset" and i == 2:
                raise RuntimeError("testserve forced connection reset")
            if scenario == "keepalive" and i % 3 == 0:
                yield "data:\n\n"
            await pace()
            yield frame(dict(base, object="chat.completion.chunk", choices=[
                {"index": 0, "delta": {"content": piece}, "finish_reason": None}]))
        yield frame(dict(base, object="chat.completion.chunk", choices=[
            {"index": 0, "delta": {}, "finish_reason": "stop"}]))
        if scenario != "nousage":
            yield frame(dict(base, object="chat.completion.chunk", choices=[], usage=usage))
        yield frame(DONE_SIGNAL if scenario == "customdone" else "[DONE]")

    return sse(gen)


@router.post("/openai/v1/chat/completions")
async def completions(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied
    return await chat(entry, openai_blocks(entry["body"]), "openai")
