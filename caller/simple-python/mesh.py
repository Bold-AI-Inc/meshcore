"""
Mesh in one file. Copy mesh.py into your project and `from mesh import llm_caller`.

The only dependency is `requests` (pip install requests). Nothing to install
from PyPI, no package, no build step. If you want the fuller client with typed
results and content-block builders, use caller/bold-mesh-python instead.

Set two environment variables first (or pass api_key= / server_path= per call):

    export MESH_API_KEY=mesh_live_xxxxxxxx        # macOS/Linux
    export MESH_SERVER_PATH=https://your-mesh-server.example.com

    set MESH_API_KEY=mesh_live_xxxxxxxx           # Windows

Quick tour:

    from mesh import llm_caller, llm_generate, llm_embeddings, models, usage

    for chunk in llm_caller("claude-sonnet-5", "hello"):        # streams
        print(chunk, end="", flush=True)

    r = llm_caller("claude-sonnet-5", "hello", streaming=False) # blocks
    print(r["text"], r["usage"])
"""

import base64
import json
import mimetypes
import os

import requests

DEFAULT_SERVER_PATH = "http://localhost:8080"

#: What the gateway applies when you don't pass max_tokens.
DEFAULT_MAX_TOKENS = 4096


class MeshError(Exception):
    """Raised for any non-2xx response from the Mesh gateway.

    `code` is the machine-readable error code (e.g. "invalid_or_revoked_key",
    "budget_exceeded", "server_busy"); `body` is the full parsed JSON error,
    which for some codes carries extra fields such as `resume_at` or `cap`.
    """

    def __init__(self, code, body, status_code=0):
        super().__init__(code)
        self.code = code
        self.body = body or {}
        self.status_code = status_code

    @property
    def resume_at(self):
        return self.body.get("resume_at")


def _resolve(api_key, server_path):
    api_key = api_key or os.environ.get("MESH_API_KEY")
    if not api_key:
        raise ValueError("MESH_API_KEY is not set (env var or api_key= argument)")
    server_path = (server_path or os.environ.get("MESH_SERVER_PATH") or DEFAULT_SERVER_PATH).rstrip("/")
    return api_key, server_path


def _headers(api_key):
    return {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}


def _check(resp):
    """Mesh answers every failure with {"error": "<code>", ...}. Turn that into
    a MeshError; turn anything else (an HTML 502 from a proxy, say) into one too."""
    if resp.status_code == 200:
        return
    try:
        body = resp.json() if resp.content else {}
    except ValueError:
        body = {}
    if not isinstance(body, dict):
        body = {}
    raise MeshError(body.get("error", "unknown_error"), body, resp.status_code)


def _file_block(block_type, path):
    """Reads a local file into a base64 content block, guessing its MIME type
    from the extension."""
    media_type = mimetypes.guess_type(path)[0]
    if not media_type:
        raise ValueError(f"couldn't guess the media type of {path} -- rename it or build the block by hand")
    with open(path, "rb") as f:
        data = base64.standard_b64encode(f.read()).decode("ascii")
    return {"type": block_type, "media_type": media_type, "data": data}


def _merge_sources(existing, incoming):
    """Merges by url, in first-seen order. Mesh streams newly-discovered sources
    as it finds them and then repeats the complete set on the final line, so a
    naive += would duplicate every one. The repeat is often richer than the
    mid-stream version (it carries cited_text), so a source we already have is
    topped up field by field rather than dropped."""
    merged = list(existing)
    index = {s.get("url"): i for i, s in enumerate(merged)}
    for source in incoming:
        position = index.get(source.get("url"))
        if position is None:
            index[source.get("url")] = len(merged)
            merged.append(dict(source))
        else:
            merged[position] = {**source, **merged[position]}
    return merged


def llm_caller(model, prompt, streaming=True, max_tokens=None, web_search=False,
               images=None, documents=None, audio=None, video=None,
               api_key=None, server_path=None, timeout=120):
    """Calls a chat model over POST /v1/proxy.

    streaming=True (the default) returns a _TextStream: iterate it and you get
    each text delta as it arrives, exactly as the provider sent it.

        for chunk in llm_caller("claude-sonnet-5", "hello"):
            print(chunk, end="", flush=True)

    streaming=False blocks until the reply is complete and returns a dict:

        {"text": "...",
         "usage": {"input_tokens": N, "output_tokens": N, ...},
         "sources": [...],          # web_search only
         "queries": [...],          # web_search only
         "truncated": bool,         # hit max_tokens instead of finishing
         "incomplete": bool}        # connection dropped mid-reply

    images / documents / audio / video take lists of local file paths to attach
    alongside the prompt:

        llm_caller("claude-opus-5", "what's in this?", images=["cat.png"])

    Whether a model accepts a given type is in models()[i]["formats_accepted"];
    sending one it doesn't gives a clean content_type_not_supported_by_model.

    max_tokens caps the reply (gateway default 4096). A reply that comes back
    with truncated=True hit that ceiling rather than finishing.

    web_search=True lets the model search the web and cite what it found. Only
    models with "web_search": true in models() accept it -- anywhere else you
    get MeshError("web_search_not_supported_by_model"). Searches bill on top of
    tokens against the same daily cap.
    """
    resolved_key, resolved_path = _resolve(api_key, server_path)

    if max_tokens is not None and max_tokens <= 0:
        raise ValueError("max_tokens must be a positive integer")

    attachments = [
        ("image", images or []),
        ("document", documents or []),
        ("audio", audio or []),
        ("video", video or []),
    ]
    if any(paths for _, paths in attachments):
        content = [{"type": "text", "text": prompt}]
        for block_type, paths in attachments:
            content += [_file_block(block_type, p) for p in paths]
        body = {"content": content}
    else:
        body = {"query": prompt}

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
    _check(resp)

    stream = _TextStream(resp)
    if streaming:
        return stream

    text = "".join(stream)
    return {
        "text": text,
        "usage": stream.usage,
        "sources": stream.sources,
        "queries": stream.queries,
        "search_suggestions_html": stream.search_suggestions_html,
        "truncated": stream.truncated,
        "incomplete": stream.incomplete,
    }


class _TextStream:
    """Iterating yields each text delta as a plain string. Web-search results
    land on .sources / .queries as they arrive, and token counts on .usage;
    all three are complete once iteration finishes."""

    def __init__(self, resp):
        self._resp = resp
        self._done = False
        self.sources = []
        self.queries = []
        self.usage = {"input_tokens": 0, "output_tokens": 0}
        self.search_suggestions_html = None
        self.truncated = False
        self.incomplete = False

    def __iter__(self):
        try:
            yield from self._deltas()
        finally:
            # The gateway always sends a "done" line; not reaching one means the
            # connection dropped. Say so rather than pretending the reply ended.
            if not self._done:
                self.incomplete = True
            self._resp.close()

    def _deltas(self):
        try:
            for line in self._resp.iter_lines(decode_unicode=True):
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                except ValueError:
                    continue  # a truncated line can only mean a dropped connection

                if chunk.get("sources"):
                    self.sources = _merge_sources(self.sources, chunk["sources"])

                if chunk.get("done"):
                    self._done = True
                    self.usage = chunk.get("usage", self.usage)
                    self.queries = chunk.get("queries", self.queries)
                    self.search_suggestions_html = chunk.get("search_suggestions_html")
                    self.truncated = bool(chunk.get("truncated"))
                    self.incomplete = bool(chunk.get("incomplete"))
                    return

                delta = chunk.get("delta")
                if delta:
                    yield delta
        except requests.exceptions.RequestException:
            # The response was already a 200 and text may have arrived, so a
            # transport failure here means a truncated reply, not a failed call.
            # End the stream and let .incomplete carry the news rather than
            # raising a requests-specific exception at the caller.
            return


def llm_generate(model, prompt, api_key=None, server_path=None, timeout=900):
    """Calls POST /v1/generate for image/video generation models (the ones
    models() reports as "kind": "generation"). Returns the response dict:

        {"id", "model", "media_type", "output": {"type", "data"},
         "outputs": [...], "usage": {...}}

    output["type"] is "base64" (raw bytes) or "url" (hosted by the provider,
    not by Mesh). Use save_output() to write it to disk. Mesh does not store
    the generated file anywhere -- it is in this response and nowhere else.

    Generation is synchronous and can take minutes, hence the long timeout.
    """
    resolved_key, resolved_path = _resolve(api_key, server_path)
    resp = requests.post(
        f"{resolved_path}/v1/generate",
        params={"model": model},
        headers=_headers(resolved_key),
        json={"prompt": prompt},
        timeout=timeout,
    )
    _check(resp)
    return resp.json()


def save_output(output, path, timeout=300):
    """Writes one /v1/generate output ({"type", "data"}) to a local file."""
    if output["type"] == "base64":
        with open(path, "wb") as f:
            f.write(base64.standard_b64decode(output["data"]))
    elif output["type"] == "url":
        resp = requests.get(output["data"], timeout=timeout)
        resp.raise_for_status()
        with open(path, "wb") as f:
            f.write(resp.content)
    else:
        raise ValueError(f"unknown output type {output['type']!r}")


def llm_embeddings(model, input, api_key=None, server_path=None, timeout=120):
    """Calls POST /v1/embeddings. One text input per call -- the gateway does
    not take batch input. Returns {"id", "model", "embedding", "usage"}."""
    resolved_key, resolved_path = _resolve(api_key, server_path)
    resp = requests.post(
        f"{resolved_path}/v1/embeddings",
        params={"model": model},
        headers=_headers(resolved_key),
        json={"input": input},
        timeout=timeout,
    )
    _check(resp)
    return resp.json()


def models(api_key=None, server_path=None, timeout=120):
    """Calls GET /v1/models. Returns a list of dicts: model, kind
    ("chat"/"embedding"/"generation"), formats_accepted, web_search, status,
    usage, and resume_at/allowed_hours where relevant."""
    resolved_key, resolved_path = _resolve(api_key, server_path)
    resp = requests.get(f"{resolved_path}/v1/models", headers=_headers(resolved_key), timeout=timeout)
    _check(resp)
    return resp.json().get("models", [])


def usage(api_key=None, server_path=None, timeout=120):
    """Calls GET /v1/usage. Returns a list of dicts: model, kind, spent_usd,
    daily_cap_usd, remaining_usd (the last two are None with no cap set)."""
    resolved_key, resolved_path = _resolve(api_key, server_path)
    resp = requests.get(f"{resolved_path}/v1/usage", headers=_headers(resolved_key), timeout=timeout)
    _check(resp)
    return resp.json().get("usage", [])
