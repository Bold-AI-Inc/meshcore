export type ContentBlockType = "text" | "image" | "document" | "audio" | "video";

export interface ContentBlock {
  type: ContentBlockType;
  text?: string;
  media_type?: string;
  data?: string; // base64, no "data:...;base64," prefix
  url?: string;
}

/** Token counts the provider reported for one call. Either side is 0 if the
 * provider has no usage path configured for it. */
export interface Usage {
  inputTokens: number;
  outputTokens: number;
  /** Billable web searches this call ran (web_search only). */
  webSearches: number;
  /** Seconds of media generated (/v1/generate on duration-priced models). */
  seconds: number;
}

/** One web-search citation. `url` is always present; the rest depend on what
 * the provider supplied. */
export interface Source {
  url: string;
  title?: string;
  cited_text?: string;
  page_age?: string;
}

/** The fully-assembled result of a (streamed or non-streamed) call to
 * llmCaller: the joined text plus everything the final `done` line carried. */
export interface MeshResponse {
  id: string;
  model: string;
  text: string;
  usage: Usage;
  sources: Source[];
  queries: string[];
  searchSuggestionsHtml?: string;
  /** True when the reply stopped because it hit maxTokens, not because the
   * model was finished — raise maxTokens and call again. */
  truncated: boolean;
  /** True when the connection dropped before the provider signalled
   * completion. The text so far is valid, just partial. */
  incomplete: boolean;
}

export interface MediaOutputData {
  type: "url" | "base64";
  data: string;
}

export interface GenerateResult {
  id: string;
  model: string;
  /** The model's fixed output MIME type — use it to pick a file extension. */
  media_type: string;
  output: MediaOutputData;
  outputs: MediaOutputData[];
  usage?: { input_tokens?: number; output_tokens?: number; seconds?: number };
}

export interface EmbeddingResult {
  id: string;
  model: string;
  embedding: number[];
  usage?: { input_tokens?: number };
}

/** kind is "chat" (POST /v1/proxy), "embedding" (POST /v1/embeddings) or
 * "generation" (POST /v1/generate). */
export type ModelKind = "chat" | "embedding" | "generation";

export type ModelStatus = "available" | "rate_limited" | "budget_exceeded" | "outside_hours";

export interface ModelInfo {
  model: string;
  kind: ModelKind;
  formats_accepted: string[];
  /** Whether `webSearch: true` will be accepted for this model. */
  web_search: boolean;
  status: ModelStatus;
  usage: {
    calls_this_hour: number;
    calls_per_hour_limit: number | null;
    spent_today_usd: number;
    daily_cap_usd: number | null;
  };
  /** Only on kind: "generation" — the MIME type it produces. */
  output_media_type?: string;
  /** RFC3339 timestamp at which a non-available status lifts. */
  resume_at?: string;
  allowed_hours?: { from: string; to: string; timezone: string; days: number[] };
}

export interface UsageInfo {
  model: string;
  kind: ModelKind;
  spent_usd: number;
  /** null when no daily cap is set for this model. */
  daily_cap_usd: number | null;
  remaining_usd: number | null;
}

/** Merges by url, in first-seen order. Mesh streams newly-discovered sources
 * as it finds them and then repeats the complete set on the final `done` line,
 * so a naive concat would duplicate every one. The repeat is often richer than
 * the mid-stream version (it carries `cited_text`), so a source we already have
 * is topped up field by field rather than dropped. */
export function mergeSources(existing: Source[], incoming: Source[]): Source[] {
  const merged = [...existing];
  const index = new Map(merged.map((s, i) => [s.url, i]));

  for (const source of incoming) {
    const position = index.get(source.url);
    if (position === undefined) {
      index.set(source.url, merged.length);
      merged.push(source);
      continue;
    }
    const current = merged[position];
    merged[position] = {
      url: current.url,
      title: current.title ?? source.title,
      cited_text: current.cited_text ?? source.cited_text,
      page_age: current.page_age ?? source.page_age,
    };
  }
  return merged;
}
