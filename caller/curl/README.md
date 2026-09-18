# Calling Mesh with curl

Mesh's entire public API is plain HTTPS + newline-delimited JSON. The SDKs in this download
wrap exactly the contract below; curl needs no wrapper at all.

Throughout, replace `http://localhost:8080` with your Mesh server and `mesh_live_xxxxxxxx`
with your key. The key goes in the `Authorization` header on every endpoint, `GET` included —
there is no query-string form, because a key in a URL ends up in proxy logs, browser history
and `Referer` headers.

```bash
export MESH="http://localhost:8080"
export KEY="mesh_live_xxxxxxxx"
```

## 1. Chat — `POST /v1/proxy?model=<name>`

```bash
curl -N "$MESH/v1/proxy?model=claude-sonnet-5" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "Say hello in one sentence."}'
```

The response streams as one JSON object per line:

```
{"id":"req_01J...","delta":"Hello"}
{"id":"req_01J...","delta":" there!"}
{"id":"req_01J...","done":true,"usage":{"input_tokens":12,"output_tokens":34}}
```

Concatenate every `delta` in order to get the full reply. `-N` disables curl's output
buffering so you see each line as it arrives rather than all at once at the end.

The final `done` line may also carry:

| Field | Meaning |
|---|---|
| `usage` | The provider's own token counts. Either side is `0` if that provider has no usage path configured. |
| `truncated` | The reply hit `max_tokens` instead of finishing. Raise it and call again. |
| `incomplete` | The upstream connection dropped before the provider signalled completion. The text so far is valid, just partial. |

### Capping the reply

`max_tokens` is optional and defaults to **4096**. Pass it if replies cut off short — or to
keep a cheap call cheap:

```bash
curl -N "$MESH/v1/proxy?model=claude-sonnet-5" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "Write a detailed essay.", "max_tokens": 8192}'
```

## 2. Multimodal input

Send `content` instead of `query` — an ordered array of typed blocks. Set exactly one of
`query` or `content`, never both.

```json
{"type": "text",     "text": "..."}
{"type": "image",    "media_type": "image/png",      "data": "<base64>"}
{"type": "document", "media_type": "application/pdf", "data": "<base64>"}
{"type": "audio",    "media_type": "audio/mpeg",      "data": "<base64>"}
{"type": "video",    "media_type": "video/mp4",       "data": "<base64>"}
```

Every non-text block takes either `media_type` + `data` (inline base64) or `url`
(a remote file), not both. Per-block size limits: image 20 MB, document 50 MB, audio 75 MB,
video 200 MB.

```bash
IMG_B64=$(base64 -w0 photo.png)        # macOS: base64 -i photo.png

curl -N "$MESH/v1/proxy?model=claude-opus-5" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "content": [
      {"type": "text", "text": "What is in this image?"},
      {"type": "image", "media_type": "image/png", "data": "'"$IMG_B64"'"}
    ]
  }'
```

Check `formats_accepted` in `/v1/models` before sending a type — a model that doesn't accept
it is rejected with `content_type_not_supported_by_model` before Mesh forwards anything.

## 3. Web search

Set `"web_search": true` to let the model search the web and cite what it found. It has to be
a model your admin enabled it on — check `web_search` in `/v1/models` first.

```bash
curl -N "$MESH/v1/proxy?model=claude-opus-5" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"query": "What shipped in AI this week?", "web_search": true}'
```

Sources arrive on their own line as soon as the provider returns them — before the text that
cites them — so you can render "found 2 sources" while the answer is still streaming:

```
{"id":"req_01J...","sources":[{"url":"https://example.com/a","title":"Example A"}]}
{"id":"req_01J...","delta":"This week "}
{"id":"req_01J...","delta":"the big release was…"}
{"id":"req_01J...","done":true,"usage":{"input_tokens":6039,"output_tokens":931,"web_searches":2},"sources":[{"url":"https://example.com/a","title":"Example A","cited_text":"…"},{"url":"https://example.com/b","title":"Example B"}]}
```

The `done` line always repeats the **complete, deduplicated** source list, so a client that
ignores the mid-stream `sources` lines loses nothing — and one that reads both must
deduplicate by `url`. Fields per source: `url` (always), plus `title`, `cited_text` and
`page_age` where the provider supplies them.

The done line may also carry `queries` (the searches the model actually ran) and, for Gemini
providers, `search_suggestions_html` — a ready-made HTML fragment of Google Search chips that
Google's grounding terms require you to display alongside a grounded answer.

Asking for search on a model that doesn't support it is rejected outright rather than silently
answered without it:

```json
{"error":"web_search_not_supported_by_model"}
```

Searches are billed on top of tokens, against the same daily cap — `usage.web_searches` on the
done line tells you how many this call ran.

## 4. Image & video generation — `POST /v1/generate?model=<name>`

Only for models `/v1/models` reports as `"kind": "generation"`. One prompt in, one piece of
media out, as a single JSON response — not streamed. It is synchronous and can take minutes,
so give it a generous client-side timeout rather than assuming it's stuck.

```bash
curl "$MESH/v1/generate?model=gpt-image-2.5-flare" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "A watercolor painting of a lighthouse at dusk"}'
```

```json
{
  "id": "req_01J...",
  "model": "gpt-image-2.5-flare",
  "media_type": "image/png",
  "output": {"type": "base64", "data": "<base64-encoded image bytes>"},
  "outputs": [{"type": "base64", "data": "<base64-encoded image bytes>"}],
  "usage": {"input_tokens": 0, "output_tokens": 0}
}
```

`output` is either `{"type": "base64", "data": "..."}` (raw bytes) or
`{"type": "url", "data": "https://..."}` (a link the upstream provider is hosting, not Mesh).
`outputs` is the same thing as a list, for the rare model configured to return more than one.

**Mesh does not store the generated file anywhere** — it is in this response and nowhere else.
Decode or download it immediately:

```bash
# base64 response — decode straight to a file
curl -s "$MESH/v1/generate?model=gpt-image-2.5-flare" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "A watercolor painting of a lighthouse at dusk"}' \
  | jq -r '.output.data' | base64 -d > lighthouse.png

# url response — download it separately
RESULT=$(curl -s "$MESH/v1/generate?model=some-video-model" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Waves crashing on a rocky shore, slow motion"}')

if [ "$(echo "$RESULT" | jq -r '.output.type')" = "url" ]; then
  curl -s "$(echo "$RESULT" | jq -r '.output.data')" -o clip.mp4
else
  echo "$RESULT" | jq -r '.output.data' | base64 -d > clip.mp4
fi
```

`media_type` is the model's fixed output MIME type — use it to pick a file extension rather
than guessing.

## 5. Embeddings — `POST /v1/embeddings?model=<name>`

Only for models `/v1/models` reports as `"kind": "embedding"`. One text input per call.

```bash
curl "$MESH/v1/embeddings?model=text-embedding-3-small" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"input": "The quick brown fox"}'
```

```json
{"id":"req_01J...","model":"text-embedding-3-small","embedding":[0.0123,-0.045],"usage":{"input_tokens":5}}
```

## 6. What you have access to — `GET /v1/models`

```bash
curl "$MESH/v1/models" -H "Authorization: Bearer $KEY"
```

```json
{
  "models": [
    {
      "model": "claude-sonnet-5",
      "kind": "chat",
      "formats_accepted": ["text", "image", "document"],
      "web_search": true,
      "status": "available",
      "usage": {
        "calls_this_hour": 12,
        "calls_per_hour_limit": 100,
        "spent_today_usd": 1.42,
        "daily_cap_usd": 20
      },
      "allowed_hours": {"from": "09:00", "to": "18:00", "timezone": "America/New_York", "days": [1,2,3,4,5]}
    }
  ]
}
```

- `kind` — `chat` (`/v1/proxy`), `embedding` (`/v1/embeddings`) or `generation` (`/v1/generate`).
- `status` — `available`, `rate_limited`, `budget_exceeded` or `outside_hours`. Anything but
  `available` also carries `resume_at`.
- `web_search` — whether `"web_search": true` will be accepted.
- Models an admin blocked or retired never appear here at all.

## 7. Spend — `GET /v1/usage`

```bash
curl "$MESH/v1/usage" -H "Authorization: Bearer $KEY"
```

```json
{"usage":[{"model":"claude-sonnet-5","kind":"chat","spent_usd":1.42,"daily_cap_usd":20,"remaining_usd":18.58}]}
```

`daily_cap_usd` and `remaining_usd` are `null` when no cap is set for that model.

## 8. Errors

Every non-2xx response is a structured JSON error, not a bare status code — so you always know
*why* a call was rejected, not just that it was. The shape is
`{"error": "<code>", ...extra fields}`.

```bash
# a model you were never granted
curl "$MESH/v1/proxy?model=some-model-you-lack" \
  -H "Authorization: Bearer $KEY" -H "Content-Type: application/json" \
  -d '{"query": "test"}'
```
```json
{"error":"model_not_permitted"}
```

| Code | Meaning |
|---|---|
| `invalid_or_revoked_key` | Your key is missing, wrong, revoked, or your account/key has expired. |
| `model_not_permitted` | You were never granted access to this model. |
| `model_blocked` | An admin has disabled this model globally. |
| `provider_hourly_limit_reached` | That provider's total call quota for this hour is used up. Includes `resume_at`. |
| `user_model_hourly_limit_reached` | Your own per-model hourly call cap is used up. Includes `resume_at` and `limit`. |
| `outside_allowed_hours` | Your schedule doesn't permit calls right now. Includes `resume_at`. |
| `budget_exceeded` | You've hit your daily spending cap for this model. Includes `cap` and `resume_at`. |
| `paused_by_admin` | An admin has paused the whole gateway. |
| `content_type_not_supported_by_model` | You sent an image/document/audio/video block to a model that doesn't accept that type. Includes `content_type`. |
| `web_search_not_supported_by_model` | `"web_search": true` on a model it isn't enabled for. |
| `embeddings_not_supported_by_provider` | The model you called `/v1/embeddings` with isn't configured for embeddings. |
| `generation_not_supported_by_provider` | The model you called `/v1/generate` with isn't configured for generation. |
| `invalid_request` | Malformed body — both `query` and `content` set, neither set, `max_tokens` ≤ 0, an oversized block, or a block missing required fields. |
| `server_busy` | Mesh is at capacity. Back off a second or two with jitter and retry. |
| `provider_busy` | That provider's concurrency limit is saturated. Same handling. |
| `upstream_unavailable` | The provider itself failed. Includes `provider_status` and `provider_message`. |

Mesh rejects fast rather than queueing you silently, so `server_busy` is a normal part of
operation under load — treat it exactly like a 429. Where the body carries `resume_at`, use it
instead of guessing a backoff.
