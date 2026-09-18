"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { getAllLogsSummary, type LogsSummary } from "@/lib/api";
import { RANGE_PRESETS, useTimeRange } from "@/hooks/useTimeRange";
import TestTrafficToggle from "@/components/TestTrafficToggle";
import {
  BarList,
  Card,
  DANGER,
  Donut,
  GOOD,
  Histogram,
  SERIES,
  StackedBar,
  StatTile,
  TimeSeriesChart,
  WARN,
  formatMs,
  formatNumber,
  formatUSD,
  type SeriesPoint,
} from "@/components/charts/Charts";

function statusColor(code: number): string {
  if (code < 300) return GOOD;
  if (code < 400) return SERIES[2];
  if (code < 500) return WARN;
  return DANGER;
}

function bucketLabel(iso: string, hourly: boolean): string {
  const d = new Date(iso);
  return hourly
    ? d.toLocaleTimeString(undefined, { hour: "numeric" })
    : d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export default function DashboardPage() {
  const router = useRouter();
  const { preset, setPreset, range, hourly } = useTimeRange("7d");
  const [includeTest, setIncludeTest] = useState(false);
  const requestKey = `${range.from}|${range.to}|${includeTest}`;
  const [result, setResult] = useState<{ key: string; summary: LogsSummary | null; failed: boolean } | null>(null);
  const fresh = result?.key === requestKey;
  const summary = fresh ? result.summary : (result?.summary ?? null);
  const loading = !fresh;
  const error = fresh && result.failed;
  // A range switch keeps the previous numbers on screen while the new ones
  // load. Without this the click looks inert, especially when two ranges
  // happen to return the same totals.
  const refreshing = loading && summary !== null;

  useEffect(() => {
    let cancelled = false;
    getAllLogsSummary({ from: range.from, to: range.to, includeTest })
      .then((data) => {
        if (!cancelled) setResult({ key: requestKey, summary: data, failed: false });
      })
      .catch(() => {
        if (!cancelled) setResult({ key: requestKey, summary: null, failed: true });
      });
    return () => {
      cancelled = true;
    };
  }, [range, includeTest, requestKey]);

  // Every drill-in lands on the logs explorer with the same time range the
  // dashboard is showing, so the numbers line up with what you clicked.
  const drill = useCallback(
    (params: Record<string, string>) => {
      const q = new URLSearchParams({ preset, ...params });
      if (includeTest) q.set("include_test", "true");
      router.push(`/admin/logs?${q.toString()}`);
    },
    [preset, includeTest, router],
  );

  const points: SeriesPoint[] = useMemo(
    () =>
      (summary?.time_series ?? []).map((b) => ({
        label: bucketLabel(b.bucket, hourly),
        calls: b.calls,
        errors: b.errors,
        avgMs: b.avg_latency_ms,
      })),
    [summary, hourly],
  );

  const totalCost = (summary?.total_input_cost ?? 0) + (summary?.total_output_cost ?? 0);
  const totalCalls = summary?.total_calls ?? 0;
  const errorCalls = summary?.error_calls ?? 0;
  const deniedCalls = summary?.denied_calls ?? 0;
  const successCalls = Math.max(totalCalls - errorCalls - deniedCalls, 0);
  const errorRate = totalCalls > 0 ? (errorCalls / totalCalls) * 100 : 0;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between flex-wrap gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h1 className="text-lg font-serif leading-tight">Dashboard</h1>
          <p className="text-[11px] text-gray-400 leading-tight">
            Traffic, spend and reliability across every user.
            {!includeTest && " Test traffic excluded."}
            {refreshing && <span className="ml-1 text-gray-500">Updating…</span>}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <TestTrafficToggle checked={includeTest} onChange={setIncludeTest} />
          <div aria-busy={refreshing} className="flex gap-0.5 bg-white rounded-md shadow-sm p-0.5">
            {RANGE_PRESETS.map((p) => (
              <button
                key={p.value}
                type="button"
                onClick={() => setPreset(p.value)}
                className={`rounded px-2.5 py-1 text-xs font-medium ${
                  preset === p.value ? "bg-black text-white" : "text-gray-600 hover:bg-gray-50"
                }`}
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {error ? (
        <div className="bg-white rounded-lg shadow-sm p-4 text-center">
          <p className="text-xs text-red-600">Could not load dashboard data.</p>
        </div>
      ) : loading && !summary ? (
        <div className="bg-white rounded-lg shadow-sm p-4 text-center">
          <p className="text-xs text-gray-400">Loading...</p>
        </div>
      ) : (
        <div className={`space-y-3 transition-opacity ${refreshing ? "opacity-50" : "opacity-100"}`}>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
            <StatTile compact label="Calls" value={formatNumber(totalCalls)} onClick={() => drill({})} />
            <StatTile
              compact
              label="Spend"
              value={formatUSD(totalCost)}
              hint={`in / out ${formatUSD(summary?.total_input_cost ?? 0)} / ${formatUSD(
                summary?.total_output_cost ?? 0,
              )}`}
            />
            <StatTile
              compact
              label="Error rate"
              value={`${errorRate.toFixed(1)}%`}
              hint={`${formatNumber(errorCalls)} calls`}
              tone={errorRate > 5 ? "danger" : "good"}
              onClick={() => drill({ status: "error" })}
            />
            <StatTile
              compact
              label="Denied"
              value={formatNumber(deniedCalls)}
              hint="policy or auth rejections"
              tone={deniedCalls > 0 ? "danger" : "default"}
              onClick={() => drill({ status: "denied" })}
            />
            <StatTile
              compact
              label="Avg latency"
              value={formatMs(summary?.avg_latency_ms ?? 0)}
              hint={`${formatNumber((summary?.total_tokens_in ?? 0) + (summary?.total_tokens_out ?? 0))} tokens`}
            />
          </div>

          {/* Traffic is the headline, so it keeps two thirds of the row and the
              status mix rides alongside it instead of pushing everything down. */}
          <div className="grid lg:grid-cols-3 gap-2">
            <Card
              compact
              className="lg:col-span-2"
              title="Traffic over time"
              subtitle="Calls, errors and average latency per bucket"
            >
              <TimeSeriesChart points={points} height={180} />
            </Card>

            <Card compact title="Status codes" subtitle="HTTP status returned to callers">
              <Donut
                size={112}
                slices={(summary?.by_status ?? []).map((s) => ({
                  label: String(s.status_code),
                  value: s.calls,
                  color: statusColor(s.status_code),
                }))}
              />
            </Card>
          </div>

          <div className="grid lg:grid-cols-3 gap-2">
            <Card compact title="Outcome mix" subtitle="Where every call ended up">
              <StackedBar
                segments={[
                  { label: "Success", value: successCalls, color: GOOD },
                  { label: "Upstream error", value: errorCalls, color: DANGER },
                  { label: "Denied", value: deniedCalls, color: WARN },
                ]}
              />
            </Card>

            <Card compact title="Latency distribution" subtitle="How call latency is spread">
              <Histogram
                height={120}
                buckets={(summary?.latency_buckets ?? []).map((b) => ({ label: b.label, calls: b.calls }))}
              />
            </Card>

            <Card compact title="Denial reasons" subtitle="Why calls were rejected upfront">
              {(summary?.by_deny_reason ?? []).length === 0 ? (
                <div className="h-[120px] flex items-center justify-center">
                  <p className="text-xs text-gray-400">No denials in this range</p>
                </div>
              ) : (
                <BarList
                  data={(summary?.by_deny_reason ?? []).slice(0, 6).map((d) => ({
                    label: d.reason.replace(/_/g, " "),
                    value: d.calls,
                    onClick: () => drill({ deny_reason: d.reason }),
                  }))}
                />
              )}
            </Card>
          </div>

          <div className="grid lg:grid-cols-3 gap-2">
            <Card compact title="Top models" subtitle="By call volume">
              <BarList
                data={(summary?.by_model ?? []).slice(0, 8).map((m) => ({
                  label: m.model_name,
                  sublabel: m.provider_name,
                  value: m.calls,
                  onClick: m.model_id ? () => drill({ model_id: m.model_id }) : undefined,
                }))}
              />
            </Card>

            <Card compact title="Top users" subtitle="By call volume">
              <BarList
                data={(summary?.by_user ?? []).slice(0, 8).map((u) => ({
                  label: u.user_email,
                  sublabel: formatUSD(u.cost_usd),
                  value: u.calls,
                  onClick: u.user_id ? () => drill({ user_id: u.user_id }) : undefined,
                }))}
              />
            </Card>

            <Card compact title="Spend by model" subtitle="Input + output cost">
              <BarList
                data={(summary?.by_model ?? [])
                  .filter((m) => m.cost_usd > 0)
                  .sort((a, b) => b.cost_usd - a.cost_usd)
                  .slice(0, 8)
                  .map((m) => ({
                    label: m.model_name,
                    sublabel: m.provider_name,
                    value: m.cost_usd,
                    onClick: m.model_id ? () => drill({ model_id: m.model_id }) : undefined,
                  }))}
                valueFormat={formatUSD}
              />
            </Card>
          </div>

          <Card compact title="Model detail" subtitle="Volume, reliability, tokens and cost per model">
            {(summary?.by_model ?? []).length === 0 ? (
              <p className="text-xs text-gray-400">No calls in this range</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="text-left text-gray-400 border-b border-gray-100">
                      <th className="py-1.5 font-medium">Model</th>
                      <th className="py-1.5 font-medium">Provider</th>
                      <th className="py-1.5 font-medium text-right">Calls</th>
                      <th className="py-1.5 font-medium text-right">Errors</th>
                      <th className="py-1.5 font-medium text-right">Avg latency</th>
                      <th className="py-1.5 font-medium text-right">Tokens in</th>
                      <th className="py-1.5 font-medium text-right">Tokens out</th>
                      <th className="py-1.5 font-medium text-right">Cost</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(summary?.by_model ?? []).map((m) => (
                      <tr
                        key={`${m.model_id}-${m.model_name}`}
                        onClick={m.model_id ? () => drill({ model_id: m.model_id }) : undefined}
                        className={`border-b border-gray-50 last:border-0 ${
                          m.model_id ? "hover:bg-gray-50 cursor-pointer" : ""
                        }`}
                      >
                        <td className="py-1.5">{m.model_name}</td>
                        <td className="py-1.5 text-gray-500">{m.provider_name}</td>
                        <td className="py-1.5 text-right tabular-nums">{formatNumber(m.calls)}</td>
                        <td
                          className={`py-1.5 text-right tabular-nums ${m.errors > 0 ? "text-red-600" : "text-gray-400"}`}
                        >
                          {formatNumber(m.errors)}
                        </td>
                        <td className="py-1.5 text-right tabular-nums">{formatMs(m.avg_latency_ms)}</td>
                        <td className="py-1.5 text-right tabular-nums">{formatNumber(m.tokens_in)}</td>
                        <td className="py-1.5 text-right tabular-nums">{formatNumber(m.tokens_out)}</td>
                        <td className="py-1.5 text-right tabular-nums">{formatUSD(m.cost_usd)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>

          <Card compact title="User detail" subtitle="Volume, reliability and spend per user">
            {(summary?.by_user ?? []).length === 0 ? (
              <p className="text-xs text-gray-400">No calls in this range</p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-xs">
                  <thead>
                    <tr className="text-left text-gray-400 border-b border-gray-100">
                      <th className="py-1.5 font-medium">User</th>
                      <th className="py-1.5 font-medium text-right">Calls</th>
                      <th className="py-1.5 font-medium text-right">Errors</th>
                      <th className="py-1.5 font-medium text-right">Error rate</th>
                      <th className="py-1.5 font-medium text-right">Spend</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(summary?.by_user ?? []).map((u) => (
                      <tr
                        key={`${u.user_id}-${u.user_email}`}
                        onClick={u.user_id ? () => drill({ user_id: u.user_id }) : undefined}
                        className={`border-b border-gray-50 last:border-0 ${
                          u.user_id ? "hover:bg-gray-50 cursor-pointer" : ""
                        }`}
                      >
                        <td className="py-1.5">{u.user_email}</td>
                        <td className="py-1.5 text-right tabular-nums">{formatNumber(u.calls)}</td>
                        <td
                          className={`py-1.5 text-right tabular-nums ${u.errors > 0 ? "text-red-600" : "text-gray-400"}`}
                        >
                          {formatNumber(u.errors)}
                        </td>
                        <td className="py-1.5 text-right tabular-nums">
                          {u.calls > 0 ? `${((u.errors / u.calls) * 100).toFixed(1)}%` : "—"}
                        </td>
                        <td className="py-1.5 text-right tabular-nums">{formatUSD(u.cost_usd)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>
      )}
    </div>
  );
}
