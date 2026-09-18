# bold-mesh (Python)

A thin Python client for the Mesh AI gateway. One function per endpoint, no retries, no
caching, no magic.

## Install

From this folder (it is not on PyPI — install it from the copy you downloaded):

```bash
pip install ./caller/bold-mesh-python
```

Then put your key in a `.env` file next to your script:

```
MESH_API_KEY=mesh_live_xxxxxxxx
MESH_SERVER_PATH=https://your-mesh-server.example.com
```

## Chat

```python
from bold.mesh import llm_caller

# streaming (the default)
with llm_caller(model="claude-sonnet-5", prompt="Say hello in one sentence.") as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)
    final = stream.get_final_response()
    print()
    print(final.usage.input_tokens, final.usage.output_tokens)

# non-streaming -- blocks, returns the full response
resp = llm_caller(model="claude-sonnet-5", prompt="Say hello in one sentence.", streaming=False)
print(resp.text)
```

`max_tokens=` caps the reply (gateway default: 4096). If `resp.truncated` is True the reply hit
that ceiling rather than finishing — raise `max_tokens` and call again.

## Multimodal input

```python
from bold.mesh import llm_caller, text_block, image_block, document_block

resp = llm_caller(model="claude-opus-5", content=[
    text_block("Compare these two photos"),
    image_block(path=["photo.jpg", "ninja.jpg"]),   # a list builds several blocks at once
    document_block(path="report.pdf"),
], streaming=False)
```

`image_block` / `document_block` / `audio_block` / `video_block` each accept `path=` (local
file, read + base64-encoded for you), `data=` (raw bytes), or `url=` — any of which can be a
single value or a list. Which types a given model accepts is in
`list_models()[i].formats_accepted`.

## Web search

```python
resp = llm_caller(model="claude-opus-5", prompt="What shipped in AI this week?",
                  web_search=True, streaming=False)
print(resp.text)
for s in resp.sources:
    print(s.title, s.url)
```

Only models with `web_search=True` in `list_models()` accept it; anywhere else it raises
`MeshError("web_search_not_supported_by_model")`. Searches bill on top of tokens.

## Generation and embeddings

```python
from bold.mesh import llm_generate, llm_embeddings

gen = llm_generate(model="gpt-image-2.5-flare", prompt="a watercolor lighthouse at dusk")
gen.save("lighthouse.png")     # Mesh never stores it — save it now or lose it

emb = llm_embeddings(model="text-embedding-3-small", input="hello world")
print(len(emb.embedding))
```

## Discovery and spend

```python
from bold.mesh import list_models, get_usage

for m in list_models():
    print(m.model, m.kind, m.status, m.formats_accepted, m.web_search)

for u in get_usage():
    print(u.model, u.spent_usd, u.remaining_usd)
```

## Errors

Every non-2xx response raises `MeshError` with a machine-readable `.code` (`budget_exceeded`,
`model_not_permitted`, `server_busy`, …), the full `.body`, and `.resume_at` where the gateway
supplied one.

```python
from bold.mesh import MeshError

try:
    llm_caller(model="claude-opus-5", prompt="hi", streaming=False)
except MeshError as e:
    print(e.code, e.resume_at, e.body)
```

## Configuration

`MESH_API_KEY` and `MESH_SERVER_PATH` come from the process environment, or from a `.env` file
in the working directory (real env vars always win). Both can be overridden per call with
`api_key=` / `server_path=`. Point `MESH_DOTENV_PATH` elsewhere to load a different file.

See `example.py` in this folder for a runnable tour of every endpoint.
