# testserve

A dummy **upstream provider** for Mesh. It impersonates Anthropic, OpenAI, Gemini and
Ollama on the wire so every Mesh code path — template rendering, provider-family content
blocks, SSE delta/usage dot-paths, retry/backoff, embeddings, image/video generation,
admin verify-on-save — runs for real without touching a commercial API.

```
caller/ SDK  ->  mesh :8080  ->  testserve :9000
```

## Run

```bash
pip install -r testserve/requirements.txt
uvicorn testserve.main:app --port 9000              # from the repo root
uvicorn testserve.main:app --port 9000 --workers 4  # more headroom
```

`GET /` lists every route, `GET /healthz` is a liveness probe, `/_docs` is the OpenAPI UI.

## Endpoints

| Route | Purpose | Mesh dot-path |
|---|---|---|
| `POST /anthropic/v1/messages` | chat SSE, Anthropic shape | `delta.text`, `message.usage.input_tokens`, `usage.output_tokens` |
| `POST /openai/v1/chat/completions` | chat SSE, OpenAI shape, `[DONE]` | `choices.0.delta.content`, `usage.prompt_tokens`, `usage.completion_tokens` |
| `POST /gemini/v1beta/models/{model}:streamGenerateContent` | chat SSE, cumulative usage | `candidates.0.content.parts.0.text`, `usageMetadata.promptTokenCount`, `usageMetadata.candidatesTokenCount` |
| `POST /ollama/v1/chat/completions` | OpenAI shape, text+image only | `choices.0.delta.content` |
| `POST /openai/v1/embeddings` | embedding vector | `data.0.embedding` |
| `POST /gemini/v1beta/models/{model}:embedContent` | embedding vector | `embedding.values` |
| `POST /openai/v1/images/generations` | image, base64 | `data.0.b64_json` |
| `POST /openai/v1/images/urls` | image, URL | `data.0.url` |
| `POST /openai/v1/images/batch` | several images | `outputs` |
| `POST /v1/videos/generations` | video, URL | `video.url` |
| `POST /v1/videos/inline` | video, base64 | `video.b64` |
| `POST /gemini/v1beta/models/{model}:predict` | image or video, base64 | `predictions.0.bytesBase64Encoded` |
| `GET /files/{name}` | serves the generated media | — |

Chat routes also answer non-streaming (`"stream": false`) with the provider's normal
single-JSON shape. Gemini additionally exposes `:generateContent`.

Replies are derived from what actually arrived — `testserve[anthropic] ack prompt<...>
image(image/png,70B,c414cd0e204de974)` — so a multimodal round-trip is verifiable, not
just non-crashing. Embedding vectors are a deterministic function of the input text, so
the same input always yields the same vector.

## Scenarios

The behaviour is picked from the **resolved model id's suffix**, so you trigger each one
by registering another model in Mesh against the same provider — no template edits.

| Suffix | Behaviour |
|---|---|
| `-ok` (or none) | normal response |
| `-slow` | sleeps `TESTSERVE_SLOW_DELAY_MS` before responding |
| `-hang` | never responds (exercises Mesh's idle/generation timeouts) |
| `-error-400/429/500` | provider error in `{"error":{"message":...}}` shape; 429 sends `Retry-After` |
| `-flaky` | fails the first `TESTSERVE_FLAKY_FAILURES` calls, then succeeds — proves retry/backoff |
| `-conn-reset` | drops the connection mid-stream after 2 deltas |
| `-badjson` | emits unparseable SSE payloads |
| `-nousage` | omits the usage fields entirely |
| `-empty` | zero deltas |
| `-unicode` | emoji / CJK / accented deltas |
| `-huge` | ~200x the normal delta count |
| `-keepalive` | interleaves blank `data:` lines |
| `-customdone` | ends with `END_OF_STREAM` (set that as the provider's done signal) |

Override globally without touching model ids:

```bash
curl -X POST localhost:9000/_debug/scenario -H 'content-type: application/json' -d '{"scenario":"error-429"}'
curl -X POST localhost:9000/_debug/scenario -H 'content-type: application/json' -d '{"scenario":""}'   # clear
```

## Inspection

Every request is recorded with its headers, the fully rendered body, and two assertions
you cannot otherwise make from outside Mesh:

- `key_leaked_in_body` — the provider key appeared in the request **body** (Mesh promises it only ever goes in headers/URL)
- `placeholders_left` — a `{{...}}` token survived template rendering

```bash
curl localhost:9000/_debug/last
curl 'localhost:9000/_debug/requests?limit=5&path=anthropic&model=dummy-chat'
curl localhost:9000/_debug/stats        # total, in_flight, peak_in_flight, rps
curl -X DELETE localhost:9000/_debug/requests
```

`peak_in_flight` is how you verify Mesh honours `MaxConcurrentUpstream` / `MaxOutboundRPS` —
set `TESTSERVE_DELTA_DELAY_MS` so streams stay open long enough to overlap.

## Config

| Env var | Default | Meaning |
|---|---|---|
| `TESTSERVE_PORT` | `9000` | port (also used to build `/files` URLs) |
| `TESTSERVE_BASE_URL` | `http://localhost:9000` | host used in returned media URLs |
| `TESTSERVE_API_KEY` | `testserve-key` | expected key |
| `TESTSERVE_REQUIRE_KEY` | `0` | `1` rejects a wrong/missing key with 401 |
| `TESTSERVE_DELTA_DELAY_MS` | `0` | pause between SSE deltas |
| `TESTSERVE_SLOW_DELAY_MS` | `5000` | delay for `-slow` |
| `TESTSERVE_HANG_SECONDS` | `3600` | delay for `-hang` |
| `TESTSERVE_CHAOS_RATE` | `0` | fraction of all calls that fail with 500 |
| `TESTSERVE_FLAKY_FAILURES` | `2` | failures before `-flaky` succeeds |
| `TESTSERVE_EMBEDDING_DIM` | `16` | embedding vector length |
| `TESTSERVE_LOG_LIMIT` | `500` | requests kept in the debug log |
| `TESTSERVE_FORCE_SCENARIO` | — | force one scenario at startup |

## Wiring it into Mesh

`seed/providers.json` has ready-to-paste provider and model config for all six providers
(endpoint, header template, body template, dot-paths, provider family, model kinds). Note
that `{{content_json}}`, `{{max_tokens_json}}`, `{{input_json}}` and `{{prompt_json}}` must
be written **quoted** in the templates — Mesh strips the quotes when it splices the raw
JSON in.

Then verify the whole thing end to end:

```bash
python3 testserve/selftest.py
```

`selftest.py` renders those templates exactly the way Mesh does and resolves the exact
dot-paths against live responses, so a broken provider config is caught before you paste
it into the admin UI. Stdlib only, no dependencies.

## Media

`files/` is generated on first start: a 1x1 PNG, a WAV, a minimal PDF, and a
structurally-framed MP4 container (parseable, not a playable clip). Drop your own file
with the same name and it's used as-is.
