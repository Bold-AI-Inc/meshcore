/**
 * Mesh in one file. Copy mesh.js into your project and `require("./mesh")`.
 *
 * Nothing to install at all -- uses Node's built-in fetch (Node 18+), no
 * package.json, nothing from npm. If you want the fuller client with types
 * and content-block builders, use caller/bold-mesh-typescript instead.
 *
 * Set two environment variables first (or pass apiKey/serverPath per call):
 *
 *   export MESH_API_KEY=mesh_live_xxxxxxxx        # macOS/Linux
 *   export MESH_SERVER_PATH=https://your-mesh-server.example.com
 *
 *   set MESH_API_KEY=mesh_live_xxxxxxxx           # Windows
 *
 * Quick tour:
 *
 *   const { llmCaller } = require("./mesh");
 *
 *   for await (const chunk of llmCaller("claude-sonnet-5", "hello")) {
 *     process.stdout.write(chunk);                // streams
 *   }
 *
 *   const r = await llmCaller("claude-sonnet-5", "hello", { streaming: false });
 *   console.log(r.text, r.usage);                 // blocks
 */

const fs = require("fs");
const path = require("path");

const DEFAULT_SERVER_PATH = "http://localhost:8080";

/** What the gateway applies when you don't pass maxTokens. */
const DEFAULT_MAX_TOKENS = 4096;

/**
 * Thrown for any non-2xx response from the Mesh gateway. `code` is the
 * machine-readable error code ("budget_exceeded", "server_busy", ...); `body`
 * is the full parsed JSON error, which for some codes carries extra fields
 * such as `resume_at` or `cap`.
 */
class MeshError extends Error {
  constructor(code, body, statusCode = 0) {
    super(code);
    this.name = "MeshError";
    this.code = code;
    this.body = body && typeof body === "object" ? body : {};
    this.statusCode = statusCode;
  }

  get resumeAt() {
    return this.body.resume_at;
  }
}

function resolveConfig(apiKey, serverPath) {
  const resolvedKey = apiKey || process.env.MESH_API_KEY;
  if (!resolvedKey) {
    throw new Error("MESH_API_KEY is not set (env var or apiKey option)");
  }
  const resolvedPath = (serverPath || process.env.MESH_SERVER_PATH || DEFAULT_SERVER_PATH).replace(/\/+$/, "");
  return { resolvedKey, resolvedPath };
}

function authHeaders(apiKey) {
  return { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" };
}

/** Mesh answers every failure with {"error": "<code>", ...}. Turn that into a
 * MeshError; turn anything else (an HTML 502 from a proxy, say) into one too. */
async function checkResponse(res) {
  if (res.ok) return;
  const body = await res.json().catch(() => ({}));
  throw new MeshError(body?.error || "unknown_error", body, res.status);
}

const MIME_BY_EXT = {
  ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif",
  ".webp": "image/webp", ".bmp": "image/bmp", ".heic": "image/heic",
  ".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
  ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  ".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".m4a": "audio/mp4",
  ".flac": "audio/flac",
  ".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
};

/** Reads a local file into a base64 content block, guessing its MIME type
 * from the extension. */
function fileBlock(blockType, filePath) {
  const mediaType = MIME_BY_EXT[path.extname(filePath).toLowerCase()];
  if (!mediaType) {
    throw new Error(`couldn't guess the media type of ${filePath} -- rename it or build the block by hand`);
  }
  return { type: blockType, media_type: mediaType, data: fs.readFileSync(filePath).toString("base64") };
}

/** Merges by url, in first-seen order. Mesh streams newly-discovered sources as
 * it finds them and then repeats the complete set on the final line, so a naive
 * concat would duplicate every one. The repeat is often richer than the
 * mid-stream version (it carries cited_text), so a source we already have is
 * topped up field by field rather than dropped. */
function mergeSources(existing, incoming) {
  const merged = [...existing];
  const index = new Map(merged.map((s, i) => [s.url, i]));

  for (const source of incoming) {
    const position = index.get(source.url);
    if (position === undefined) {
      index.set(source.url, merged.length);
      merged.push({ ...source });
    } else {
      merged[position] = { ...source, ...merged[position] };
    }
  }
  return merged;
}

/** Turns the NDJSON response body into an async iterable of parsed lines. */
async function* parseLines(res) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      let done, value;
      try {
        ({ done, value } = await reader.read());
      } catch {
        // The response was already a 200 and text may have arrived, so a
        // transport failure here means a truncated reply, not a failed call.
        // End the stream and let `incomplete` carry the news.
        return;
      }
      if (done) {
        const tail = buffer.trim();
        if (tail) {
          try {
            yield JSON.parse(tail);
          } catch {
            /* a truncated line can only mean a dropped connection */
          }
        }
        return;
      }
      buffer += decoder.decode(value, { stream: true });

      let newlineIndex;
      while ((newlineIndex = buffer.indexOf("\n")) !== -1) {
        const line = buffer.slice(0, newlineIndex).trim();
        buffer = buffer.slice(newlineIndex + 1);
        if (!line) continue;
        try {
          yield JSON.parse(line);
        } catch {
          /* same */
        }
      }
    }
  } finally {
    reader.releaseLock();
  }
}

async function openProxyStream(model, prompt, opts, resolvedKey, resolvedPath) {
  const { maxTokens, webSearch, images, documents, audio, video, signal } = opts;

  if (maxTokens !== undefined && maxTokens <= 0) {
    throw new Error("maxTokens must be a positive integer");
  }

  const attachments = [
    ["image", images || []],
    ["document", documents || []],
    ["audio", audio || []],
    ["video", video || []],
  ];

  let body;
  if (attachments.some(([, paths]) => paths.length > 0)) {
    const content = [{ type: "text", text: prompt }];
    for (const [blockType, paths] of attachments) {
      for (const p of paths) content.push(fileBlock(blockType, p));
    }
    body = { content };
  } else {
    body = { query: prompt };
  }

  if (maxTokens !== undefined) body.max_tokens = maxTokens;
  if (webSearch) body.web_search = true;

  const res = await fetch(`${resolvedPath}/v1/proxy?model=${encodeURIComponent(model)}`, {
    method: "POST",
    headers: authHeaders(resolvedKey),
    body: JSON.stringify(body),
    signal,
  });
  await checkResponse(res);
  return res;
}

/**
 * Calls a chat model over POST /v1/proxy.
 *
 * streaming: true (the default) returns an async generator that yields each
 * text delta as it arrives. Token counts and web-search results are attached
 * to the generator object itself, and are complete once the loop ends:
 *
 *   const stream = llmCaller("claude-sonnet-5", "hello");
 *   for await (const chunk of stream) process.stdout.write(chunk);
 *   console.log(stream.usage, stream.sources);
 *
 * streaming: false returns a Promise resolving to:
 *
 *   { text, usage, sources, queries, searchSuggestionsHtml, truncated, incomplete }
 *
 * truncated means the reply hit maxTokens instead of finishing; incomplete
 * means the connection dropped before the provider signalled completion.
 *
 * opts.images / documents / audio / video take arrays of local file paths to
 * attach alongside the prompt. Whether a model accepts a given type is in
 * models()[i].formats_accepted.
 *
 * opts.maxTokens caps the reply (gateway default 4096).
 *
 * opts.webSearch lets the model search the web and cite what it found. Only
 * models with web_search: true in models() accept it -- anywhere else this
 * throws MeshError("web_search_not_supported_by_model"). Searches bill on top
 * of tokens against the same daily cap.
 */
function llmCaller(model, prompt, opts = {}) {
  const { streaming = true, apiKey, serverPath } = opts;
  const { resolvedKey, resolvedPath } = resolveConfig(apiKey, serverPath);

  // The generator object is created first so usage/sources can be attached to
  // the very thing the caller holds -- they read it after the loop, no second
  // handle needed.
  const stream = (async function* () {
    let sawDone = false;
    try {
      const res = await openProxyStream(model, prompt, opts, resolvedKey, resolvedPath);
      for await (const chunk of parseLines(res)) {
        if (chunk.id) stream.id = chunk.id;
        if (chunk.sources) stream.sources = mergeSources(stream.sources, chunk.sources);
        if (chunk.done) {
          sawDone = true;
          stream.usage = chunk.usage || stream.usage;
          stream.queries = chunk.queries || stream.queries;
          stream.searchSuggestionsHtml = chunk.search_suggestions_html;
          stream.truncated = Boolean(chunk.truncated);
          stream.incomplete = Boolean(chunk.incomplete);
          return;
        }
        if (chunk.delta) {
          stream.text += chunk.delta;
          yield chunk.delta;
        }
      }
    } finally {
      // The gateway always sends a "done" line; not reaching one means the
      // connection dropped. Say so rather than pretending the reply ended.
      if (!sawDone) stream.incomplete = true;
    }
  })();

  stream.id = "";
  stream.text = "";
  stream.sources = [];
  stream.queries = [];
  stream.usage = { input_tokens: 0, output_tokens: 0 };
  stream.searchSuggestionsHtml = undefined;
  stream.truncated = false;
  stream.incomplete = false;

  if (streaming) return stream;

  return (async () => {
    for await (const _chunk of stream) {
      /* drain -- the deltas are accumulated onto stream.text */
    }
    return {
      id: stream.id,
      text: stream.text,
      usage: stream.usage,
      sources: stream.sources,
      queries: stream.queries,
      searchSuggestionsHtml: stream.searchSuggestionsHtml,
      truncated: stream.truncated,
      incomplete: stream.incomplete,
    };
  })();
}

/**
 * Calls POST /v1/generate for image/video generation models (the ones
 * models() reports as kind: "generation"). Resolves to:
 *
 *   { id, model, media_type, output: { type, data }, outputs: [...], usage }
 *
 * output.type is "base64" (raw bytes) or "url" (hosted by the provider, not
 * by Mesh). Use saveOutput() to write it to disk. Mesh does not store the
 * generated file anywhere -- it is in this response and nowhere else.
 *
 * Generation is synchronous and can take minutes; don't wrap it in a short
 * AbortSignal timeout.
 */
async function llmGenerate(model, prompt, opts = {}) {
  const { resolvedKey, resolvedPath } = resolveConfig(opts.apiKey, opts.serverPath);
  const res = await fetch(`${resolvedPath}/v1/generate?model=${encodeURIComponent(model)}`, {
    method: "POST",
    headers: authHeaders(resolvedKey),
    body: JSON.stringify({ prompt }),
    signal: opts.signal,
  });
  await checkResponse(res);
  return res.json();
}

/** Writes one /v1/generate output ({ type, data }) to a local file. */
async function saveOutput(output, filePath) {
  let bytes;
  if (output.type === "base64") {
    bytes = Buffer.from(output.data, "base64");
  } else if (output.type === "url") {
    const res = await fetch(output.data);
    if (!res.ok) throw new Error(`failed to download ${output.data}: ${res.status}`);
    bytes = Buffer.from(await res.arrayBuffer());
  } else {
    throw new Error(`unknown output type ${output.type}`);
  }
  fs.writeFileSync(filePath, bytes);
}

/** Calls POST /v1/embeddings. One text input per call -- the gateway does not
 * take batch input. Resolves to { id, model, embedding, usage }. */
async function llmEmbeddings(model, input, opts = {}) {
  const { resolvedKey, resolvedPath } = resolveConfig(opts.apiKey, opts.serverPath);
  const res = await fetch(`${resolvedPath}/v1/embeddings?model=${encodeURIComponent(model)}`, {
    method: "POST",
    headers: authHeaders(resolvedKey),
    body: JSON.stringify({ input }),
    signal: opts.signal,
  });
  await checkResponse(res);
  return res.json();
}

/** Calls GET /v1/models. Resolves to an array of { model, kind,
 * formats_accepted, web_search, status, usage, resume_at?, allowed_hours? }. */
async function models(opts = {}) {
  const { resolvedKey, resolvedPath } = resolveConfig(opts.apiKey, opts.serverPath);
  const res = await fetch(`${resolvedPath}/v1/models`, { headers: authHeaders(resolvedKey) });
  await checkResponse(res);
  return (await res.json()).models || [];
}

/** Calls GET /v1/usage. Resolves to an array of { model, kind, spent_usd,
 * daily_cap_usd, remaining_usd } (the last two are null with no cap set). */
async function usage(opts = {}) {
  const { resolvedKey, resolvedPath } = resolveConfig(opts.apiKey, opts.serverPath);
  const res = await fetch(`${resolvedPath}/v1/usage`, { headers: authHeaders(resolvedKey) });
  await checkResponse(res);
  return (await res.json()).usage || [];
}

module.exports = {
  llmCaller,
  llmGenerate,
  llmEmbeddings,
  models,
  usage,
  saveOutput,
  MeshError,
  DEFAULT_MAX_TOKENS,
  DEFAULT_SERVER_PATH,
};
