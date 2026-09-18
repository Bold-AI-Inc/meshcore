"use client";

import { useState, type FormEvent } from "react";
import { createModelSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { createModel, ApiError, type ModelListItem, type ModelKind, type Provider } from "@/lib/api";

interface CreateModelFormProps {
  providers: Provider[];
  onCreated: (model: ModelListItem) => void;
}

export default function CreateModelForm({ providers, onCreated }: CreateModelFormProps) {
  const [providerId, setProviderId] = useState(providers[0]?.id ?? "");
  const [kind, setKind] = useState<ModelKind>("chat");
  const [name, setName] = useState("");
  const [resolvedModel, setResolvedModel] = useState("");
  const [inputPrice, setInputPrice] = useState("0");
  const [outputPrice, setOutputPrice] = useState("0");
  const [supportsImageIn, setSupportsImageIn] = useState(false);
  const [supportsDocumentIn, setSupportsDocumentIn] = useState(false);
  const [supportsAudioIn, setSupportsAudioIn] = useState(false);
  const [supportsVideoIn, setSupportsVideoIn] = useState(false);
  const [pricePerSecond, setPricePerSecond] = useState("0");
  const [supportsWebSearch, setSupportsWebSearch] = useState(false);
  const [outputMediaType, setOutputMediaType] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});

    const result = createModelSchema.safeParse({
      providerId,
      name,
      resolvedModel,
      inputPrice,
      outputPrice,
    });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setLoading(true);
    try {
      const created = await createModel(
        result.data.providerId,
        result.data.name,
        result.data.resolvedModel,
        result.data.inputPrice,
        result.data.outputPrice,
        { supportsImageIn, supportsDocumentIn, supportsAudioIn, supportsVideoIn, supportsWebSearch, pricePerSecond: Number(pricePerSecond) || 0 },
        kind,
        outputMediaType || undefined,
      );
      onCreated(created);
    } catch (err) {
      if (err instanceof ApiError && err.message === "model_verification_failed") {
        const failures = (err.details as { failures?: { reason: string }[] })?.failures ?? [];
        setFormError(failures[0]?.reason ?? "The model did not respond to a test call");
      } else if (err instanceof ApiError && err.message === "model_already_exists") {
        setFormError("This model name already exists for the selected provider");
      } else {
        setFormError("Could not create model");
      }
    } finally {
      setLoading(false);
    }
  }

  if (providers.length === 0) {
    return <p className="text-xs text-gray-400">Create a provider first.</p>;
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div>
        <label className="block text-xs font-medium mb-1">Provider</label>
        <select
          value={providerId}
          onChange={(event) => setProviderId(event.target.value)}
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        >
          {providers.map((provider) => (
            <option key={provider.id} value={provider.id}>
              {provider.name}
            </option>
          ))}
        </select>
        {fieldErrors.providerId && (
          <p className="text-xs text-red-600 mt-1">{fieldErrors.providerId}</p>
        )}
      </div>
      <div>
        <label className="block text-xs font-medium mb-1">Kind</label>
        <select
          value={kind}
          onChange={(event) => setKind(event.target.value as ModelKind)}
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        >
          <option value="chat">Chat (callable via /v1/proxy)</option>
          <option value="embedding">Embedding (callable via /v1/embeddings)</option>
          <option value="image_generation">Image generation (callable via /v1/generate)</option>
          <option value="video_generation">Video generation (callable via /v1/generate)</option>
        </select>
        {kind === "embedding" && (
          <p className="text-[11px] text-gray-400 mt-1">
            The provider must have embeddings configured (endpoint, body template, response vector
            path) — set that up from the provider&apos;s edit form first.
          </p>
        )}
        {(kind === "image_generation" || kind === "video_generation") && (
          <p className="text-[11px] text-gray-400 mt-1">
            The provider must have generation configured (endpoint, body template, response media
            path) — set that up from the provider&apos;s edit form first.
          </p>
        )}
      </div>
      {kind === "video_generation" && (
        <div>
          <label className="block text-xs font-medium mb-1">Price $/second of video</label>
          <input
            value={pricePerSecond}
            onChange={(event) => setPricePerSecond(event.target.value)}
            placeholder="0.10"
            className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
          />
          <p className="text-[11px] text-gray-400 mt-1">
            Video is billed per second, not per token — Sora reports no token count at all, so the
            token prices above stay 0 and this is what counts against the caller&apos;s daily cap.
            sora-2 is $0.10/s; sora-2-pro is $0.30/s at 720p, $0.50 at 1024p, $0.70 at 1080p.
          </p>
        </div>
      )}
      {(kind === "image_generation" || kind === "video_generation") && (
        <div>
          <label className="block text-xs font-medium mb-1">Output media type</label>
          <input
            value={outputMediaType}
            onChange={(event) => setOutputMediaType(event.target.value)}
            placeholder={kind === "image_generation" ? "image/png" : "video/mp4"}
            className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
          />
          <p className="text-[11px] text-gray-400 mt-1">
            The fixed MIME type this model produces — returned to callers in every /v1/generate
            response so they know how to save the file.
          </p>
        </div>
      )}
      <div>
        <label className="block text-xs font-medium mb-1">Name (client-facing)</label>
        <input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder="claude-4-6"
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        />
        {fieldErrors.name && <p className="text-xs text-red-600 mt-1">{fieldErrors.name}</p>}
      </div>
      <div>
        <label className="block text-xs font-medium mb-1">Resolved upstream model id</label>
        <input
          value={resolvedModel}
          onChange={(event) => setResolvedModel(event.target.value)}
          placeholder="claude-opus-4-6-20260301"
          className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
        />
        {fieldErrors.resolvedModel && (
          <p className="text-xs text-red-600 mt-1">{fieldErrors.resolvedModel}</p>
        )}
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium mb-1">Input $/1k tokens</label>
          <input
            value={inputPrice}
            onChange={(event) => setInputPrice(event.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
          />
          {fieldErrors.inputPrice && (
            <p className="text-xs text-red-600 mt-1">{fieldErrors.inputPrice}</p>
          )}
        </div>
        <div>
          <label className="block text-xs font-medium mb-1">Output $/1k tokens</label>
          <input
            value={outputPrice}
            onChange={(event) => setOutputPrice(event.target.value)}
            className="w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm outline-none focus:border-gray-500"
          />
          {fieldErrors.outputPrice && (
            <p className="text-xs text-red-600 mt-1">{fieldErrors.outputPrice}</p>
          )}
        </div>
      </div>
      <div className={kind !== "chat" ? "hidden" : undefined}>
        <label className="block text-xs font-medium mb-1">Accepts, beyond text (input)</label>
        <div className="flex flex-wrap gap-3">
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsImageIn} onChange={(e) => setSupportsImageIn(e.target.checked)} />
            Image
          </label>
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsDocumentIn} onChange={(e) => setSupportsDocumentIn(e.target.checked)} />
            Document (PDF)
          </label>
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsAudioIn} onChange={(e) => setSupportsAudioIn(e.target.checked)} />
            Audio
          </label>
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsVideoIn} onChange={(e) => setSupportsVideoIn(e.target.checked)} />
            Video
          </label>
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsWebSearch} onChange={(e) => setSupportsWebSearch(e.target.checked)} />
            Web search
          </label>
        </div>
        <p className="text-[11px] text-gray-400 mt-1">
          A request sending a content type not checked here is rejected before any call to the provider.
          Web search additionally requires the provider itself to have a web search body template
          configured — on OpenAI it only works on a <code>*-search-preview</code> resolved model.
        </p>
      </div>
      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <button
        type="submit"
        disabled={loading}
        className="bg-black text-white rounded-md px-4 py-1.5 text-xs font-medium disabled:opacity-50 w-full"
      >
        {loading ? "Creating..." : "Create model"}
      </button>
    </form>
  );
}
