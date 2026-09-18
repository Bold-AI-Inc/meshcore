"use client";

import { useState, type FormEvent } from "react";
import { modelDraftSchema } from "@/lib/validation";
import { fieldErrorsFrom } from "@/lib/zodErrors";
import { updateModel, ApiError, type ModelListItem } from "@/lib/api";
import { MiniField } from "@/components/FormFields";

interface EditModelFormProps {
  model: ModelListItem;
  onSaved: () => void;
  onCancel: () => void;
}

export default function EditModelForm({ model, onSaved, onCancel }: EditModelFormProps) {
  const [name, setName] = useState(model.name);
  const [resolvedModel, setResolvedModel] = useState(model.resolved_model);
  const [inputPrice, setInputPrice] = useState(String(model.input_price_per_1k_tokens));
  const [outputPrice, setOutputPrice] = useState(String(model.output_price_per_1k_tokens));
  const [blocked, setBlocked] = useState(model.blocked);
  const [supportsImageIn, setSupportsImageIn] = useState(model.supports_image_in);
  const [supportsDocumentIn, setSupportsDocumentIn] = useState(model.supports_document_in);
  const [supportsAudioIn, setSupportsAudioIn] = useState(model.supports_audio_in);
  const [supportsVideoIn, setSupportsVideoIn] = useState(model.supports_video_in);
  const [pricePerSecond, setPricePerSecond] = useState(String(model.price_per_second ?? 0));
  const [supportsWebSearch, setSupportsWebSearch] = useState(model.supports_web_search);
  const [outputMediaType, setOutputMediaType] = useState(model.output_media_type ?? "");
  const isGeneration = model.kind === "image_generation" || model.kind === "video_generation";
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState("");
  const [verifying, setVerifying] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setFormError("");
    setFieldErrors({});

    const result = modelDraftSchema.safeParse({ name, resolvedModel, inputPrice, outputPrice });
    if (!result.success) {
      setFieldErrors(fieldErrorsFrom(result.error));
      return;
    }

    setVerifying(true);
    try {
      await updateModel(
        model.id,
        result.data.name,
        result.data.resolvedModel,
        result.data.inputPrice,
        result.data.outputPrice,
        blocked,
        { supportsImageIn, supportsDocumentIn, supportsAudioIn, supportsVideoIn, supportsWebSearch, pricePerSecond: Number(pricePerSecond) || 0 },
        outputMediaType || undefined,
      );
      onSaved();
    } catch (err) {
      if (err instanceof ApiError && err.message === "model_verification_failed") {
        const failures = (err.details as { failures?: { reason: string }[] })?.failures ?? [];
        setFormError(failures[0]?.reason ?? "The model did not respond to a test call — nothing was saved.");
      } else if (err instanceof ApiError && err.message === "model_already_exists") {
        setFormError("This model name already exists for this provider");
      } else {
        setFormError("Could not save model");
      }
    } finally {
      setVerifying(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="rounded-md border border-gray-200 p-2.5 space-y-1.5 bg-gray-50">
      <div className="grid grid-cols-2 gap-2">
        <MiniField label="Display name" value={name} onChange={setName} />
        <MiniField label="Resolved model id" value={resolvedModel} onChange={setResolvedModel} />
        <MiniField label="Input $/1k tokens" value={inputPrice} onChange={setInputPrice} />
        <MiniField label="Output $/1k tokens" value={outputPrice} onChange={setOutputPrice} />
      </div>
      {fieldErrors.name && <p className="text-xs text-red-600">{fieldErrors.name}</p>}
      {fieldErrors.resolvedModel && <p className="text-xs text-red-600">{fieldErrors.resolvedModel}</p>}
      <label className="flex items-center gap-1.5 text-xs text-gray-600">
        <input type="checkbox" checked={blocked} onChange={(e) => setBlocked(e.target.checked)} />
        Blocked
      </label>
      {isGeneration ? (
        <div>
          <label className="block text-[11px] text-gray-500 mb-0.5">Output media type</label>
          <input
            value={outputMediaType}
            onChange={(e) => setOutputMediaType(e.target.value)}
            placeholder={model.kind === "image_generation" ? "image/png" : "video/mp4"}
            className="w-full rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-500"
          />
        </div>
      ) : (
        <div className="flex flex-wrap gap-3">
      {model.kind === "video_generation" && (
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
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsImageIn} onChange={(e) => setSupportsImageIn(e.target.checked)} />
            Image
          </label>
          <label className="flex items-center gap-1.5 text-xs text-gray-600">
            <input type="checkbox" checked={supportsDocumentIn} onChange={(e) => setSupportsDocumentIn(e.target.checked)} />
            Document
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
      )}
      {formError && <p className="text-xs text-red-600">{formError}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={verifying}
          className="bg-black text-white rounded-md px-3 py-1 text-xs font-medium disabled:opacity-50"
        >
          {verifying ? "Verifying..." : "Save"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-gray-300 px-3 py-1 text-xs font-medium hover:bg-white"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
