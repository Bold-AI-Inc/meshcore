"use client";

import { useState, type FormEvent } from "react";
import { updateProviderSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { updateProvider, ApiError, type Provider, type ProviderFamily } from "@/lib/api";
import PasswordField from "@/components/PasswordField";
import { Field, TextArea } from "@/components/FormFields";

const PROVIDER_FAMILIES: { value: ProviderFamily; label: string }[] = [
  { value: "generic", label: "Generic (text only)" },
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "ollama", label: "Ollama / OpenAI-compatible local" },
];

interface EditProviderFormProps {
  provider: Provider;
  onSaved: (provider: Provider) => void;
  onCancel: () => void;
}

export default function EditProviderForm({ provider, onSaved, onCancel }: EditProviderFormProps) {
  const [name, setName] = useState(provider.name);
  const [apiKey, setApiKey] = useState("");
  const [endpointUrl, setEndpointUrl] = useState(provider.endpoint_url);
  const [httpMethod, setHttpMethod] = useState(provider.http_method);
  const [headerTemplate, setHeaderTemplate] = useState(JSON.stringify(provider.header_template, null, 2));
  const [requestBodyTemplate, setRequestBodyTemplate] = useState(
    JSON.stringify(provider.request_body_template, null, 2),
  );
  const [responseDeltaPath, setResponseDeltaPath] = useState(provider.response_delta_path);
  const [usageInputTokensPath, setUsageInputTokensPath] = useState(provider.usage_input_tokens_path ?? "");
  const [usageOutputTokensPath, setUsageOutputTokensPath] = useState(provider.usage_output_tokens_path ?? "");
  const [maxOutboundRps, setMaxOutboundRps] = useState(String(provider.max_outbound_rps));
  const [maxConcurrentUpstream, setMaxConcurrentUpstream] = useState(String(provider.max_concurrent_upstream));
  const [maxRetries, setMaxRetries] = useState(String(provider.max_retries));
  const [retryBackoffMs, setRetryBackoffMs] = useState(String(provider.retry_backoff_ms));
  const [maxCallsPerHour, setMaxCallsPerHour] = useState(
    provider.max_calls_per_hour == null ? "" : String(provider.max_calls_per_hour),
  );
  const [providerFamily, setProviderFamily] = useState<ProviderFamily>(provider.provider_family);
  const [supportsEmbeddings, setSupportsEmbeddings] = useState(provider.supports_embeddings);
  const [embeddingEndpointUrl, setEmbeddingEndpointUrl] = useState(provider.embedding_endpoint_url ?? "");
  const [embeddingRequestBodyTemplate, setEmbeddingRequestBodyTemplate] = useState(
    provider.embedding_request_body_template ? JSON.stringify(provider.embedding_request_body_template, null, 2) : "",
  );
  const [embeddingResponseVectorPath, setEmbeddingResponseVectorPath] = useState(
    provider.embedding_response_vector_path ?? "",
  );
  const [supportsWebSearch, setSupportsWebSearch] = useState(provider.supports_web_search);
  const [webSearchRequestBodyTemplate, setWebSearchRequestBodyTemplate] = useState(
    provider.web_search_request_body_template ? JSON.stringify(provider.web_search_request_body_template, null, 2) : "",
  );
  const [webSearchPricePerCall, setWebSearchPricePerCall] = useState(String(provider.web_search_price_per_call ?? 0));
  const [generationMode, setGenerationMode] = useState<"sync" | "async_poll">(provider.generation_mode);
  const [generationJobIdPath, setGenerationJobIdPath] = useState(provider.generation_job_id_path ?? "");
  const [generationStatusUrlTemplate, setGenerationStatusUrlTemplate] = useState(provider.generation_status_url_template ?? "");
  const [generationStatusPath, setGenerationStatusPath] = useState(provider.generation_status_path ?? "");
  const [generationContentUrlTemplate, setGenerationContentUrlTemplate] = useState(provider.generation_content_url_template ?? "");
  const [generationPollIntervalMs, setGenerationPollIntervalMs] = useState(String(provider.generation_poll_interval_ms ?? 3000));
  const [generationMaxWaitSeconds, setGenerationMaxWaitSeconds] = useState(String(provider.generation_max_wait_seconds ?? 600));
  const [embeddingUsageTokensPath, setEmbeddingUsageTokensPath] = useState(provider.embedding_usage_tokens_path ?? "");
  const [generationUsageInputTokensPath, setGenerationUsageInputTokensPath] = useState(provider.generation_usage_input_tokens_path ?? "");
  const [generationUsageOutputTokensPath, setGenerationUsageOutputTokensPath] = useState(provider.generation_usage_output_tokens_path ?? "");
  const [generationDurationSecondsPath, setGenerationDurationSecondsPath] = useState(provider.generation_duration_seconds_path ?? "");
  const [supportsGeneration, setSupportsGeneration] = useState(provider.supports_generation);
  const [generationEndpointUrl, setGenerationEndpointUrl] = useState(provider.generation_endpoint_url ?? "");
  const [generationRequestBodyTemplate, setGenerationRequestBodyTemplate] = useState(
    provider.generation_request_body_template ? JSON.stringify(provider.generation_request_body_template, null, 2) : "",
  );
  const [generationResponseMediaPath, setGenerationResponseMediaPath] = useState(
    provider.generation_response_media_path ?? "",
  );
  const [logRequests, setLogRequests] = useState(provider.log_requests);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [verifying, setVerifying] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});

    const result = updateProviderSchema.safeParse({
      name,
      apiKey,
      endpointUrl,
      httpMethod,
      headerTemplate,
      requestBodyTemplate,
      responseDeltaPath,
      usageInputTokensPath,
      usageOutputTokensPath,
      maxOutboundRps,
      maxConcurrentUpstream,
      maxRetries,
      retryBackoffMs,
    });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setVerifying(true);
    try {
      const updated = await updateProvider(provider.id, {
        ...result.data,
        maxCallsPerHour: maxCallsPerHour.trim() === "" ? null : Number(maxCallsPerHour) || null,
        responseDoneSignal: provider.response_done_signal,
        providerFamily,
        supportsEmbeddings,
        embeddingEndpointUrl,
        embeddingRequestBodyTemplate,
        embeddingResponseVectorPath,
        supportsWebSearch,
        webSearchRequestBodyTemplate,
        webSearchPricePerCall: Number(webSearchPricePerCall) || 0,
        generationMode,
        generationJobIdPath,
        generationStatusUrlTemplate,
        generationStatusPath,
        generationContentUrlTemplate,
        generationPollIntervalMs: Number(generationPollIntervalMs) || 3000,
        generationMaxWaitSeconds: Number(generationMaxWaitSeconds) || 600,
        embeddingUsageTokensPath,
        generationUsageInputTokensPath,
        generationUsageOutputTokensPath,
        generationDurationSecondsPath,
        supportsGeneration,
        generationEndpointUrl,
        generationRequestBodyTemplate,
        generationResponseMediaPath,
        logRequests,
      });
      onSaved(updated);
    } catch (err) {
      if (err instanceof ApiError && err.message === "model_verification_failed") {
        const failures = (err.details as { failures?: { model: string; reason: string }[] })?.failures ?? [];
        const reasons = failures.map((f) => `${f.model}: ${f.reason}`).join("; ");
        setFormError(`One or more existing models failed verification — nothing was saved. ${reasons}`);
      } else if (err instanceof ApiError && err.message === "provider_already_exists") {
        setFormError("A provider with this name already exists");
      } else if (err instanceof ApiError) {
        setFormError(`Could not save provider (${err.message})`);
      } else {
        setFormError("Could not reach the backend — check it's running and reachable");
      }
    } finally {
      setVerifying(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="thin-scroll space-y-2.5 max-h-[75vh] overflow-y-auto pr-2">
      <Field label="Name" value={name} onChange={setName} error={fieldErrors.name} />
      <PasswordField
        id="editApiKey"
        label="API key (leave blank to keep existing key)"
        value={apiKey}
        onChange={setApiKey}
        error={fieldErrors.apiKey}
      />
      <Field
        label="Endpoint URL"
        value={endpointUrl}
        onChange={setEndpointUrl}
        error={fieldErrors.endpointUrl}
      />
      <Field label="HTTP method" value={httpMethod} onChange={setHttpMethod} error={fieldErrors.httpMethod} />
      <div>
        <label className="block text-xs font-medium mb-1">Provider family (multimodal content shape)</label>
        <select
          value={providerFamily}
          onChange={(e) => setProviderFamily(e.target.value as ProviderFamily)}
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500 bg-white"
        >
          {PROVIDER_FAMILIES.map((f) => (
            <option key={f.value} value={f.value}>
              {f.label}
            </option>
          ))}
        </select>
      </div>
      <TextArea
        label="Header template (JSON)"
        value={headerTemplate}
        onChange={setHeaderTemplate}
        error={fieldErrors.headerTemplate}
      />
      <TextArea
        label="Request body template (JSON)"
        value={requestBodyTemplate}
        onChange={setRequestBodyTemplate}
        error={fieldErrors.requestBodyTemplate}
      />
      <Field
        label="Response delta path"
        value={responseDeltaPath}
        onChange={setResponseDeltaPath}
        error={fieldErrors.responseDeltaPath}
      />
      <div className="grid grid-cols-2 gap-2">
        <Field
          label="Usage input tokens path (optional)"
          value={usageInputTokensPath}
          onChange={setUsageInputTokensPath}
          error={fieldErrors.usageInputTokensPath}
          placeholder="usage.prompt_tokens"
        />
        <Field
          label="Usage output tokens path (optional)"
          value={usageOutputTokensPath}
          onChange={setUsageOutputTokensPath}
          error={fieldErrors.usageOutputTokensPath}
          placeholder="usage.completion_tokens"
        />
      </div>
      <div className="grid grid-cols-2 gap-2">
        <Field label="Max outbound RPS" value={maxOutboundRps} onChange={setMaxOutboundRps} error={fieldErrors.maxOutboundRps} />
        <Field label="Max concurrent" value={maxConcurrentUpstream} onChange={setMaxConcurrentUpstream} error={fieldErrors.maxConcurrentUpstream} />
        <Field label="Max retries" value={maxRetries} onChange={setMaxRetries} error={fieldErrors.maxRetries} />
        <Field label="Retry backoff (ms)" value={retryBackoffMs} onChange={setRetryBackoffMs} error={fieldErrors.retryBackoffMs} />
        <Field label="Max calls/hour (blank = unlimited)" value={maxCallsPerHour} onChange={setMaxCallsPerHour} error={fieldErrors.maxCallsPerHour} />
      </div>

      <div className="border-t border-gray-100 pt-2.5 space-y-2">
        <label className="flex items-center gap-1.5 text-xs font-medium">
          <input type="checkbox" checked={supportsEmbeddings} onChange={(e) => setSupportsEmbeddings(e.target.checked)} />
          This provider also supports embeddings
        </label>
        {supportsEmbeddings && (
          <div className="rounded-md border border-gray-200 p-2.5 space-y-1.5 bg-gray-50">
            <Field
              label="Embedding endpoint URL"
              value={embeddingEndpointUrl}
              onChange={setEmbeddingEndpointUrl}
              placeholder="https://api.openai.com/v1/embeddings"
            />
            <TextArea
              label="Embedding request body template (JSON) — use {{input_json}} for the input text"
              value={embeddingRequestBodyTemplate}
              onChange={setEmbeddingRequestBodyTemplate}
              placeholder={'{\n  "model": "{{model}}",\n  "input": "{{input_json}}"\n}'}
            />
            <Field label="Embedding usage tokens path (for cost)" value={embeddingUsageTokensPath} onChange={setEmbeddingUsageTokensPath} placeholder="usage.prompt_tokens" />
            <Field
              label="Embedding response vector path"
              value={embeddingResponseVectorPath}
              onChange={setEmbeddingResponseVectorPath}
              placeholder="data.0.embedding"
            />
          </div>
        )}
      </div>

      <div className="border-t border-gray-100 pt-2.5 space-y-2">
        <label className="flex items-center gap-1.5 text-xs font-medium">
          <input type="checkbox" checked={supportsWebSearch} onChange={(e) => setSupportsWebSearch(e.target.checked)} />
          This provider supports web search
        </label>
        {supportsWebSearch && (
          <div className="rounded-md border border-gray-200 p-2.5 space-y-1.5 bg-gray-50">
            <TextArea
              label="Web search request body template (JSON) — sent instead of the normal body when a caller sets web_search: true"
              value={webSearchRequestBodyTemplate}
              onChange={setWebSearchRequestBodyTemplate}
              placeholder={'{\n  "model": "{{model}}",\n  "messages": [{ "role": "user", "content": "{{content_json}}" }],\n  "stream": true,\n  "tools": [{ "type": "web_search_20250305", "name": "web_search", "max_uses": 5 }]\n}'}
            />
            <Field
              label="Price per search (USD) — billed on top of tokens, against the caller's daily cap"
              value={webSearchPricePerCall}
              onChange={setWebSearchPricePerCall}
              placeholder="0.01"
            />
            <p className="text-[11px] text-gray-400">
              Same endpoint and headers as the normal chat call — only the body differs. Mesh extracts
              the cited sources out of the response stream and returns them as <code>sources</code>.
              Tick <b>Web search</b> on each model that can actually use it.
            </p>
          </div>
        )}
      </div>

      <div className="border-t border-gray-100 pt-2.5 space-y-2">
        <label className="flex items-center gap-1.5 text-xs font-medium">
          <input type="checkbox" checked={supportsGeneration} onChange={(e) => setSupportsGeneration(e.target.checked)} />
          This provider also supports image/video generation
        </label>
        {supportsGeneration && (
          <div className="rounded-md border border-gray-200 p-2.5 space-y-1.5 bg-gray-50">
            <Field
              label="Generation endpoint URL"
              value={generationEndpointUrl}
              onChange={setGenerationEndpointUrl}
              placeholder="https://api.openai.com/v1/images/generations"
            />
            <TextArea
              label="Generation request body template (JSON) — use {{prompt_json}} for the prompt text"
              value={generationRequestBodyTemplate}
              onChange={setGenerationRequestBodyTemplate}
              placeholder={'{\n  "model": "{{model}}",\n  "prompt": "{{prompt_json}}",\n  "n": 1\n}'}
            />
            <label className="flex items-center gap-1.5 text-xs font-medium mt-1">
              <input
                type="checkbox"
                checked={generationMode === "async_poll"}
                onChange={(e) => setGenerationMode(e.target.checked ? "async_poll" : "sync")}
              />
              This endpoint returns a job to poll (video / Sora)
            </label>
            {generationMode === "async_poll" && (
              <div className="rounded-md border border-gray-200 p-2 space-y-1.5 bg-white">
                <Field label="Job id path" value={generationJobIdPath} onChange={setGenerationJobIdPath} placeholder="id" />
                <Field label="Status URL template ({{job_id}})" value={generationStatusUrlTemplate} onChange={setGenerationStatusUrlTemplate} placeholder="https://api.openai.com/v1/videos/{{job_id}}" />
                <Field label="Status path" value={generationStatusPath} onChange={setGenerationStatusPath} placeholder="status" />
                <Field label="Content URL template ({{job_id}})" value={generationContentUrlTemplate} onChange={setGenerationContentUrlTemplate} placeholder="https://api.openai.com/v1/videos/{{job_id}}/content" />
                <Field label="Poll interval (ms)" value={generationPollIntervalMs} onChange={setGenerationPollIntervalMs} placeholder="3000" />
                <Field label="Max wait (seconds)" value={generationMaxWaitSeconds} onChange={setGenerationMaxWaitSeconds} placeholder="600" />
                <p className="text-[11px] text-gray-400">
                  Mesh polls the status URL until it reports success, then downloads the media from the
                  content URL and returns it base64-encoded. Response media path is ignored in this mode.
                </p>
              </div>
            )}
            <Field label="Generation usage: input tokens path" value={generationUsageInputTokensPath} onChange={setGenerationUsageInputTokensPath} placeholder="usage.input_tokens" />
            <Field label="Generation usage: output tokens path" value={generationUsageOutputTokensPath} onChange={setGenerationUsageOutputTokensPath} placeholder="usage.output_tokens" />
            <Field label="Generated duration path (per-second billing)" value={generationDurationSecondsPath} onChange={setGenerationDurationSecondsPath} placeholder="seconds" />
            <Field
              label="Generation response media path"
              value={generationResponseMediaPath}
              onChange={setGenerationResponseMediaPath}
              placeholder="data.0.b64_json"
            />
          </div>
        )}
      </div>

      <div className="border-t border-gray-100 pt-2.5">
        <label className="flex items-center gap-1.5 text-xs font-medium">
          <input type="checkbox" checked={logRequests} onChange={(e) => setLogRequests(e.target.checked)} />
          Log requests to this provider
        </label>
        <p className="text-[11px] text-gray-400 mt-1">
          On for every real provider — the logs are the only record that a billable call happened.
          Turn it off only for a dummy/test upstream whose traffic would otherwise pollute a real
          user&apos;s history. Rate limits and budget caps keep applying either way, and a request
          rejected before it resolves a provider is always logged.
        </p>
      </div>

      <p className="text-[11px] text-gray-400">
        Saving re-verifies every existing model under this provider against the connection above before writing anything.
      </p>

      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={verifying}
          className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50 flex-1"
        >
          {verifying ? "Verifying with provider..." : "Save"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-gray-300 px-4 py-1.5 text-xs font-medium hover:bg-gray-50"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
