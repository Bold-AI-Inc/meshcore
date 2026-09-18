"use client";

import { useEffect, useMemo, useState } from "react";
import {
  grantAccess,
  listAccessForUser,
  listModels,
  listPolicies,
  revokeAccess,
  upsertPolicy,
  type ModelListItem,
  type ModelPolicy,
} from "@/lib/api";
import { ApiError } from "@/lib/api";
import { isTestProviderName } from "@/lib/testProviders";
import { policySchema } from "@/lib/validation";

interface ModelAccessPanelProps {
  userId: string;
}

const DAYS = [
  { value: 1, label: "M", name: "Monday" },
  { value: 2, label: "T", name: "Tuesday" },
  { value: 3, label: "W", name: "Wednesday" },
  { value: 4, label: "T", name: "Thursday" },
  { value: 5, label: "F", name: "Friday" },
  { value: 6, label: "S", name: "Saturday" },
  { value: 0, label: "S", name: "Sunday" },
];

const DEFAULT_POLICY: Omit<ModelPolicy, "user_id" | "model_id" | "spent_today_usd" | "calls_this_hour"> = {
  daily_cap_usd: 20,
  allowed_from: null,
  allowed_to: null,
  timezone: "UTC",
  active_days: [1, 2, 3, 4, 5],
  always_open: false,
  max_calls_per_hour: null,
};

const WEEKDAYS = [1, 2, 3, 4, 5];
const WEEKEND = [0, 6];

function dayLabel(activeDays: number[]): string {
  const sorted = [...activeDays].sort();
  const isSet = (values: number[]) =>
    sorted.length === values.length && values.every((v) => sorted.includes(v));
  if (isSet([0, 1, 2, 3, 4, 5, 6])) return "Every day";
  if (isSet(WEEKDAYS)) return "Weekdays";
  if (isSet(WEEKEND)) return "Weekends";
  if (sorted.length === 0) return "No days selected";
  return DAYS.filter((d) => activeDays.includes(d.value))
    .map((d) => d.name.slice(0, 3))
    .join(", ");
}

function formatTime(value: string): string {
  const [h, m] = value.split(":").map(Number);
  const period = h >= 12 ? "PM" : "AM";
  const hour12 = h % 12 === 0 ? 12 : h % 12;
  return `${hour12}:${String(m).padStart(2, "0")} ${period}`;
}

function ScheduleSummary({
  policy,
  spentTodayUsd,
  callsThisHour,
}: {
  policy: Omit<ModelPolicy, "user_id" | "model_id" | "spent_today_usd" | "calls_this_hour">;
  spentTodayUsd: number;
  callsThisHour: number;
}) {
  return (
    <span className="inline-flex items-center gap-1.5 flex-wrap">
      <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
        ${spentTodayUsd.toFixed(2)} / {policy.daily_cap_usd > 0 ? `$${policy.daily_cap_usd}/day` : "unlimited"}
      </span>
      {policy.max_calls_per_hour != null && (
        <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600">
          {callsThisHour} / {policy.max_calls_per_hour} calls/hr
        </span>
      )}
      {policy.always_open ? (
        <span className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-0.5 text-[11px] font-medium text-green-700">
          Always open
        </span>
      ) : !policy.allowed_from || !policy.allowed_to ? (
        <span className="text-gray-400">No schedule set</span>
      ) : (
        <>
          <span className="text-gray-600">{dayLabel(policy.active_days)}</span>
          <span className="text-gray-300">&middot;</span>
          <span className="font-medium text-gray-700">
            {formatTime(policy.allowed_from)}&ndash;{formatTime(policy.allowed_to)}
          </span>
          <span className="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500">
            {policy.timezone}
          </span>
        </>
      )}
    </span>
  );
}

export default function ModelAccessPanel({ userId }: ModelAccessPanelProps) {
  const [models, setModels] = useState<ModelListItem[]>([]);
  const [granted, setGranted] = useState<Set<string>>(new Set());
  const [drafts, setDrafts] = useState<Record<string, Omit<ModelPolicy, "user_id" | "model_id" | "spent_today_usd" | "calls_this_hour">>>({});
  const [spentToday, setSpentToday] = useState<Record<string, number>>({});
  const [callsThisHour, setCallsThisHour] = useState<Record<string, number>>({});
  const [loadedUserId, setLoadedUserId] = useState<string | null>(null);
  const [pendingAccess, setPendingAccess] = useState<Set<string>>(new Set());
  const [openScheduleFor, setOpenScheduleFor] = useState<string | null>(null);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [savedId, setSavedId] = useState<string | null>(null);
  const [errorId, setErrorId] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [warning, setWarning] = useState<{ modelId: string; message: string } | null>(null);
  const loading = loadedUserId !== userId;

  useEffect(() => {
    let cancelled = false;
    Promise.all([listModels(), listAccessForUser(userId), listPolicies(userId)])
      .then(([allModels, grants, policies]) => {
        if (cancelled) return;
        setModels(allModels);
        setGranted(new Set(grants.map((g) => g.model_id)));
        const byModel: Record<string, Omit<ModelPolicy, "user_id" | "model_id" | "spent_today_usd" | "calls_this_hour">> = {};
        for (const model of allModels) {
          const existing = policies.find((p) => p.model_id === model.id);
          byModel[model.id] = existing
            ? {
                daily_cap_usd: existing.daily_cap_usd,
                allowed_from: existing.allowed_from,
                allowed_to: existing.allowed_to,
                timezone: existing.timezone,
                active_days: existing.active_days,
                always_open: existing.always_open,
                max_calls_per_hour: existing.max_calls_per_hour,
              }
            : { ...DEFAULT_POLICY };
        }
        setDrafts(byModel);
        const bySpend: Record<string, number> = {};
        const byCalls: Record<string, number> = {};
        for (const p of policies) {
          bySpend[p.model_id] = p.spent_today_usd;
          byCalls[p.model_id] = p.calls_this_hour;
        }
        setSpentToday(bySpend);
        setCallsThisHour(byCalls);
        setLoadedUserId(userId);
      })
      .catch(() => {
        if (!cancelled) setLoadedUserId(userId);
      });
    return () => {
      cancelled = true;
    };
  }, [userId]);

  const grouped = useMemo(() => {
    const byProvider = new Map<string, ModelListItem[]>();
    for (const model of models) {
      const list = byProvider.get(model.provider_name) ?? [];
      list.push(model);
      byProvider.set(model.provider_name, list);
    }
    return byProvider;
  }, [models]);

  // A user holding any test-model grant is a test user. That single fact
  // decides which half of the catalogue is worth showing them: a test user
  // sees the dummy providers, everyone else sees the real ones. The other
  // half is never callable for them anyway (access is deny-by-default in
  // the gateway), so listing it is noise that only invites a misclick.
  const isTestUser = useMemo(
    () => models.some((m) => isTestProviderName(m.provider_name) && granted.has(m.id)),
    [models, granted],
  );

  // ...with one exception that outranks the above: an already-granted model
  // is always shown, whichever side it's on. Hiding a live grant would make
  // it impossible to revoke from the UI, which is a worse failure than a
  // slightly noisy list.
  const visibleGroups = useMemo(() => {
    const out: [string, ModelListItem[]][] = [];
    for (const [providerName, providerModels] of grouped.entries()) {
      const belongs = isTestProviderName(providerName) === isTestUser;
      const visible = providerModels.filter((m) => belongs || granted.has(m.id));
      if (visible.length > 0) out.push([providerName, visible]);
    }
    return out;
  }, [grouped, isTestUser, granted]);

  async function toggleAccess(modelId: string, isGranted: boolean) {
    setPendingAccess((prev) => new Set(prev).add(modelId));
    try {
      if (isGranted) {
        await revokeAccess(userId, modelId);
        setGranted((prev) => {
          const next = new Set(prev);
          next.delete(modelId);
          return next;
        });
        if (openScheduleFor === modelId) setOpenScheduleFor(null);
      } else {
        await grantAccess(userId, modelId);
        setGranted((prev) => new Set(prev).add(modelId));
      }
    } finally {
      setPendingAccess((prev) => {
        const next = new Set(prev);
        next.delete(modelId);
        return next;
      });
    }
  }

  function updateDraft(modelId: string, patch: Partial<Omit<ModelPolicy, "user_id" | "model_id" | "spent_today_usd" | "calls_this_hour">>) {
    setDrafts((prev) => ({ ...prev, [modelId]: { ...prev[modelId], ...patch } }));
  }

  function toggleDay(modelId: string, day: number) {
    const current = drafts[modelId]?.active_days ?? [];
    updateDraft(modelId, {
      active_days: current.includes(day) ? current.filter((d) => d !== day) : [...current, day],
    });
  }

  async function saveSchedule(modelId: string) {
    const draft = drafts[modelId];
    setErrorId(null);
    setErrorMessage(null);
    setWarning((prev) => (prev?.modelId === modelId ? null : prev));
    const result = policySchema.safeParse({
      dailyCapUsd: draft.daily_cap_usd,
      allowedFrom: draft.allowed_from ?? undefined,
      allowedTo: draft.allowed_to ?? undefined,
      timezone: draft.timezone,
      activeDays: draft.active_days,
      alwaysOpen: draft.always_open,
    });
    if (!result.success) {
      setErrorId(modelId);
      setErrorMessage("Check the values above");
      return;
    }
    setSavingId(modelId);
    try {
      const res = await upsertPolicy(userId, modelId, draft);
      setSavedId(modelId);
      // The policy saved either way; a warning means the timezone could not
      // be resolved and GMT will be enforced instead. Shown until dismissed
      // rather than auto-hidden, since it changes what the schedule means.
      setWarning(res?.warning ? { modelId, message: res.warning } : null);
      setTimeout(() => setSavedId((prev) => (prev === modelId ? null : prev)), 1500);
    } catch (err) {
      setErrorId(modelId);
      if (err instanceof ApiError && err.message === "invalid_request") {
        setErrorMessage("Check the values above");
      } else if (err instanceof ApiError) {
        setErrorMessage(`Save failed: ${err.message}. Try logging out and back in.`);
      } else {
        setErrorMessage("Could not reach the server. Check your connection and try again.");
      }
    } finally {
      setSavingId(null);
    }
  }

  const browserTimezone = useMemo(() => Intl.DateTimeFormat().resolvedOptions().timeZone, []);

  if (loading) return <p className="text-xs text-gray-400">Loading...</p>;
  if (models.length === 0) return <p className="text-xs text-gray-400">No models exist yet.</p>;

  const renderGroup = ([providerName, providerModels]: [string, ModelListItem[]]) => (
        <div key={providerName}>
          <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400 mb-1.5">
            {providerName}
          </p>
          <div className="rounded-md border border-gray-200 divide-y divide-gray-100 overflow-hidden">
            {providerModels.map((model) => {
              const isGranted = granted.has(model.id);
              const isPendingAccess = pendingAccess.has(model.id);
              const draft = drafts[model.id] ?? DEFAULT_POLICY;
              const isOpen = openScheduleFor === model.id;
              const disabled = model.blocked || !isGranted;

              return (
                <div key={model.id} className={model.blocked ? "bg-gray-50" : "bg-white"}>
                  <div className="flex items-center gap-3 px-3 py-2 text-xs">
                    <input
                      type="checkbox"
                      checked={isGranted}
                      disabled={isPendingAccess || model.blocked}
                      onChange={() => toggleAccess(model.id, isGranted)}
                      className="accent-black shrink-0"
                    />
                    <span className={`w-32 shrink-0 truncate ${model.blocked ? "text-gray-300" : "text-gray-900"}`}>
                      {model.name}
                      {model.blocked && <span className="ml-1.5 text-gray-300">(blocked)</span>}
                    </span>
                    <div className="flex-1 min-w-0">
                      {disabled ? (
                        <span className="text-gray-300">&mdash;</span>
                      ) : (
                        <ScheduleSummary
                          policy={draft}
                          spentTodayUsd={spentToday[model.id] ?? 0}
                          callsThisHour={callsThisHour[model.id] ?? 0}
                        />
                      )}
                    </div>
                    <div className="w-14 shrink-0 text-right">
                      {savedId === model.id && <span className="text-[11px] text-green-600">Saved</span>}
                      {errorId === model.id && <span className="text-[11px] text-red-600">Error</span>}
                    </div>
                    <button
                      type="button"
                      disabled={disabled}
                      onClick={() => setOpenScheduleFor(isOpen ? null : model.id)}
                      className={`shrink-0 rounded-md border px-2.5 py-1 text-[11px] font-medium transition-colors ${
                        disabled
                          ? "border-gray-100 text-gray-300"
                          : isOpen
                            ? "border-black bg-black text-white"
                            : "border-gray-300 text-gray-600 hover:border-gray-400"
                      }`}
                    >
                      {isOpen ? "Close" : "Edit"}
                    </button>
                  </div>

                  {isOpen && !disabled && (
                    <div className="border-t border-gray-100 bg-gray-50 px-3 py-2.5 space-y-2 text-xs">
                      <div className="flex items-center flex-wrap gap-x-5 gap-y-2">
                        <label className="flex items-center gap-1.5">
                          <span className="text-gray-400">$</span>
                          <input
                            type="text"
                            inputMode="decimal"
                            value={draft.daily_cap_usd || ""}
                            placeholder="unlimited"
                            onChange={(e) =>
                              updateDraft(model.id, { daily_cap_usd: Number(e.target.value) || 0 })
                            }
                            className="w-20 rounded-md border border-gray-300 px-1.5 py-1 text-xs outline-none focus:border-gray-900"
                          />
                          <span className="text-gray-400">/day</span>
                        </label>

                        <label className="flex items-center gap-1.5">
                          <input
                            type="text"
                            inputMode="numeric"
                            placeholder="unlimited"
                            value={draft.max_calls_per_hour ?? ""}
                            onChange={(e) => {
                              const raw = e.target.value.trim();
                              updateDraft(model.id, { max_calls_per_hour: raw === "" ? null : Number(raw) || null });
                            }}
                            className="w-16 rounded-md border border-gray-300 px-1.5 py-1 text-xs outline-none focus:border-gray-900"
                          />
                          <span className="text-gray-400">calls/hr (blank = unlimited)</span>
                        </label>

                        <button
                          type="button"
                          role="switch"
                          aria-checked={draft.always_open}
                          onClick={() => updateDraft(model.id, { always_open: !draft.always_open })}
                          className={`rounded-full border px-2 py-0.5 text-[10px] font-medium transition-colors ${
                            draft.always_open
                              ? "border-black bg-black text-white"
                              : "border-gray-300 text-gray-500 hover:border-gray-400"
                          }`}
                        >
                          Always open
                        </button>

                        {!draft.always_open && (
                          <div className="flex items-center gap-1.5">
                            <input
                              type="time"
                              value={draft.allowed_from ?? ""}
                              onChange={(e) => updateDraft(model.id, { allowed_from: e.target.value })}
                              className="rounded-md border border-gray-300 px-1.5 py-1 text-xs outline-none focus:border-gray-900"
                            />
                            <span className="text-gray-300">&mdash;</span>
                            <input
                              type="time"
                              value={draft.allowed_to ?? ""}
                              onChange={(e) => updateDraft(model.id, { allowed_to: e.target.value })}
                              className="rounded-md border border-gray-300 px-1.5 py-1 text-xs outline-none focus:border-gray-900"
                            />
                          </div>
                        )}
                      </div>

                      {!draft.always_open && (
                        <div className="flex gap-1">
                          {DAYS.map((d) => (
                            <button
                              key={d.value}
                              type="button"
                              title={d.name}
                              onClick={() => toggleDay(model.id, d.value)}
                              className={`h-5 w-5 rounded-full text-[9px] font-medium transition-colors ${
                                draft.active_days.includes(d.value)
                                  ? "bg-black text-white"
                                  : "bg-white border border-gray-200 text-gray-400 hover:border-gray-300"
                              }`}
                            >
                              {d.label}
                            </button>
                          ))}
                        </div>
                      )}

                      <div className="flex items-center flex-wrap gap-1.5">
                        <input
                          value={draft.timezone}
                          onChange={(e) => updateDraft(model.id, { timezone: e.target.value })}
                          placeholder="UTC"
                          className="w-36 rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-900"
                        />
                        <button
                          type="button"
                          onClick={() => updateDraft(model.id, { timezone: "UTC" })}
                          className={`rounded-md border px-2 py-1 text-[11px] font-medium transition-colors ${
                            draft.timezone === "UTC"
                              ? "border-black bg-black text-white"
                              : "border-gray-200 text-gray-500 hover:border-gray-300"
                          }`}
                        >
                          UTC
                        </button>
                        <button
                          type="button"
                          onClick={() => updateDraft(model.id, { timezone: browserTimezone })}
                          className={`rounded-md border px-2 py-1 text-[11px] font-medium transition-colors ${
                            draft.timezone === browserTimezone
                              ? "border-black bg-black text-white"
                              : "border-gray-200 text-gray-500 hover:border-gray-300"
                          }`}
                        >
                          This device
                        </button>

                        <button
                          type="button"
                          onClick={() => saveSchedule(model.id)}
                          disabled={savingId === model.id}
                          className="ml-auto rounded-md bg-black text-white px-3 py-1 text-[11px] font-medium hover:bg-gray-800 transition-colors disabled:opacity-50"
                        >
                          {savingId === model.id ? "Saving..." : "Save"}
                        </button>
                      </div>
                      {errorId === model.id && errorMessage && (
                        <p className="text-[11px] text-red-600">{errorMessage}</p>
                      )}
                      {warning?.modelId === model.id && (
                        <p className="text-[11px] text-amber-700 bg-amber-50 rounded px-2 py-1.5">
                          {warning.message}
                        </p>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>
  );

  return (
    <div className="space-y-4">
      {visibleGroups.map(renderGroup)}
    </div>
  );
}
