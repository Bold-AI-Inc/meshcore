import type { ProviderFamily } from "@/lib/api";

export interface ProviderPreset {
  label: string;
  endpointUrl: string;
  httpMethod: string;
  headerTemplate: string;
  requestBodyTemplate: string;
  responseDeltaPath: string;
  usageInputTokensPath: string;
  usageOutputTokensPath: string;
  providerFamily: ProviderFamily;
  model: { name: string; resolvedModel: string };

  webSearchRequestBodyTemplate?: string;
  generationMode?: "sync" | "async_poll";
  generationEndpointUrl?: string;
  generationRequestBodyTemplate?: string;
  generationJobIdPath?: string;
  generationStatusUrlTemplate?: string;
  generationStatusPath?: string;
  generationContentUrlTemplate?: string;
  generationDurationSecondsPath?: string;
  generationUsageInputTokensPath?: string;
  generationUsageOutputTokensPath?: string;
  embeddingUsageTokensPath?: string;
  webSearchPricePerCall?: number;
}

export const PROVIDER_PRESETS: Record<string, ProviderPreset> = {
  anthropic: {
    label: "Anthropic",
    endpointUrl: "https://api.anthropic.com/v1/messages",
    httpMethod: "POST",
    headerTemplate: JSON.stringify(
      { "x-api-key": "{{api_key}}", "anthropic-version": "2023-06-01" },
      null,
      2,
    ),
    requestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        max_tokens: "{{max_tokens_json}}",
        stream: true,
        messages: [{ role: "user", content: "{{content_json}}" }],
      },
      null,
      2,
    ),
    webSearchRequestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        max_tokens: "{{max_tokens_json}}",
        stream: true,
        messages: [{ role: "user", content: "{{content_json}}" }],
        tools: [{ type: "web_search_20250305", name: "web_search", max_uses: 5 }],
      },
      null,
      2,
    ),
    webSearchPricePerCall: 0.01,
    responseDeltaPath: "delta.text",
    usageInputTokensPath: "message.usage.input_tokens",
    usageOutputTokensPath: "usage.output_tokens",
    providerFamily: "anthropic",
    model: { name: "claude-opus", resolvedModel: "claude-opus-4-8" },
  },
  openai: {
    label: "OpenAI",
    endpointUrl: "https://api.openai.com/v1/chat/completions",
    httpMethod: "POST",
    headerTemplate: JSON.stringify({ Authorization: "Bearer {{api_key}}" }, null, 2),
    requestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        messages: [{ role: "user", content: "{{content_json}}" }],
        stream: true,
        stream_options: { include_usage: true },
      },
      null,
      2,
    ),
    webSearchRequestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        messages: [{ role: "user", content: "{{content_json}}" }],
        stream: true,
        stream_options: { include_usage: true },
        web_search_options: {},
      },
      null,
      2,
    ),
    webSearchPricePerCall: 0.025,
    responseDeltaPath: "choices.0.delta.content",
    usageInputTokensPath: "usage.prompt_tokens",
    usageOutputTokensPath: "usage.completion_tokens",
    providerFamily: "openai",
    model: { name: "gpt-4o", resolvedModel: "gpt-4o" },
  },
  "openai-responses": {
    label: "OpenAI (Responses API)",
    endpointUrl: "https://api.openai.com/v1/responses",
    httpMethod: "POST",
    headerTemplate: JSON.stringify({ Authorization: "Bearer {{api_key}}" }, null, 2),
    requestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        input: [{ role: "user", content: "{{content_json}}" }],
        stream: true,
      },
      null,
      2,
    ),
    webSearchRequestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        input: [{ role: "user", content: "{{content_json}}" }],
        stream: true,
        tools: [{ type: "web_search" }],
      },
      null,
      2,
    ),
    webSearchPricePerCall: 0.01,
    responseDeltaPath: "delta",
    usageInputTokensPath: "response.usage.input_tokens",
    usageOutputTokensPath: "response.usage.output_tokens",
    providerFamily: "openai_responses",
    model: { name: "gpt-4o-responses", resolvedModel: "gpt-4o" },
  },
  gemini: {
    label: "Gemini",
    endpointUrl: "https://generativelanguage.googleapis.com/v1beta/models/{{model}}:streamGenerateContent?alt=sse",
    httpMethod: "POST",
    headerTemplate: JSON.stringify({ "x-goog-api-key": "{{api_key}}" }, null, 2),
    requestBodyTemplate: JSON.stringify(
      { contents: [{ role: "user", parts: "{{content_json}}" }] },
      null,
      2,
    ),
    webSearchRequestBodyTemplate: JSON.stringify(
      {
        contents: [{ role: "user", parts: "{{content_json}}" }],
        tools: [{ google_search: {} }],
      },
      null,
      2,
    ),
    webSearchPricePerCall: 0,
    responseDeltaPath: "candidates.0.content.parts.0.text",
    usageInputTokensPath: "usageMetadata.promptTokenCount",
    usageOutputTokensPath: "usageMetadata.candidatesTokenCount",
    providerFamily: "gemini",
    model: { name: "gemini-2.5-pro", resolvedModel: "gemini-2.5-pro" },
  },
  ollama: {
    label: "Ollama / local",
    endpointUrl: "http://localhost:11434/v1/chat/completions",
    httpMethod: "POST",
    headerTemplate: JSON.stringify({ Authorization: "Bearer {{api_key}}" }, null, 2),
    requestBodyTemplate: JSON.stringify(
      {
        model: "{{model}}",
        messages: [{ role: "user", content: "{{content_json}}" }],
        stream: true,
      },
      null,
      2,
    ),
    responseDeltaPath: "choices.0.delta.content",
    usageInputTokensPath: "",
    usageOutputTokensPath: "",
    providerFamily: "ollama",
    model: { name: "llama3.1", resolvedModel: "llama3.1" },
  },
};
