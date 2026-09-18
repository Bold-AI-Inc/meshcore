import base64
import hashlib


def _digest(data):
    try:
        raw = base64.b64decode(data or "", validate=False)
    except Exception:
        raw = (data or "").encode()
    return raw


def media(kind, media_type, data=None, url=None):
    if url:
        return {"type": kind, "media_type": media_type, "url": url, "bytes": 0, "sha256": ""}
    raw = _digest(data)
    return {
        "type": kind,
        "media_type": media_type,
        "bytes": len(raw),
        "sha256": hashlib.sha256(raw).hexdigest()[:16],
    }


def _kind(media_type):
    head = (media_type or "").split("/")[0]
    return head if head in ("image", "audio", "video") else "document"


def _from_data_uri(kind, url):
    if url.startswith("data:"):
        head, _, data = url.partition(",")
        return media(kind, head[5:].split(";")[0], data)
    return media(kind, "", url=url)


def anthropic_blocks(body):
    out = []
    for message in body.get("messages") or []:
        content = message.get("content")
        if isinstance(content, str):
            out.append({"type": "text", "text": content})
            continue
        for block in content or []:
            if block.get("type") == "text":
                out.append({"type": "text", "text": block.get("text", "")})
            else:
                source = block.get("source") or {}
                out.append(media(
                    block.get("type", "unknown"), source.get("media_type", ""),
                    source.get("data"), source.get("url"),
                ))
    return out


def openai_blocks(body):
    out = []
    for message in body.get("messages") or []:
        content = message.get("content")
        if isinstance(content, str):
            out.append({"type": "text", "text": content})
            continue
        for part in content or []:
            kind = part.get("type")
            if kind == "text":
                out.append({"type": "text", "text": part.get("text", "")})
            elif kind == "image_url":
                out.append(_from_data_uri("image", (part.get("image_url") or {}).get("url", "")))
            elif kind == "file":
                out.append(_from_data_uri("document", (part.get("file") or {}).get("file_data", "")))
            elif kind == "input_audio":
                audio = part.get("input_audio") or {}
                out.append(media("audio", "audio/" + audio.get("format", ""), audio.get("data")))
            else:
                out.append({"type": kind or "unknown", "media_type": "", "bytes": 0, "sha256": ""})
    return out


def gemini_blocks(body):
    out = []
    for content in body.get("contents") or []:
        for part in content.get("parts") or []:
            if "text" in part:
                out.append({"type": "text", "text": part["text"]})
            elif "inline_data" in part or "inlineData" in part:
                d = part.get("inline_data") or part.get("inlineData")
                mt = d.get("mime_type") or d.get("mimeType", "")
                out.append(media(_kind(mt), mt, d.get("data")))
            elif "file_data" in part or "fileData" in part:
                d = part.get("file_data") or part.get("fileData")
                mt = d.get("mime_type") or d.get("mimeType", "")
                out.append(media(_kind(mt), mt, url=d.get("file_uri") or d.get("fileUri")))
    return out


def texts(blocks):
    return [b["text"] for b in blocks if b["type"] == "text"]


def reply_text(blocks, family):
    parts = [f"testserve[{family}] ack"]
    joined = " | ".join(texts(blocks))
    if joined:
        parts.append(f"prompt<{joined[:400]}>")
    for b in blocks:
        if b["type"] == "text":
            continue
        ref = b.get("url") or b.get("sha256")
        parts.append(f"{b['type']}({b.get('media_type') or 'url'},{b.get('bytes', 0)}B,{ref})")
    if len(parts) == 1:
        parts.append("empty-content")
    return " ".join(parts)


def input_tokens(blocks):
    total = 0
    for b in blocks:
        total += max(1, len(b["text"]) // 4) if b["type"] == "text" else 100 + b.get("bytes", 0) // 1000
    return max(1, total)
