from fastapi import APIRouter, Request

from ..content import anthropic_blocks, input_tokens, reply_text
from ..sse import DONE_SIGNAL, chunks, frame, pace
from .common import capture, preflight, sse, tracked

router = APIRouter()


@router.post("/anthropic/v1/messages")
async def messages(request: Request):
    entry = await capture(request)
    denied = await preflight(entry)
    if denied:
        return denied

    blocks = anthropic_blocks(entry["body"])
    text = reply_text(blocks, "anthropic")
    scenario = entry["scenario"]
    tokens_in = input_tokens(blocks)
    pieces = chunks(text, scenario)

    if not entry["body"].get("stream"):
        async with tracked():
            return {
                "id": f"msg_{entry['seq']}",
                "type": "message",
                "role": "assistant",
                "model": entry["model"],
                "content": [{"type": "text", "text": "".join(pieces)}],
                "stop_reason": "end_turn",
                "usage": {"input_tokens": tokens_in, "output_tokens": len(pieces)},
            }

    async def gen():
        if scenario == "badjson":
            yield "data: {this is not json\n\n"
            return
        yield frame({
            "type": "message_start",
            "message": {
                "id": f"msg_{entry['seq']}", "type": "message", "role": "assistant",
                "model": entry["model"],
                "usage": {"input_tokens": tokens_in, "output_tokens": 0},
            },
        }, "message_start")
        yield frame({"type": "content_block_start", "index": 0,
                     "content_block": {"type": "text", "text": ""}}, "content_block_start")
        for i, piece in enumerate(pieces):
            if scenario == "conn-reset" and i == 2:
                raise RuntimeError("testserve forced connection reset")
            if scenario == "keepalive" and i % 3 == 0:
                yield "data:\n\n"
            await pace()
            yield frame({"type": "content_block_delta", "index": 0,
                         "delta": {"type": "text_delta", "text": piece}}, "content_block_delta")
        yield frame({"type": "content_block_stop", "index": 0}, "content_block_stop")
        if scenario != "nousage":
            yield frame({"type": "message_delta", "delta": {"stop_reason": "end_turn"},
                         "usage": {"output_tokens": len(pieces)}}, "message_delta")
        yield frame({"type": "message_stop"}, "message_stop")
        if scenario == "customdone":
            yield frame(DONE_SIGNAL)

    return sse(gen)
