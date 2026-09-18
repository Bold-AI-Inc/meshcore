# Calling Mesh

Four ways to call the same gateway. Pick one — they all speak the identical HTTP contract.

| Folder | Use it when |
|---|---|
| [`simple-python/`](simple-python) | Python, want one file you can drop in. Needs only `requests`. |
| [`simple-js/`](simple-js) | Node 18+, want one file you can drop in. Zero dependencies. |
| [`bold-mesh-python/`](bold-mesh-python) | Python, want typed results, content-block builders and a proper package. |
| [`bold-mesh-typescript/`](bold-mesh-typescript) | TypeScript/Node, same, with full type definitions. |
| [`curl/`](curl) | Any other language — the raw wire contract, endpoint by endpoint. |

## 60-second start

1. Put your key in a `.env` file (copy [`.env.example`](.env.example)) or export it:

   ```bash
   export MESH_API_KEY=mesh_live_xxxxxxxx
   export MESH_SERVER_PATH=https://your-mesh-server.example.com
   ```

2. Run the example for your language:

   ```bash
   pip install requests && python simple-python/example.py
   node simple-js/example.js
   ```

   The first thing each example prints is every model your key can actually call. Use those
   names — asking for one you weren't granted returns `model_not_permitted`.

## The five endpoints

| Endpoint | Does |
|---|---|
| `POST /v1/proxy?model=` | Chat. Streams NDJSON. Text, images, documents, audio, video, optional web search. |
| `POST /v1/generate?model=` | Image/video generation. One JSON response. |
| `POST /v1/embeddings?model=` | One text in, one vector out. |
| `GET /v1/models` | What you can call, what it accepts, whether it's available right now. |
| `GET /v1/usage` | Today's spend and remaining budget per model. |

Full reference with request/response shapes for each: [`curl/README.md`](curl/README.md).

## Things worth knowing before you start

- **The key goes in the `Authorization` header**, never a query string, on every endpoint.
- **`/v1/proxy` streams.** One JSON object per line; concatenate the `delta` fields. The final
  line has `done: true` plus `usage`.
- **`max_tokens` defaults to 4096.** If a reply comes back with `truncated`, it hit the
  ceiling rather than finishing.
- **Generated media is never stored by Mesh.** `/v1/generate` hands you base64 or a provider
  URL — save it in that moment or it's gone.
- **Every failure is JSON**, shaped `{"error": "<code>", ...}`. `server_busy` is normal under
  load: back off and retry. Where a `resume_at` is present, use it instead of guessing.
