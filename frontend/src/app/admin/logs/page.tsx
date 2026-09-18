"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import {
  listAllLogs,
  listModels,
  listUsers,
  type ModelListItem,
  type RequestLog,
  type UserListItem,
} from "@/lib/api";
import { RANGE_PRESETS, useTimeRange, type RangePreset } from "@/hooks/useTimeRange";
import TestTrafficToggle from "@/components/TestTrafficToggle";
import { isTestUserEmail } from "@/lib/testProviders";
import Pagination from "@/components/Pagination";
import SearchInput from "@/components/SearchInput";
import { formatMs, formatUSD } from "@/components/charts/Charts";

const PAGE_SIZES = [25, 50, 100];

const STATUS_OPTIONS = [
  { value: "", label: "All outcomes" },
  { value: "success", label: "Success" },
  { value: "error", label: "Errors + denials" },
  { value: "denied", label: "Denied only" },
];

function outcomeClass(log: RequestLog): string {
  if (log.outcome === "denied") return "text-amber-700 bg-amber-50";
  if (log.outcome === "upstream_error" || log.status_code >= 400) return "text-red-700 bg-red-50";
  return "text-green-700 bg-green-50";
}

function LogsExplorer() {
  const params = useSearchParams();

  const initialPreset = (params.get("preset") as RangePreset) ?? "7d";
  const { preset, setPreset, range } = useTimeRange(
    RANGE_PRESETS.some((p) => p.value === initialPreset) ? initialPreset : "7d",
  );

  const [filters, setFilters] = useState({
    userId: params.get("user_id") ?? "",
    modelId: params.get("model_id") ?? "",
    status: params.get("status") ?? "",
    denyReason: params.get("deny_reason") ?? "",
    search: "",
    includeTest: params.get("include_test") === "true",
    pageSize: 25,
  });
  const { userId, modelId, status, denyReason, search, includeTest, pageSize } = filters;
  const setFilter = (patch: Partial<typeof filters>) => setFilters((prev) => ({ ...prev, ...patch }));

  const [page, setPage] = useState(1);
  const [sortBy, setSortBy] = useState("created_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [users, setUsers] = useState<UserListItem[]>([]);
  const [models, setModels] = useState<ModelListItem[]>([]);
  const [expanded, setExpanded] = useState<string | null>(null);

  useEffect(() => {
    listUsers().then(setUsers).catch(() => setUsers([]));
    listModels().then(setModels).catch(() => setModels([]));
  }, []);

  const requestKey = JSON.stringify([
    range, userId, modelId, status, denyReason, search, includeTest, page, pageSize, sortBy, sortDir,
  ]);
  const [result, setResult] = useState<{
    key: string;
    logs: RequestLog[];
    total: number;
    failed: boolean;
  } | null>(null);
  const fresh = result?.key === requestKey;
  const logs = result?.logs ?? [];
  const total = result?.total ?? 0;
  const loading = !fresh;
  const error = fresh && result.failed;

  useEffect(() => {
    let cancelled = false;
    listAllLogs({
      from: range.from,
      to: range.to,
      userId: userId || undefined,
      modelId: modelId || undefined,
      status: status || undefined,
      denyReason: denyReason || undefined,
      search: search || undefined,
      includeTest: includeTest || undefined,
      page,
      pageSize,
      sortBy,
      sortDir,
    })
      .then((res) => {
        if (!cancelled) {
          setResult({ key: requestKey, logs: res.logs ?? [], total: res.total ?? 0, failed: false });
        }
      })
      .catch(() => {
        if (!cancelled) setResult({ key: requestKey, logs: [], total: 0, failed: true });
      });
    return () => {
      cancelled = true;
    };
  }, [requestKey, range, userId, modelId, status, denyReason, search, includeTest, page, pageSize, sortBy, sortDir]);

  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const activeFilters = [userId, modelId, status, denyReason, search].filter(Boolean).length;

  const sortHeader = (key: string, label: string, align = "left") => (
    <th
      className={`py-2 font-medium cursor-pointer select-none hover:text-gray-600 ${
        align === "right" ? "text-right" : "text-left"
      }`}
      onClick={() => {
        if (sortBy === key) {
          setSortDir(sortDir === "asc" ? "desc" : "asc");
        } else {
          setSortBy(key);
          setSortDir("desc");
        }
      }}
    >
      {label}
      {sortBy === key && <span className="ml-1">{sortDir === "asc" ? "↑" : "↓"}</span>}
    </th>
  );

  const clearAll = () => {
    setFilter({ userId: "", modelId: "", status: "", denyReason: "", search: "" });
    setPage(1);
  };

  const selectClass = "border border-gray-200 rounded-md px-2 py-1.5 text-xs bg-white";

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div>
          <h1 className="text-lg font-serif">Logs</h1>
          <p className="text-xs text-gray-400 mt-0.5">
            Every request across every user.{!includeTest && " Test traffic excluded."}
          </p>
        </div>
        <div className="flex gap-1 bg-white rounded-md shadow-sm p-0.5">
          {RANGE_PRESETS.map((p) => (
            <button
              key={p.value}
              type="button"
              onClick={() => setPreset(p.value)}
              className={`rounded px-3 py-1.5 text-xs font-medium ${
                preset === p.value ? "bg-black text-white" : "text-gray-600 hover:bg-gray-50"
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      <div className="bg-white rounded-lg shadow-sm p-4">
        <div className="flex flex-wrap items-center gap-2 mb-3">
          <select className={selectClass} value={userId} onChange={(e) => { setFilter({ userId: e.target.value }); setPage(1); }}>
            <option value="">All users</option>
            {/* Test accounts are only offered when test traffic is shown —
                picking one otherwise would filter to a set the server has
                already excluded, giving an empty table with no explanation.
                The currently-selected user always stays listed so a deep
                link never renders a blank dropdown. */}
            {users
              .filter((u) => includeTest || !isTestUserEmail(u.email) || u.id === userId)
              .map((u) => (
                <option key={u.id} value={u.id}>
                  {u.email}
                </option>
              ))}
          </select>

          <select className={selectClass} value={modelId} onChange={(e) => { setFilter({ modelId: e.target.value }); setPage(1); }}>
            <option value="">All models</option>
            {models.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </select>

          <select className={selectClass} value={status} onChange={(e) => { setFilter({ status: e.target.value }); setPage(1); }}>
            {STATUS_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>

          {denyReason && (
            <button
              type="button"
              onClick={() => { setFilter({ denyReason: "" }); setPage(1); }}
              className="text-xs bg-amber-50 text-amber-800 rounded-md px-2 py-1.5 hover:bg-amber-100"
            >
              {denyReason.replace(/_/g, " ")} ✕
            </button>
          )}

          <div className="ml-auto flex items-center gap-3">
            <TestTrafficToggle
              checked={includeTest}
              onChange={(v) => {
                setFilter({ includeTest: v });
                setPage(1);
              }}
            />
            {activeFilters > 0 && (
              <button type="button" onClick={clearAll} className="text-xs text-gray-500 hover:text-gray-900 underline">
                Clear filters
              </button>
            )}
            <SearchInput
              value={search}
              onChange={(v) => { setFilter({ search: v }); setPage(1); }}
              placeholder="Search user, model, IP..."
            />
          </div>
        </div>

        <div className="flex items-center justify-between mb-3">
          <p className="text-xs text-gray-400">
            {loading ? "Loading..." : `${total.toLocaleString()} request${total === 1 ? "" : "s"}`}
          </p>
          <select
            className={selectClass}
            value={pageSize}
            onChange={(e) => { setFilter({ pageSize: Number(e.target.value) }); setPage(1); }}
          >
            {PAGE_SIZES.map((n) => (
              <option key={n} value={n}>
                {n} per page
              </option>
            ))}
          </select>
        </div>

        {error ? (
          <p className="text-xs text-red-600">Could not load logs.</p>
        ) : logs.length === 0 && !loading ? (
          <p className="text-xs text-gray-400">No requests match these filters</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-gray-400 border-b border-gray-100">
                  {sortHeader("created_at", "Time")}
                  <th className="py-2 font-medium text-left">User</th>
                  <th className="py-2 font-medium text-left">Model</th>
                  <th className="py-2 font-medium text-left">Outcome</th>
                  {sortHeader("status_code", "Status", "right")}
                  {sortHeader("latency_ms", "Latency", "right")}
                  {sortHeader("tokens_in", "In", "right")}
                  {sortHeader("tokens_out", "Out", "right")}
                  <th className="py-2 font-medium text-right">Cost</th>
                </tr>
              </thead>
              <tbody>
                {logs.map((log) => (
                  <tr
                    key={log.id}
                    onClick={() => setExpanded(expanded === log.id ? null : log.id)}
                    className="border-b border-gray-50 last:border-0 hover:bg-gray-50 cursor-pointer align-top"
                  >
                    <td className="py-2.5 whitespace-nowrap">
                      {new Date(log.created_at).toLocaleString()}
                      {expanded === log.id && (
                        <div className="mt-2 space-y-0.5 text-[11px] text-gray-500 font-normal">
                          <p>ID: {log.id}</p>
                          {log.source_ip && <p>IP: {log.source_ip}</p>}
                          {log.browser && (
                            <p>
                              {log.browser} {log.browser_version} on {log.os} {log.os_version}
                            </p>
                          )}
                          {log.device_type && <p>Device: {log.device_type}</p>}
                          {log.user_agent && <p className="break-all">UA: {log.user_agent}</p>}
                        </div>
                      )}
                    </td>
                    <td className="py-2.5">{log.user_email ?? "—"}</td>
                    <td className="py-2.5">
                      {log.model_name ?? log.requested_model ?? "—"}
                      {log.provider_name && <span className="text-gray-400 ml-1.5">{log.provider_name}</span>}
                    </td>
                    <td className="py-2.5">
                      <span className={`rounded px-1.5 py-0.5 ${outcomeClass(log)}`}>
                        {log.deny_reason ? log.deny_reason.replace(/_/g, " ") : log.outcome.replace(/_/g, " ")}
                      </span>
                    </td>
                    <td className="py-2.5 text-right tabular-nums">{log.status_code}</td>
                    <td className="py-2.5 text-right tabular-nums">{formatMs(log.latency_ms)}</td>
                    <td className="py-2.5 text-right tabular-nums">{log.tokens_in.toLocaleString()}</td>
                    <td className="py-2.5 text-right tabular-nums">{log.tokens_out.toLocaleString()}</td>
                    <td className="py-2.5 text-right tabular-nums">{formatUSD(log.input_cost + log.output_cost)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <Pagination page={page} totalPages={totalPages} onChange={setPage} />
      </div>
    </div>
  );
}

// useSearchParams client-side renders the tree up to the nearest Suspense
// boundary, so the page provides one rather than opting the whole route out
// of prerendering.
export default function LogsPage() {
  return (
    <Suspense fallback={<p className="text-xs text-gray-400">Loading...</p>}>
      <LogsExplorer />
    </Suspense>
  );
}
