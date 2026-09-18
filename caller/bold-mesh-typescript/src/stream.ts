import { MeshError } from "./errors";
import { mergeSources, type MeshResponse, type Source, type Usage } from "./types";

/** One event out of a MeshStream.
 *
 * - "delta"        — `text` is the raw chunk exactly as the provider emitted it.
 * - "sources"      — only the citations discovered since the previous sources
 *                    event (webSearch: true only).
 * - "message_stop" — the stream is finished; `usage` carries the final token
 *                    counts. Same shape as Anthropic's message_stop. */
export type MeshStreamEvent =
  | { type: "delta"; text: string }
  | { type: "sources"; sources: Source[] }
  | { type: "message_stop"; usage: Usage };

const emptyUsage = (): Usage => ({ inputTokens: 0, outputTokens: 0, webSearches: 0, seconds: 0 });

/** Returned by llmCaller({ ... }) when streaming isn't disabled. Iterate it
 * for events (`for await (const event of stream)`), or use `.textStream` for
 * just the text, then call `.finalResponse()` for the assembled MeshResponse.
 * The request is kicked off lazily, on first iteration. */
export class MeshStream implements AsyncIterable<MeshStreamEvent> {
  /** Web-search citations, filled in as they arrive; complete once iteration
   * finishes. Empty unless the call set `webSearch: true`. */
  sources: Source[] = [];
  queries: string[] = [];
  usage: Usage = emptyUsage();
  searchSuggestionsHtml?: string;
  truncated = false;
  incomplete = false;

  private model: string;
  private startFetch: () => Promise<Response>;
  private responsePromise: Promise<Response> | null = null;
  private accumulated = "";
  private final: MeshResponse | null = null;
  private requestId = "";

  constructor(model: string, startFetch: () => Promise<Response>) {
    this.model = model;
    this.startFetch = startFetch;
  }

  private start(): Promise<Response> {
    if (!this.responsePromise) {
      this.responsePromise = this.startFetch().then(async (res) => {
        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new MeshError(body?.error ?? "unknown_error", body, res.status);
        }
        if (!res.body) {
          throw new MeshError("unknown_error", { error: "response had no body" }, res.status);
        }
        return res;
      });
    }
    return this.responsePromise;
  }

  async *[Symbol.asyncIterator](): AsyncGenerator<MeshStreamEvent> {
    if (this.final) return;
    const res = await this.start();
    const reader = res.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    try {
      while (true) {
        let done: boolean;
        let value: Uint8Array | undefined;
        try {
          ({ done, value } = await reader.read());
        } catch {
          // The response was already a 200 and text may have arrived, so a
          // transport failure here means a truncated reply, not a failed call.
          // End the stream and let `incomplete` carry the news rather than
          // throwing a fetch-specific error at the caller.
          return;
        }

        if (done) {
          // The gateway always sends a `done` line; getting here means the
          // connection dropped first. Flush whatever is buffered, then fall
          // through to the finally block, which assembles a partial response
          // rather than leaving finalResponse() with nothing to return.
          const tail = buffer.trim();
          if (tail) {
            const event = this.consumeLine(tail);
            if (event) yield event;
            if (this.final) return;
          }
          return;
        }

        buffer += decoder.decode(value, { stream: true });

        let newlineIndex: number;
        while ((newlineIndex = buffer.indexOf("\n")) !== -1) {
          const line = buffer.slice(0, newlineIndex).trim();
          buffer = buffer.slice(newlineIndex + 1);
          if (!line) continue;

          const event = this.consumeLine(line);
          if (event) yield event;
          if (this.final) return;
        }
      }
    } finally {
      if (!this.final) {
        this.incomplete = true;
        this.assemble();
      }
      reader.releaseLock();
    }
  }

  /** Parses one NDJSON line, folds it into the accumulated state, and returns
   * the event to surface (if any). Sets `this.final` on the `done` line. */
  private consumeLine(line: string): MeshStreamEvent | null {
    let chunk: Record<string, unknown>;
    try {
      chunk = JSON.parse(line);
    } catch {
      return null; // a truncated line can only mean a dropped connection
    }

    if (typeof chunk.id === "string") this.requestId = chunk.id;

    if (Array.isArray(chunk.sources)) {
      const before = this.sources.length;
      this.sources = mergeSources(this.sources, chunk.sources as Source[]);
      const fresh = this.sources.slice(before);
      if (fresh.length && !chunk.done) return { type: "sources", sources: fresh };
    }

    if (chunk.done) {
      const raw = (chunk.usage ?? {}) as Record<string, number>;
      this.usage = {
        inputTokens: raw.input_tokens ?? 0,
        outputTokens: raw.output_tokens ?? 0,
        webSearches: raw.web_searches ?? 0,
        seconds: raw.seconds ?? 0,
      };
      this.queries = Array.isArray(chunk.queries) ? (chunk.queries as string[]) : [];
      this.searchSuggestionsHtml = chunk.search_suggestions_html as string | undefined;
      this.truncated = Boolean(chunk.truncated);
      this.incomplete = Boolean(chunk.incomplete);
      this.assemble();
      return { type: "message_stop", usage: this.usage };
    }

    if (typeof chunk.delta === "string" && chunk.delta) {
      this.accumulated += chunk.delta;
      return { type: "delta", text: chunk.delta };
    }

    return null;
  }

  private assemble(): void {
    this.final = {
      id: this.requestId,
      model: this.model,
      text: this.accumulated,
      usage: this.usage,
      sources: this.sources,
      queries: this.queries,
      searchSuggestionsHtml: this.searchSuggestionsHtml,
      truncated: this.truncated,
      incomplete: this.incomplete,
    };
  }

  /** Just the text deltas, in order — skips every other event. */
  get textStream(): AsyncGenerator<string> {
    const self = this;
    return (async function* () {
      for await (const event of self) {
        if (event.type === "delta") yield event.text;
      }
    })();
  }

  /** Drains the rest of the stream (if not already consumed) and returns the
   * assembled MeshResponse — full text, token usage, web sources, and the
   * truncated/incomplete flags. */
  async finalResponse(): Promise<MeshResponse> {
    if (!this.final) {
      for await (const _ of this) {
        /* drain */
        void _;
      }
    }
    return this.final!;
  }
}
