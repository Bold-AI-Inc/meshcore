# bold-mesh (TypeScript / Node)

A thin TypeScript client for the Mesh AI gateway. One function per endpoint, no retries, no
caching, no magic. Node 18+ (uses the built-in `fetch`).

## Install

From this folder (it is not on npm — install it from the copy you downloaded):

```bash
npm install ./caller/bold-mesh-typescript
```

Then put your key in a `.env` file next to your entrypoint:

```
MESH_API_KEY=mesh_live_xxxxxxxx
MESH_SERVER_PATH=https://your-mesh-server.example.com
```

## Chat

```ts
import { llmCaller } from "bold-mesh";

// streaming (the default)
const stream = llmCaller({ model: "claude-sonnet-5", prompt: "Say hello in one sentence." });
for await (const text of stream.textStream) process.stdout.write(text);
const final = await stream.finalResponse();
console.log(final.usage.inputTokens, final.usage.outputTokens);

// non-streaming -- resolves with the full response
const resp = await llmCaller({
  model: "claude-sonnet-5",
  prompt: "Say hello in one sentence.",
  streaming: false,
});
console.log(resp.text);
```

`maxTokens` caps the reply (gateway default: 4096). If `resp.truncated` is true the reply hit
that ceiling rather than finishing — raise `maxTokens` and call again.

## Multimodal input

```ts
import { llmCaller, textBlock, imageBlock, documentBlock } from "bold-mesh";

const resp = await llmCaller({
  model: "claude-opus-5",
  content: [
    textBlock("Compare these two photos"),
    await imageBlock({ path: ["photo.jpg", "ninja.jpg"] }), // an array builds several blocks
    await documentBlock({ path: "report.pdf" }),
  ],
  streaming: false,
});
```

`imageBlock` / `documentBlock` / `audioBlock` / `videoBlock` each accept `path` (local file,
read + base64-encoded for you), `data` (a `Uint8Array`), or `url` — any of which can be a
single value or an array. They are async, so `await` them. Which types a given model accepts
is in `listModels()[i].formats_accepted`.

## Web search

```ts
const resp = await llmCaller({
  model: "claude-opus-5",
  prompt: "What shipped in AI this week?",
  webSearch: true,
  streaming: false,
});
console.log(resp.text);
for (const s of resp.sources) console.log(s.title, s.url);
```

Only models with `web_search: true` in `listModels()` accept it; anywhere else it throws
`MeshError("web_search_not_supported_by_model")`. Searches bill on top of tokens.

## Generation and embeddings

```ts
import { llmGenerate, llmEmbeddings, saveMediaOutput } from "bold-mesh";

const gen = await llmGenerate({
  model: "gpt-image-2.5-flare",
  prompt: "a watercolor lighthouse at dusk",
});
await saveMediaOutput(gen.output, "lighthouse.png"); // Mesh never stores it — save it now

const emb = await llmEmbeddings({ model: "text-embedding-3-small", input: "hello world" });
console.log(emb.embedding.length);
```

## Discovery and spend

```ts
import { listModels, getUsage } from "bold-mesh";

for (const m of await listModels()) console.log(m.model, m.kind, m.status, m.web_search);
for (const u of await getUsage()) console.log(u.model, u.spent_usd, u.remaining_usd);
```

## Errors

Every non-2xx response throws `MeshError` with a machine-readable `.code`
(`budget_exceeded`, `model_not_permitted`, `server_busy`, …), the full `.body`, the HTTP
`.statusCode`, and `.resumeAt` where the gateway supplied one.

```ts
import { MeshError } from "bold-mesh";

try {
  await llmCaller({ model: "claude-opus-5", prompt: "hi", streaming: false });
} catch (err) {
  if (err instanceof MeshError) console.error(err.code, err.resumeAt, err.body);
  else throw err;
}
```

## Configuration

`MESH_API_KEY` and `MESH_SERVER_PATH` come from `process.env`, or from a `.env` file in the
working directory (real env vars always win). Both can be overridden per call with `apiKey` /
`serverPath`. Point `MESH_DOTENV_PATH` elsewhere to load a different file.

See `src/example.ts` for a runnable tour of every endpoint (`npm run example`).
