"""Content-block builders -- match the wire contract exactly
(server/internal/gateway/contract.go's contentBlockRequest): each block is
{"type", "text"?, "media_type"?, "data"?, "url"?}.

Every builder besides text_block() accepts path/data/media_type/url as
either a single value or a list. Passing a list on any one of them builds
several blocks at once and returns a list, e.g.:

    image_block(path=["photo.jpg", "ninja.jpg"])
    -> [ {...block for photo.jpg...}, {...block for ninja.jpg...} ]

so it can be spliced straight into a content=[...] list -- llm_caller
flattens one level of nested lists in `content` before sending, so mixing
scalar and batch blocks in the same call just works:

    content=[
        text_block("Compare these"),
        image_block(path=["photo.jpg", "ninja.jpg"]),
    ]
"""

import base64
import mimetypes
from typing import List, Optional, Union

Scalar = Union[str, bytes]
OneOrMany = Union[Scalar, List[Scalar]]


def text_block(text: str) -> dict:
    return {"type": "text", "text": text}


def _as_list(value):
    if value is None:
        return None
    return value if isinstance(value, list) else [value]


def _one_block(
    block_type: str,
    path: Optional[str],
    data: Optional[bytes],
    media_type: Optional[str],
    url: Optional[str],
    default_media_type: Optional[str] = None,
) -> dict:
    sources = sum(x is not None for x in (path, data, url))
    if sources != 1:
        raise ValueError(f"{block_type} block needs exactly one of path, data, or url")

    if url is not None:
        return {"type": block_type, "url": url}

    if path is not None:
        guessed, _ = mimetypes.guess_type(path)
        # An explicit media_type always wins; otherwise prefer what
        # mimetypes actually detected from the file over a generic
        # per-block-type default (e.g. a .docx passed to document_block
        # must not get force-tagged as application/pdf).
        resolved_media_type = media_type or guessed or default_media_type
        if not resolved_media_type:
            raise ValueError(f"could not guess a media type for {path!r} -- pass media_type explicitly")
        with open(path, "rb") as f:
            data = f.read()
        media_type = resolved_media_type
    elif not media_type:
        media_type = default_media_type

    if not media_type:
        raise ValueError(f"{block_type} block with inline data needs media_type")

    return {
        "type": block_type,
        "media_type": media_type,
        "data": base64.standard_b64encode(data).decode("ascii"),
    }


def _build_blocks(block_type: str, *, path=None, data=None, media_type=None, url=None, default_media_type=None):
    is_batch = isinstance(path, list) or isinstance(data, list) or isinstance(url, list)

    paths = _as_list(path)
    datas = _as_list(data)
    urls = _as_list(url)
    media_types = media_type if isinstance(media_type, list) else None

    n = len(paths or datas or urls or [None])
    for name, seq in (("path", paths), ("data", datas), ("url", urls), ("media_type", media_types)):
        if seq is not None and len(seq) != n:
            raise ValueError(f"{block_type} block: {name} list length ({len(seq)}) doesn't match the others ({n})")

    blocks = [
        _one_block(
            block_type,
            paths[i] if paths else None,
            datas[i] if datas else None,
            media_types[i] if media_types else media_type,
            urls[i] if urls else None,
            default_media_type=default_media_type,
        )
        for i in range(n)
    ]
    return blocks if is_batch else blocks[0]


def image_block(path: OneOrMany = None, data: OneOrMany = None, media_type: OneOrMany = None, url: OneOrMany = None):
    """An image content block (or list of blocks, if any argument is a list)."""
    return _build_blocks("image", path=path, data=data, media_type=media_type, url=url)


def document_block(path: OneOrMany = None, data: OneOrMany = None, media_type: OneOrMany = None, url: OneOrMany = None):
    """A document content block (falls back to application/pdf only when
    mimetypes can't identify the file and no media_type was given)."""
    return _build_blocks("document", path=path, data=data, media_type=media_type, url=url, default_media_type="application/pdf")


def audio_block(path: OneOrMany = None, data: OneOrMany = None, media_type: OneOrMany = None, url: OneOrMany = None):
    return _build_blocks("audio", path=path, data=data, media_type=media_type, url=url)


def video_block(path: OneOrMany = None, data: OneOrMany = None, media_type: OneOrMany = None, url: OneOrMany = None):
    return _build_blocks("video", path=path, data=data, media_type=media_type, url=url)


def flatten_content(content: list) -> list:
    """Flattens one level of nesting -- lets a batch block builder's list
    output sit directly inside a content=[...] list alongside scalar
    blocks."""
    flat = []
    for item in content:
        if isinstance(item, list):
            flat.extend(item)
        else:
            flat.append(item)
    return flat
