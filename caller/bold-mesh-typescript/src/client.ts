import { loadDotenvOnce } from "./env";
import { MeshConfigError, MeshError } from "./errors";
import { flattenContent } from "./blocks";
import { MeshStream } from "./stream";
import type {
  ContentBlock,
  EmbeddingResult,
  GenerateResult,
  MediaOutputData,
  MeshResponse,
  ModelInfo,
  UsageInfo,
} from "./types";

export const DEFAULT_SERVER_PATH = "http://localhost:8080";

/** Applied when you don't pass maxTokens. Mirrors the gateway's own default
 * (server/internal/gateway/contract.go: defaultMaxTokens). */
export const DEFAULT_MAX_TOKENS = 4096;

interface Config {
  apiKey?: string;
  serverPath?: string;
}

function resolveConfig(config: Config): { apiKey: string; serverPath: string } {
  loadDotenvOnce();

  const env = typeof process !== "undefined" ? process.env : undefined;
  const apiKey = config.apiKey ?? env?.MESH_API_KEY;
  if (!apiKey) {
    throw new MeshConfigError(
      "MESH_API_KEY is not set. Set it in your environment, in a .env file, " +
        "or pass apiKey explicitly to this call.",
    );
  }

  const serverPath = (config.serverPath ?? env?.MESH_SERVER_PATH ?? DEFAULT_SERVER_PATH).replace(/\/+$/, "");
  return { apiKey, serverPath };
}

function headers(apiKey: string): Record<string, string> {
  return { Authorization: `Bearer ${apiKey}`, "Content-Type": "application/json" };
}

/** Every non-2xx from Mesh is a JSON body of the shape
 * { "error": "<code>", ...extra }. Anything else (a proxy's HTML 502, say)
 * still becomes a MeshError rather than a JSON parse exception. */
async function raiseForError(res: Response): Promise<never> {
  const body = await res.json().catch(() => ({}));
  throw new MeshError((body as { error?: string })?.error ?? "unknown_error", body, res.status);
}

export interface LlmCallerOptions extends Config {
  model: string;
  /** Plain text. Pass exactly one of `prompt` or `content`. */
  prompt?: string;
  /** Typed blocks from textBlock/imageBlock/documentBlock/audioBlock/videoBlock. */
  content?: (ContentBlock | ContentBlock[])[];
  streaming?: boolean;
  /** Caps the reply length; omitting it uses the gateway default of 4096. If
   * the result comes back with `truncated`, it hit the ceiling rather than
   * finishing — raise this and call again. */
  maxTokens?: number;
  /** Lets the model search the web and cite what it found. Only works on
   * models whose listModels() entry has `web_search: true`; anywhere else it
   * throws MeshError("web_search_not_supported_by_model"). Searches bill on
   * top of tokens, against the same daily cap. */
  webSearch?: boolean;
  /** AbortSignal to cancel the underlying fetch. */
  signal?: AbortSignal;
}

/**
 * Calls a chat model over Mesh's POST /v1/proxy.
 *
 * `streaming: true` (the default) returns a MeshStream synchronously —
 * `for await` its `.textStream`, then `await stream.finalResponse()`.
 *
 * `streaming: false` returns a Promise<MeshResponse> that resolves once the
 * full reply has arrived.
 */
export function llmCaller(opts: LlmCallerOptions & { streaming: false }): Promise<MeshResponse>;
export function llmCaller(opts: LlmCallerOptions & { streaming?: true }): MeshStream;
export function llmCaller(opts: LlmCallerOptions): MeshStream | Promise<MeshResponse> {
  const { apiKey, serverPath } = resolveConfig(opts);

  if ((opts.prompt === undefined) === (opts.content === undefined)) {
    throw new Error("pass exactly one of prompt or content");
  }
  if (opts.maxTokens !== undefined && opts.maxTokens <= 0) {
    throw new Error("maxTokens must be a positive integer");
  }

  const body: Record<string, unknown> =
    opts.content === undefined ? { query: opts.prompt } : { content: flattenContent(opts.content) };
  if (opts.maxTokens !== undefined) body.max_tokens = opts.maxTokens;
  if (opts.webSearch) body.web_search = true;

  const stream = new MeshStream(opts.model, () =>
    fetch(`${serverPath}/v1/proxy?model=${encodeURIComponent(opts.model)}`, {
      method: "POST",
      headers: headers(apiKey),
      body: JSON.stringify(body),
      signal: opts.signal,
    }),
  );

  return opts.streaming === false ? stream.finalResponse() : stream;
}

export interface LlmGenerateOptions extends Config {
  model: string;
  prompt: string;
  signal?: AbortSignal;
}

/** Calls POST /v1/generate for image/video generation models (the ones
 * listModels() reports as kind: "generation"). Generation is synchronous and
 * can take minutes — don't wrap it in a short timeout.
 *
 * Use saveMediaOutput(result.output, "out.png") to write the result to disk.
 * Mesh never stores the generated file: it is in this response and nowhere
 * else. */
export async function llmGenerate(opts: LlmGenerateOptions): Promise<GenerateResult> {
  const { apiKey, serverPath } = resolveConfig(opts);
  const res = await fetch(`${serverPath}/v1/generate?model=${encodeURIComponent(opts.model)}`, {
    method: "POST",
    headers: headers(apiKey),
    body: JSON.stringify({ prompt: opts.prompt }),
    signal: opts.signal,
  });
  if (!res.ok) return raiseForError(res);
  return (await res.json()) as GenerateResult;
}

export interface LlmEmbeddingsOptions extends Config {
  model: string;
  input: string;
  signal?: AbortSignal;
}

/** Calls POST /v1/embeddings. One text input per call (the gateway does not
 * take batch input) — call this once per input. */
export async function llmEmbeddings(opts: LlmEmbeddingsOptions): Promise<EmbeddingResult> {
  const { apiKey, serverPath } = resolveConfig(opts);
  const res = await fetch(`${serverPath}/v1/embeddings?model=${encodeURIComponent(opts.model)}`, {
    method: "POST",
    headers: headers(apiKey),
    body: JSON.stringify({ input: opts.input }),
    signal: opts.signal,
  });
  if (!res.ok) return raiseForError(res);
  return (await res.json()) as EmbeddingResult;
}

/** Calls GET /v1/models — every model you have access to, what content types
 * it accepts, whether it can web-search, and whether you can call it right
 * now. Blocked and retired models never appear. */
export async function listModels(config: Config = {}): Promise<ModelInfo[]> {
  const { apiKey, serverPath } = resolveConfig(config);
  const res = await fetch(`${serverPath}/v1/models`, { headers: headers(apiKey) });
  if (!res.ok) return raiseForError(res);
  return ((await res.json()) as { models?: ModelInfo[] }).models ?? [];
}

/** Calls GET /v1/usage — today's spend and remaining daily budget per model
 * (cumulative, not per-request; for per-call token counts read `.usage` off
 * llmCaller's MeshResponse/MeshStream instead). */
export async function getUsage(config: Config = {}): Promise<UsageInfo[]> {
  const { apiKey, serverPath } = resolveConfig(config);
  const res = await fetch(`${serverPath}/v1/usage`, { headers: headers(apiKey) });
  if (!res.ok) return raiseForError(res);
  return ((await res.json()) as { usage?: UsageInfo[] }).usage ?? [];
}

/** Writes a /v1/generate output ({ type: "url" | "base64", data }) to a local
 * file (Node only — needs "fs"). Decodes base64 directly, or downloads a url
 * first. */
export async function saveMediaOutput(output: MediaOutputData, path: string): Promise<void> {
  let bytes: Uint8Array;
  if (output.type === "base64") {
    bytes =
      typeof Buffer !== "undefined"
        ? new Uint8Array(Buffer.from(output.data, "base64"))
        : base64DecodeBrowser(output.data);
  } else if (output.type === "url") {
    const res = await fetch(output.data);
    if (!res.ok) throw new Error(`failed to download ${output.data}: ${res.status}`);
    bytes = new Uint8Array(await res.arrayBuffer());
  } else {
    throw new Error(`unknown output type ${(output as MediaOutputData).type}`);
  }
  const fs = await import("fs/promises");
  await fs.writeFile(path, bytes);
}

function base64DecodeBrowser(data: string): Uint8Array {
  const binary = atob(data);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes;
}
