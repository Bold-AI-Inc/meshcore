"use client";

import Link from "next/link";
import { useState, type FormEvent } from "react";
import { createProviderSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { createProvider, ApiError, type CreateProviderResult, type ModelDraft, type ProviderFamily } from "@/lib/api";
import { PROVIDER_PRESETS } from "@/lib/providerPresets";
import PasswordField from "@/components/PasswordField";
import { Field, MiniField, TextArea } from "@/components/FormFields";

const PROVIDER_FAMILIES: { value: ProviderFamily; label: string }[] = [
  { value: "generic", label: "Generic (text only)" },
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "ollama", label: "Ollama / OpenAI-compatible local" },
];

interface CreateProviderFormProps {
  onCreated: (result: CreateProviderResult) => void;
}

const DEFAULTS = {
  httpMethod: "POST",
  headerTemplate: '{\n  "x-api-key": "{{api_key}}"\n}',
  requestBodyTemplate:
    '{\n  "model": "{{model}}",\n  "messages": [{"role": "user", "content": "{{query}}"}],\n  "stream": true\n}',
  responseDeltaPath: "choices.0.delta.content",
  usageInputTokensPath: "",
  usageOutputTokensPath: "",
  maxOutboundRps: "10",
  maxConcurrentUpstream: "50",
  maxRetries: "2",
  retryBackoffMs: "100",
  maxCallsPerHour: "",
};

const EMPTY_MODEL = { name: "", resolvedModel: "", inputPrice: "0", outputPrice: "0" };

export default function CreateProviderForm({ onCreated }: CreateProviderFormProps) {
  const [name, setName] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [endpointUrl, setEndpointUrl] = useState("");
  const [httpMethod, setHttpMethod] = useState(DEFAULTS.httpMethod);
  const [headerTemplate, setHeaderTemplate] = useState(DEFAULTS.headerTemplate);
  const [requestBodyTemplate, setRequestBodyTemplate] = useState(DEFAULTS.requestBodyTemplate);
  const [responseDeltaPath, setResponseDeltaPath] = useState(DEFAULTS.responseDeltaPath);
  const [usageInputTokensPath, setUsageInputTokensPath] = useState(DEFAULTS.usageInputTokensPath);
  const [usageOutputTokensPath, setUsageOutputTokensPath] = useState(DEFAULTS.usageOutputTokensPath);
  const [maxOutboundRps, setMaxOutboundRps] = useState(DEFAULTS.maxOutboundRps);
  const [maxConcurrentUpstream, setMaxConcurrentUpstream] = useState(DEFAULTS.maxConcurrentUpstream);
  const [maxRetries, setMaxRetries] = useState(DEFAULTS.maxRetries);
  const [retryBackoffMs, setRetryBackoffMs] = useState(DEFAULTS.retryBackoffMs);
  const [maxCallsPerHour, setMaxCallsPerHour] = useState(DEFAULTS.maxCallsPerHour);
  const [providerFamily, setProviderFamily] = useState<ProviderFamily>("generic");
  const [supportsEmbeddings, setSupportsEmbeddings] = useState(false);
  const [embeddingEndpointUrl, setEmbeddingEndpointUrl] = useState("");
  const [embeddingRequestBodyTemplate, setEmbeddingRequestBodyTemplate] = useState("");
  const [embeddingResponseVectorPath, setEmbeddingResponseVectorPath] = useState("");
  const [generationMode, setGenerationMode] = useState<"sync" | "async_poll">("sync");
  const [generationJobIdPath, setGenerationJobIdPath] = useState("");
  const [generationStatusUrlTemplate, setGenerationStatusUrlTemplate] = useState("");
  const [generationStatusPath, setGenerationStatusPath] = useState("");
  const [generationContentUrlTemplate, setGenerationContentUrlTemplate] = useState("");
  const [generationPollIntervalMs, setGenerationPollIntervalMs] = useState("3000");
  const [generationMaxWaitSeconds, setGenerationMaxWaitSeconds] = useState("600");
  const [embeddingUsageTokensPath, setEmbeddingUsageTokensPath] = useState("");
  const [generationUsageInputTokensPath, setGenerationUsageInputTokensPath] = useState("");
  const [generationUsageOutputTokensPath, setGenerationUsageOutputTokensPath] = useState("");
  const [generationDurationSecondsPath, setGenerationDurationSecondsPath] = useState("");
  const [supportsGeneration, setSupportsGeneration] = useState(false);
  const [generationEndpointUrl, setGenerationEndpointUrl] = useState("");
  const [generationRequestBodyTemplate, setGenerationRequestBodyTemplate] = useState("");
  const [generationResponseMediaPath, setGenerationResponseMediaPath] = useState("");
  const [supportsWebSearch, setSupportsWebSearch] = useState(false);
  const [webSearchRequestBodyTemplate, setWebSearchRequestBodyTemplate] = useState("");
  const [webSearchPricePerCall, setWebSearchPricePerCall] = useState("0");
  const [logRequests, setLogRequests] = useState(true);
  const [models, setModels] = useState([{ ...EMPTY_MODEL }]);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [modelErrors, setModelErrors] = useState<Record<number, string>>({});
  const [formError, setFormError] = useState("");
  const [verifying, setVerifying] = useState(false);

  function updateModel(index: number, patch: Partial<(typeof models)[number]>) {
    setModels((prev) => prev.map((m, i) => (i === index ? { ...m, ...patch } : m)));
  }

  function addModel() {
    setModels((prev) => [...prev, { ...EMPTY_MODEL }]);
  }

  function removeModel(index: number) {
    setModels((prev) => prev.filter((_, i) => i !== index));
  }

  function applyPreset(key: keyof typeof PROVIDER_PRESETS) {
    const preset = PROVIDER_PRESETS[key];
    setName(preset.label);
    setEndpointUrl(preset.endpointUrl);
    setHttpMethod(preset.httpMethod);
    setHeaderTemplate(preset.headerTemplate);
    setRequestBodyTemplate(preset.requestBodyTemplate);
    setResponseDeltaPath(preset.responseDeltaPath);
    setUsageInputTokensPath(preset.usageInputTokensPath);
    setUsageOutputTokensPath(preset.usageOutputTokensPath);
    setProviderFamily(preset.providerFamily);
    setSupportsWebSearch(Boolean(preset.webSearchRequestBodyTemplate));
    setWebSearchRequestBodyTemplate(preset.webSearchRequestBodyTemplate ?? "");
    setWebSearchPricePerCall(String(preset.webSearchPricePerCall ?? 0));
    setModels([{ name: preset.model.name, resolvedModel: preset.model.resolvedModel, inputPrice: "0", outputPrice: "0" }]);
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});
    setModelErrors({});

    const parsedModels: ModelDraft[] = models.map((m) => ({
      name: m.name,
      resolvedModel: m.resolvedModel,
      inputPrice: Number(m.inputPrice),
      outputPrice: Number(m.outputPrice),
    }));

    const result = createProviderSchema.safeParse({
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
      models: parsedModels,
    });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setVerifying(true);
    try {
      const created = await createProvider({
        ...result.data,
        maxCallsPerHour: maxCallsPerHour.trim() === "" ? null : Number(maxCallsPerHour) || null,
        responseDoneSignal: null,
        models: parsedModels,
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
      onCreated(created);
    } catch (err) {
      if (err instanceof ApiError && err.message === "model_verification_failed") {
        const failures = (err.details as { failures?: { model: string; reason: string }[] })?.failures ?? [];
        const byIndex: Record<number, string> = {};
        for (const failure of failures) {
          const index = models.findIndex((m) => m.name === failure.model);
          if (index >= 0) byIndex[index] = failure.reason;
        }
        setModelErrors(byIndex);
        setFormError("One or more models did not respond — nothing was saved.");
      } else if (err instanceof ApiError && err.message === "provider_or_model_already_exists") {
        setFormError("A provider or model with this name already exists");
      } else if (err instanceof ApiError) {
        setFormError(`Could not create provider (${err.message})`);
      } else {
        setFormError("Could not reach the backend — check it's running and reachable");
      }
    } finally {
      setVerifying(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="thin-scroll space-y-2.5 max-h-[75vh] overflow-y-auto pr-2">
      <div className="flex items-center justify-between">
        <div className="flex gap-1.5">
          {Object.entries(PROVIDER_PRESETS).map(([key, preset]) => (
            <button
              key={key}
              type="button"
              onClick={() => applyPreset(key as keyof typeof PROVIDER_PRESETS)}
              className="rounded-md border border-gray-300 px-2.5 py-1 text-xs hover:bg-gray-50"
            >
              {preset.label}
            </button>
          ))}
        </div>
        <Link href="/admin/docs" target="_blank" className="text-xs text-gray-400 hover:text-gray-600 underline">
          Docs ↗
        </Link>
      </div>

      <Field label="Name" value={name} onChange={setName} error={fieldErrors.name} />
      <PasswordField id="apiKey" label="API key" value={apiKey} onChange={setApiKey} error={fieldErrors.apiKey} />
      <Field
        label="Endpoint URL"
        value={endpointUrl}
        onChange={setEndpointUrl}
        error={fieldErrors.endpointUrl}
        placeholder="https://api.anthropic.com/v1/messages"
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
        <p className="text-[11px] text-gray-400 mt-1">
          Selects how Mesh renders image/document/audio/video content into this provider&apos;s own
          shape for the <code>{"{{content_json}}"}</code> placeholder. Leave &quot;Generic&quot; for a
          text-only integration — <code>{"{{query}}"}</code> keeps working exactly as before.
        </p>
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
            <p className="text-[11px] text-gray-400">
              Add embedding models separately from the Models tab after saving this provider (kind =
              &quot;embedding&quot;) — they&apos;re billed and access-controlled the same way as chat
              models, just callable via <code>/v1/embeddings</code> instead of <code>/v1/proxy</code>.
            </p>
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
              This is the same endpoint and headers as the normal chat call — only the body differs,
              because each provider names its own opt-in field (Anthropic <code>tools[]</code>, OpenAI{" "}
              <code>web_search_options</code>, Gemini <code>google_search</code>). Mesh extracts the
              cited sources out of the response stream and returns them as <code>sources</code>. Tick{" "}
              <b>Supports web search</b> on each model that can actually use it — on OpenAI that means a{" "}
              <code>*-search-preview</code> resolved model. Anthropic bills $10 per 1,000 searches
              (<code>0.01</code> above); providers that fold it into their token price should leave it 0.
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
            <p className="text-[11px] text-gray-400">
              Add image/video generation models separately from the Models tab after saving this
              provider (Kind = &quot;Image generation&quot; or &quot;Video generation&quot;, with an{" "}
              <b>Output media type</b> like <code>image/png</code> or <code>video/mp4</code>) — callable
              via <code>/v1/generate</code>. Mesh never stores the generated file; it returns it inline
              (base64 or a provider URL) for the caller to save.
            </p>
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

      <div className="border-t border-gray-100 pt-2.5">
        <div className="flex items-center justify-between mb-2">
          <p className="text-xs font-medium">Models</p>
          <button type="button" onClick={addModel} className="text-xs text-gray-500 hover:text-gray-700 underline">
            + Add another model
          </button>
        </div>
        {fieldErrors.models && <p className="text-xs text-red-600 mb-2">{fieldErrors.models}</p>}
        <div className="space-y-2">
          {models.map((model, index) => (
            <div key={index} className="rounded-md border border-gray-200 p-2.5 space-y-1.5">
              <div className="flex items-center justify-between">
                <p className="text-xs text-gray-400">Model {index + 1}</p>
                {models.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeModel(index)}
                    className="text-xs text-gray-400 hover:text-red-600"
                  >
                    Remove
                  </button>
                )}
              </div>
              <div className="grid grid-cols-2 gap-2">
                <MiniField
                  label="Display name"
                  value={model.name}
                  onChange={(v) => updateModel(index, { name: v })}
                  placeholder="mistral-large"
                />
                <MiniField
                  label="Resolved model id (sent to API)"
                  value={model.resolvedModel}
                  onChange={(v) => updateModel(index, { resolvedModel: v })}
                  placeholder="mistral-large-latest"
                />
                <MiniField
                  label="Input $/1k tokens"
                  value={model.inputPrice}
                  onChange={(v) => updateModel(index, { inputPrice: v })}
                />
                <MiniField
                  label="Output $/1k tokens"
                  value={model.outputPrice}
                  onChange={(v) => updateModel(index, { outputPrice: v })}
                />
              </div>
              {modelErrors[index] && <p className="text-xs text-red-600">{modelErrors[index]}</p>}
            </div>
          ))}
        </div>
      </div>

      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <button
        type="submit"
        disabled={verifying}
        className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50 w-full"
      >
        {verifying ? "Verifying with provider..." : "Save"}
      </button>
    </form>
  );
}
