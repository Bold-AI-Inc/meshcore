import os
from typing import List, Optional

import requests

from ._env import load_dotenv_once
from .blocks import flatten_content
from .errors import MeshConfigError, MeshError
from .stream import MeshStream
from .types import EmbeddingResult, GenerateResult, MeshResponse, ModelInfo, UsageInfo

DEFAULT_SERVER_PATH = "http://localhost:8080"

#: Applied when you don't pass max_tokens. Mirrors the gateway's own
#: default (server/internal/gateway/contract.go: defaultMaxTokens).
DEFAULT_MAX_TOKENS = 4096

#: Generation can legitimately run for minutes; chat replies stream, so a
#: read timeout only has to cover the gap between chunks.
DEFAULT_TIMEOUT = 120
DEFAULT_GENERATE_TIMEOUT = 900


def _resolve_config(api_key: Optional[str], server_path: Optional[str]):
    load_dotenv_once()

    resolved_key = api_key or os.environ.get("MESH_API_KEY")
    if not resolved_key:
        raise MeshConfigError(
            "MESH_API_KEY is not set. Set it in your environment, in a .env file, "
            "or pass api_key= explicitly to this call."
        )

    resolved_path = (server_path or os.environ.get("MESH_SERVER_PATH") or DEFAULT_SERVER_PATH).rstrip("/")
    return resolved_key, resolved_path


def _headers(api_key: str) -> dict:
    return {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}


def _raise_for_error(resp: requests.Response) -> None:
    """Every non-2xx from Mesh is a JSON body of the shape
    {"error": "<code>", ...extra}. Anything else (a proxy's HTML 502, say)
    still turns into a MeshError rather than a decode traceback."""
    try:
        body = resp.json() if resp.content else {}
    except ValueError:
        body = {}
    if not isinstance(body, dict):
        body = {}
    raise MeshError(body.get("error", "unknown_error"), body, resp.status_code)


def llm_caller(
    model: str,
    prompt: Optional[str] = None,
    content: Optional[list] = None,
    streaming: bool = True,
    max_tokens: Optional[int] = None,
    web_search: bool = False,
    api_key: Optional[str] = None,
    server_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_TIMEOUT,
):
    """Calls a chat model over Mesh's POST /v1/proxy.

    Pass exactly one of prompt= (plain text) or content=[...] (built from
    text_block/image_block/document_block/audio_block/video_block, any of
    which can take a list to send several files at once).

    streaming=True (the default) returns a MeshStream -- use it as a
    context manager and iterate .text_stream, then .get_final_response().

    streaming=False blocks until the full response arrives and returns a
    MeshResponse directly (.text, .usage.input_tokens, .usage.output_tokens).

    max_tokens caps the reply length; omitting it uses the gateway default
    of 4096. If a reply comes back with .truncated set, it hit the ceiling
    rather than finishing -- raise max_tokens and call again.

    web_search=True lets the model search the web and cite what it found.
    It only works on models your admin enabled it for (check .web_search in
    list_models()); anywhere else it raises
    MeshError("web_search_not_supported_by_model"). Searches are billed on
    top of tokens against the same daily cap.
    """
    resolved_key, resolved_path = _resolve_config(api_key, server_path)

    if (prompt is None) == (content is None):
        raise ValueError("pass exactly one of prompt= or content=")
    if max_tokens is not None and max_tokens <= 0:
        raise ValueError("max_tokens must be a positive integer")

    body = {"query": prompt} if content is None else {"content": flatten_content(content)}
    if max_tokens is not None:
        body["max_tokens"] = max_tokens
    if web_search:
        body["web_search"] = True

    resp = requests.post(
        f"{resolved_path}/v1/proxy",
        params={"model": model},
        headers=_headers(resolved_key),
        json=body,
        stream=True,
        timeout=timeout,
    )
    if resp.status_code != 200:
        _raise_for_error(resp)

    stream = MeshStream(resp, model=model)
    if streaming:
        return stream
    with stream:
        return stream.get_final_response()


def llm_generate(
    model: str,
    prompt: str,
    api_key: Optional[str] = None,
    server_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_GENERATE_TIMEOUT,
) -> GenerateResult:
    """Calls POST /v1/generate for image/video generation models (the ones
    list_models() reports as kind="generation"). Returns a GenerateResult --
    result.save("out.png") writes the first generated item to disk, decoding
    base64 or downloading the url as needed.

    Generation is synchronous and can take minutes, hence the long default
    timeout.
    """
    resolved_key, resolved_path = _resolve_config(api_key, server_path)

    resp = requests.post(
        f"{resolved_path}/v1/generate",
        params={"model": model},
        headers=_headers(resolved_key),
        json={"prompt": prompt},
        timeout=timeout,
    )
    if resp.status_code != 200:
        _raise_for_error(resp)
    return GenerateResult._from_dict(resp.json())


def llm_embeddings(
    model: str,
    input: str,
    api_key: Optional[str] = None,
    server_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_TIMEOUT,
) -> EmbeddingResult:
    """Calls POST /v1/embeddings. One text input per call (the gateway does
    not take batch input) -- call this once per input."""
    resolved_key, resolved_path = _resolve_config(api_key, server_path)

    resp = requests.post(
        f"{resolved_path}/v1/embeddings",
        params={"model": model},
        headers=_headers(resolved_key),
        json={"input": input},
        timeout=timeout,
    )
    if resp.status_code != 200:
        _raise_for_error(resp)
    return EmbeddingResult._from_dict(resp.json())


def list_models(
    api_key: Optional[str] = None,
    server_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_TIMEOUT,
) -> List[ModelInfo]:
    """Calls GET /v1/models -- every model you have access to, what content
    types it accepts, whether it can web-search, and whether you can call it
    right now. Blocked and retired models never appear."""
    resolved_key, resolved_path = _resolve_config(api_key, server_path)
    resp = requests.get(f"{resolved_path}/v1/models", headers=_headers(resolved_key), timeout=timeout)
    if resp.status_code != 200:
        _raise_for_error(resp)
    return [ModelInfo._from_dict(m) for m in resp.json().get("models", [])]


def get_usage(
    api_key: Optional[str] = None,
    server_path: Optional[str] = None,
    timeout: Optional[float] = DEFAULT_TIMEOUT,
) -> List[UsageInfo]:
    """Calls GET /v1/usage -- today's spend and remaining daily budget per
    model (cumulative, not per-request; for per-call token counts read
    .usage off llm_caller's MeshResponse/MeshStream instead)."""
    resolved_key, resolved_path = _resolve_config(api_key, server_path)
    resp = requests.get(f"{resolved_path}/v1/usage", headers=_headers(resolved_key), timeout=timeout)
    if resp.status_code != 200:
        _raise_for_error(resp)
    return [UsageInfo._from_dict(u) for u in resp.json().get("usage", [])]


__all__ = [
    "llm_caller",
    "llm_generate",
    "llm_embeddings",
    "list_models",
    "get_usage",
    "MeshResponse",
]
