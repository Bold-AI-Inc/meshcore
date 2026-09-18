"use client";

import { useEffect, useMemo, useState } from "react";
import {
  getLogsSummary,
  listLogs,
  listModels,
  type LogsModelBreakdown,
  type LogsStatusBreakdown,
  type LogsSummary,
  type LogsTimeBucket,
  type ModelListItem,
  type RequestLog,
} from "@/lib/api";

interface LogsDashboardProps {
  userId: string;
  userLabel: string;
  onClose: () => void;
}

type Preset = "today" | "7d" | "30d" | "90d" | "custom";

const PRESETS: { value: Preset; label: string }[] = [
  { value: "today", label: "Today" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "custom", label: "Custom" },
];

const PAGE_SIZES = [25, 50, 100];

function toInputDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

function rangeForPreset(preset: Preset, customFrom: string, customTo: string): { from: string; to: string } {
  const now = new Date();
  const to = now.toISOString();
  if (preset === "custom") {
    const from = customFrom ? new Date(`${customFrom}T00:00:00Z`).toISOString() : to;
    const toEnd = customTo ? new Date(`${customTo}T23:59:59Z`).toISOString() : to;
    return { from, to: toEnd };
  }
  const days = preset === "today" ? 1 : preset === "7d" ? 7 : preset === "30d" ? 30 : 90;
  const from = new Date(now);
  from.setUTCDate(from.getUTCDate() - days);
  return { from: from.toISOString(), to };
}

function statusColor(code: number): string {
  if (code < 300) return "text-green-700 bg-green-50";
  if (code < 500) return "text-amber-700 bg-amber-50";
  return "text-red-700 bg-red-50";
}

function formatBucketLabel(iso: string, hourly: boolean): string {
  const d = new Date(iso);
  return hourly
    ? d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })
    : d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function TimeSeriesChart({ series, hourly }: { series: LogsTimeBucket[]; hourly: boolean }) {
  if (series.length === 0) {
    return <p className="text-xs text-gray-400 py-8 text-center">No data in this range</p>;
  }
  const max = Math.max(...series.map((b) => b.calls), 1);
  return (
    <div className="flex items-end gap-1 h-40 overflow-x-auto pb-1">
      {series.map((b) => {
        const height = Math.max((b.calls / max) * 100, 2);
        const errorHeight = b.calls > 0 ? (b.errors / b.calls) * height : 0;
        return (
          <div key={b.bucket} className="flex flex-col items-center gap-1 shrink-0 min-w-[42px] px-0.5">
            <div
              className="w-full rounded-t-sm bg-gray-900 relative"
              style={{ height: `${height}%`, minHeight: 2 }}
              title={`${formatBucketLabel(b.bucket, hourly)} — ${b.calls} calls, ${b.errors} errors, avg ${Math.round(b.avg_latency_ms)}ms`}
            >
              {errorHeight > 0 && (
                <div className="absolute bottom-0 left-0 right-0 bg-red-500 rounded-b-sm" style={{ height: `${errorHeight}%` }} />
              )}
            </div>
            <span className="text-[9px] text-gray-400 rotate-0 whitespace-nowrap">
              {formatBucketLabel(b.bucket, hourly)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function BreakdownBars<T>({
  items,
  labelOf,
  valueOf,
  keyOf,
}: {
  items: T[];
  labelOf: (item: T) => string;
  valueOf: (item: T) => number;
  keyOf: (item: T) => string;
}) {
  if (items.length === 0) {
    return <p className="text-xs text-gray-400 py-4">No data</p>;
  }
  const max = Math.max(...items.map(valueOf), 1);
  return (
    <div className="space-y-1.5">
      {items.map((item) => (
        <div key={keyOf(item)} className="flex items-center gap-2 text-xs">
          <span className="w-24 shrink-0 truncate text-gray-600">{labelOf(item)}</span>
          <div className="flex-1 h-3 bg-gray-100 rounded-sm overflow-hidden">
            <div className="h-full bg-gray-900 rounded-sm" style={{ width: `${(valueOf(item) / max) * 100}%` }} />
          </div>
          <span className="w-10 shrink-0 text-right font-mono text-gray-500">{valueOf(item)}</span>
        </div>
      ))}
    </div>
  );
}

function StatTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="border border-gray-800 rounded-md px-3 py-2.5 bg-black text-white">
      <p className="text-[10px] uppercase tracking-wide text-gray-400">{label}</p>
      <p className="text-lg font-semibold font-mono">{value}</p>
    </div>
  );
}

const SORT_COLUMNS: { key: string; label: string }[] = [
  { key: "created_at", label: "Time" },
  { key: "status_code", label: "Status" },
  { key: "latency_ms", label: "Latency" },
  { key: "tokens_in", label: "Tokens in" },
  { key: "tokens_out", label: "Tokens out" },
];

export default function LogsDashboard({ userId, userLabel, onClose }: LogsDashboardProps) {
  const [models, setModels] = useState<ModelListItem[]>([]);
  const [preset, setPreset] = useState<Preset>("7d");
  const [customFrom, setCustomFrom] = useState(() => toInputDate(new Date(Date.now() - 7 * 86400000)));
  const [customTo, setCustomTo] = useState(() => toInputDate(new Date()));
  const [modelId, setModelId] = useState("");
  const [status, setStatus] = useState("");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [sortBy, setSortBy] = useState("created_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(25);

  const [summary, setSummary] = useState<LogsSummary | null>(null);
  const [loadedSummaryKey, setLoadedSummaryKey] = useState<string | null>(null);
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [total, setTotal] = useState(0);
  const [loadedLogsKey, setLoadedLogsKey] = useState<string | null>(null);

  const { from, to } = useMemo(() => rangeForPreset(preset, customFrom, customTo), [preset, customFrom, customTo]);
  const hourly = useMemo(() => new Date(to).getTime() - new Date(from).getTime() <= 48 * 3600 * 1000, [from, to]);

  const filterKey = JSON.stringify({ userId, from, to, modelId, status, search });
  const listKey = JSON.stringify({ userId, from, to, modelId, status, search, sortBy, sortDir, page, pageSize });
  const loadingSummary = loadedSummaryKey !== filterKey;
  const loadingLogs = loadedLogsKey !== listKey;

  const [pageResetKey, setPageResetKey] = useState(filterKey);
  if (pageResetKey !== filterKey) {
    setPageResetKey(filterKey);
    if (page !== 1) setPage(1);
  }

  useEffect(() => {
    listModels().then(setModels).catch(() => setModels([]));
  }, []);

  useEffect(() => {
    const timeout = setTimeout(() => setSearch(searchInput), 300);
    return () => clearTimeout(timeout);
  }, [searchInput]);

  useEffect(() => {
    let cancelled = false;
    getLogsSummary(userId, { from, to, modelId: modelId || undefined, status: status || undefined, search: search || undefined })
      .then((data) => {
        if (cancelled) return;
        setSummary(data);
        setLoadedSummaryKey(filterKey);
      })
      .catch(() => {
        if (cancelled) return;
        setSummary(null);
        setLoadedSummaryKey(filterKey);
      });
    return () => {
      cancelled = true;
    };
  }, [filterKey, userId, from, to, modelId, status, search]);

  useEffect(() => {
    let cancelled = false;
    listLogs(userId, {
      from,
      to,
      modelId: modelId || undefined,
      status: status || undefined,
      search: search || undefined,
      sortBy,
      sortDir,
      page,
      pageSize,
    })
      .then((res) => {
        if (cancelled) return;
        setLogs(res.logs);
        setTotal(res.total);
        setLoadedLogsKey(listKey);
      })
      .catch(() => {
        if (cancelled) return;
        setLogs([]);
        setTotal(0);
        setLoadedLogsKey(listKey);
      });
    return () => {
      cancelled = true;
    };
  }, [listKey, userId, from, to, modelId, status, search, sortBy, sortDir, page, pageSize]);

  function toggleSort(col: string) {
    if (sortBy === col) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortBy(col);
      setSortDir("desc");
    }
  }

  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="fixed inset-0 z-50 bg-white flex flex-col">
      <div className="bg-black text-white px-5 py-3 flex items-center justify-between shrink-0">
        <div>
          <p className="text-sm font-medium">Activity logs</p>
          <p className="text-[11px] text-gray-400">{userLabel}</p>
        </div>
        <button
          type="button"
          onClick={onClose}
          className="text-gray-400 hover:text-white text-sm rounded-md border border-gray-700 px-2.5 py-1"
        >
          Close
        </button>
      </div>

      <div className="border-b border-gray-200 px-5 py-2.5 flex flex-wrap items-center gap-2 shrink-0 bg-gray-50">
        <div className="flex rounded-md border border-gray-300 overflow-hidden">
          {PRESETS.map((p) => (
            <button
              key={p.value}
              type="button"
              onClick={() => setPreset(p.value)}
              className={`px-2.5 py-1 text-[11px] font-medium ${
                preset === p.value ? "bg-black text-white" : "bg-white text-gray-600 hover:bg-gray-100"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>

        {preset === "custom" && (
          <div className="flex items-center gap-1.5">
            <input
              type="date"
              value={customFrom}
              onChange={(e) => setCustomFrom(e.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-900"
            />
            <span className="text-gray-400 text-xs">to</span>
            <input
              type="date"
              value={customTo}
              onChange={(e) => setCustomTo(e.target.value)}
              className="rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-900"
            />
          </div>
        )}

        <select
          value={modelId}
          onChange={(e) => setModelId(e.target.value)}
          className="rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-900 bg-white"
        >
          <option value="">All models</option>
          {models.map((m) => (
            <option key={m.id} value={m.id}>
              {m.name}
            </option>
          ))}
        </select>

        <select
          value={status}
          onChange={(e) => setStatus(e.target.value)}
          className="rounded-md border border-gray-300 px-2 py-1 text-xs outline-none focus:border-gray-900 bg-white"
        >
          <option value="">All statuses</option>
          <option value="success">Success</option>
          <option value="error">Error (upstream or denied)</option>
          <option value="denied">Denied only (rejected before any provider call)</option>
        </select>

        <input
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder="Search model, location, IP, ISP, user agent..."
          className="flex-1 min-w-[200px] rounded-md border border-gray-300 px-2.5 py-1 text-xs outline-none focus:border-gray-900"
        />

        <span className="text-[11px] text-gray-400 ml-auto">{total} calls</span>
      </div>

      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {loadingSummary || !summary ? (
          <p className="text-xs text-gray-400">Loading summary...</p>
        ) : (
          <>
            <div className="grid grid-cols-2 sm:grid-cols-6 gap-2">
              <StatTile label="Total calls" value={summary.total_calls.toLocaleString()} />
              <StatTile
                label="Error rate"
                value={summary.total_calls ? `${((summary.error_calls / summary.total_calls) * 100).toFixed(1)}%` : "0%"}
              />
              <StatTile label="Avg latency" value={`${Math.round(summary.avg_latency_ms)}ms`} />
              <StatTile label="Tokens in" value={summary.total_tokens_in.toLocaleString()} />
              <StatTile label="Tokens out" value={summary.total_tokens_out.toLocaleString()} />
              <StatTile
                label="Total cost"
                value={`$${(summary.total_input_cost + summary.total_output_cost).toFixed(2)}`}
              />
            </div>

            <div className="border border-gray-200 rounded-md p-3">
              <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400 mb-2">
                Calls over time <span className="text-gray-300">(red = errors)</span>
              </p>
              <TimeSeriesChart series={loadingSummary ? [] : (summary?.time_series ?? [])} hourly={hourly} />
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              <div className="border border-gray-200 rounded-md p-3">
                <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400 mb-2">By model</p>
                <BreakdownBars<LogsModelBreakdown>
                  items={summary.by_model}
                  labelOf={(m) => m.model_name}
                  valueOf={(m) => m.calls}
                  keyOf={(m) => m.model_id}
                />
              </div>
              <div className="border border-gray-200 rounded-md p-3">
                <p className="text-[11px] font-medium uppercase tracking-wide text-gray-400 mb-2">By status</p>
                <BreakdownBars<LogsStatusBreakdown>
                  items={summary.by_status}
                  labelOf={(s) => String(s.status_code)}
                  valueOf={(s) => s.calls}
                  keyOf={(s) => String(s.status_code)}
                />
              </div>
            </div>
          </>
        )}

        <div className="border border-gray-200 rounded-md overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="bg-gray-900 text-white">
                  {SORT_COLUMNS.map((col) => (
                    <th
                      key={col.key}
                      onClick={() => toggleSort(col.key)}
                      className="text-left px-3 py-2 font-medium cursor-pointer select-none whitespace-nowrap"
                    >
                      {col.label}
                      {sortBy === col.key && <span className="ml-1 text-gray-400">{sortDir === "asc" ? "↑" : "↓"}</span>}
                    </th>
                  ))}
                  <th className="text-left px-3 py-2 font-medium whitespace-nowrap">Cost</th>
                  <th className="text-left px-3 py-2 font-medium whitespace-nowrap">Model</th>
                  <th className="text-left px-3 py-2 font-medium whitespace-nowrap">IP</th>
                  <th className="text-left px-3 py-2 font-medium whitespace-nowrap">Device</th>
                  <th className="text-left px-3 py-2 font-medium whitespace-nowrap">User agent</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-100">
                {loadingLogs ? (
                  <tr>
                    <td colSpan={10} className="px-3 py-6 text-center text-gray-400">
                      Loading...
                    </td>
                  </tr>
                ) : logs.length === 0 ? (
                  <tr>
                    <td colSpan={10} className="px-3 py-6 text-center text-gray-400">
                      No calls match these filters
                    </td>
                  </tr>
                ) : (
                  logs.map((log) => (
                    <tr key={log.id} className="hover:bg-gray-50">
                      <td className="px-3 py-1.5 font-mono text-gray-600 whitespace-nowrap">
                        {new Date(log.created_at).toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "numeric",
                          minute: "2-digit",
                        })}
                      </td>
                      <td className="px-3 py-1.5 whitespace-nowrap">
                        <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${statusColor(log.status_code)}`}>
                          {log.status_code}
                        </span>
                      </td>
                      <td className="px-3 py-1.5 font-mono text-gray-600 whitespace-nowrap">{log.latency_ms}ms</td>
                      <td className="px-3 py-1.5 font-mono text-gray-600 whitespace-nowrap">{log.tokens_in}</td>
                      <td className="px-3 py-1.5 font-mono text-gray-600 whitespace-nowrap">{log.tokens_out}</td>
                      <td className="px-3 py-1.5 font-mono text-gray-600 whitespace-nowrap">
                        ${(log.input_cost + log.output_cost).toFixed(4)}
                      </td>
                      <td className="px-3 py-1.5 text-gray-700 whitespace-nowrap">
                        {log.model_name ?? (
                          <span title={log.deny_reason ?? undefined} className="text-red-600">
                            {log.requested_model ? `${log.requested_model} (denied)` : `denied: ${log.deny_reason ?? "unknown"}`}
                          </span>
                        )}
                      </td>
                      <td className="px-3 py-1.5 font-mono text-gray-500 whitespace-nowrap">{log.source_ip ?? "—"}</td>
                      <td className="px-3 py-1.5 text-gray-500 whitespace-nowrap">
                        {[
                          [log.browser, log.browser_version].filter(Boolean).join(" "),
                          [log.os, log.os_version].filter(Boolean).join(" "),
                          log.device_type,
                        ]
                          .filter(Boolean)
                          .join(" · ") || "—"}
                      </td>
                      <td className="px-3 py-1.5 text-gray-400 max-w-[220px] truncate" title={log.user_agent ?? ""}>
                        {log.user_agent ?? "—"}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>

          <div className="flex items-center justify-between px-3 py-2 border-t border-gray-200 bg-gray-50">
            <div className="flex items-center gap-1.5 text-[11px] text-gray-500">
              <span>Rows per page</span>
              <select
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value));
                  setPage(1);
                }}
                className="rounded-md border border-gray-300 px-1.5 py-0.5 text-[11px] outline-none bg-white"
              >
                {PAGE_SIZES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex items-center gap-2 text-[11px] text-gray-500">
              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded-md border border-gray-300 px-2 py-1 disabled:opacity-40"
              >
                Prev
              </button>
              <span>
                Page {page} of {totalPages}
              </span>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                className="rounded-md border border-gray-300 px-2 py-1 disabled:opacity-40"
              >
                Next
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
