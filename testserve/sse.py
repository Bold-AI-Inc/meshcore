import asyncio
import json

from .config import DELTA_DELAY_MS

DONE_SIGNAL = "END_OF_STREAM"


def frame(payload, event=None):
    data = payload if isinstance(payload, str) else json.dumps(payload)
    prefix = f"event: {event}\n" if event else ""
    return f"{prefix}data: {data}\n\n"


async def pace():
    if DELTA_DELAY_MS:
        await asyncio.sleep(DELTA_DELAY_MS / 1000)


def chunks(text, scenario):
    if scenario == "empty":
        return []
    if scenario == "unicode":
        text = f"🌊 你好 — {text} — ünïcödé ✅"
    if scenario == "huge":
        text = " ".join([text] * 200)
    words = [w for w in text.split(" ") if w]
    return [w + " " for w in words[:-1]] + words[-1:] if words else []
