"use client";

import { type ReactNode, useId, useState } from "react";

export const INK = "#111111";
export const MUTED = "#9ca3af";
export const GRID = "#e5e7eb";
export const DANGER = "#dc2626";
export const WARN = "#f59e0b";
export const GOOD = "#10b981";

export const SERIES = ["#111111", "#6366f1", "#0ea5e9", "#10b981", "#f59e0b", "#ec4899", "#8b5cf6", "#ef4444"];

export function formatNumber(n: number): string {
  if (!Number.isFinite(n)) return "0";
  if (Math.abs(n) >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (Math.abs(n) >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(Math.round(n));
}

export function formatUSD(n: number): string {
  if (!Number.isFinite(n)) return "$0.00";
  if (n > 0 && n < 0.01) return `$${n.toFixed(4)}`;
  return `$${n.toFixed(2)}`;
}

export function formatMs(n: number): string {
  if (!Number.isFinite(n)) return "0ms";
  return n >= 1000 ? `${(n / 1000).toFixed(2)}s` : `${Math.round(n)}ms`;
}

export function Card({
  title,
  subtitle,
  action,
  children,
  className = "",
  compact = false,
}: {
  title: string;
  subtitle?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
  compact?: boolean;
}) {
  return (
    <section className={`bg-white rounded-lg shadow-sm ${compact ? "p-3" : "p-4"} ${className}`}>
      <div className={`flex items-start justify-between gap-3 ${compact ? "mb-2" : "mb-3"}`}>
        <div className="min-w-0">
          <h2 className="text-sm font-medium leading-tight">{title}</h2>
          {subtitle && <p className="text-[11px] text-gray-400 mt-0.5 leading-tight">{subtitle}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

export function StatTile({
  label,
  value,
  hint,
  tone = "default",
  onClick,
  compact = false,
}: {
  label: string;
  value: string;
  hint?: string;
  tone?: "default" | "danger" | "good";
  onClick?: () => void;
  compact?: boolean;
}) {
  const toneClass = tone === "danger" ? "text-red-700" : tone === "good" ? "text-emerald-700" : "text-gray-900";
  const pad = compact ? "p-3" : "p-4";
  const body = (
    <>
      <p className={`${compact ? "text-[10px]" : "text-[11px]"} text-gray-400 uppercase tracking-wide leading-tight`}>
        {label}
      </p>
      <p
        className={`${compact ? "text-lg mt-0.5" : "text-xl mt-1"} font-serif tabular-nums leading-tight ${toneClass}`}
      >
        {value}
      </p>
      {hint && (
        <p className={`text-[11px] text-gray-400 mt-0.5 leading-tight ${compact ? "truncate" : ""}`}>{hint}</p>
      )}
    </>
  );
  if (onClick) {
    return (
      <button
        type="button"
        onClick={onClick}
        className={`bg-white rounded-lg shadow-sm ${pad} text-left w-full cursor-pointer hover:shadow transition-shadow`}
      >
        {body}
      </button>
    );
  }
  return <div className={`bg-white rounded-lg shadow-sm ${pad}`}>{body}</div>;
}

export function EmptyChart({
  message = "No data in this range",
  height = 160,
}: {
  message?: string;
  height?: number;
}) {
  return (
    <div className="flex items-center justify-center" style={{ height }}>
      <p className="text-xs text-gray-400">{message}</p>
    </div>
  );
}

export interface SeriesPoint {
  label: string;
  calls: number;
  errors: number;
  avgMs: number;
}

export function TimeSeriesChart({ points, height = 220 }: { points: SeriesPoint[]; height?: number }) {
  const gradientId = useId();
  const [hover, setHover] = useState<number | null>(null);

  if (points.length === 0) return <EmptyChart height={height} />;

  const W = 900;
  const H = height;
  const padL = 44;
  const padR = 48;
  const padT = 12;
  const padB = 28;
  const innerW = W - padL - padR;
  const innerH = H - padT - padB;

  const maxCalls = Math.max(1, ...points.map((p) => p.calls));
  const maxMs = Math.max(1, ...points.map((p) => p.avgMs));
  const slot = innerW / Math.max(points.length, 1);

  const x = (i: number) => (points.length === 1 ? padL + innerW / 2 : padL + (i / (points.length - 1)) * innerW);
  const yCalls = (v: number) => padT + innerH - (v / maxCalls) * innerH;
  const yMs = (v: number) => padT + innerH - (v / maxMs) * innerH;

  const line = (accessor: (p: SeriesPoint) => number, scale: (v: number) => number) =>
    points.map((p, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${scale(accessor(p)).toFixed(1)}`).join(" ");

  const area = `${line((p) => p.calls, yCalls)} L${x(points.length - 1).toFixed(1)},${padT + innerH} L${x(0).toFixed(
    1,
  )},${padT + innerH} Z`;

  const ticks = [0, 0.25, 0.5, 0.75, 1];
  const labelEvery = Math.max(1, Math.ceil(points.length / 8));
  const hasErrors = points.some((p) => p.errors > 0);

  return (
    <div className="relative">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full" style={{ height }} role="img" aria-label="Calls over time">
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={INK} stopOpacity="0.16" />
            <stop offset="100%" stopColor={INK} stopOpacity="0.01" />
          </linearGradient>
        </defs>

        {ticks.map((t) => (
          <g key={t}>
            <line x1={padL} x2={W - padR} y1={padT + innerH * t} y2={padT + innerH * t} stroke={GRID} strokeWidth="1" />
            <text x={padL - 8} y={padT + innerH * t + 3} textAnchor="end" fontSize="9" fill={MUTED}>
              {formatNumber(maxCalls * (1 - t))}
            </text>
            <text x={W - padR + 8} y={padT + innerH * t + 3} textAnchor="start" fontSize="9" fill={MUTED}>
              {formatMs(maxMs * (1 - t))}
            </text>
          </g>
        ))}

        <path d={area} fill={`url(#${gradientId})`} />
        <path d={line((p) => p.calls, yCalls)} fill="none" stroke={INK} strokeWidth="1.75" />
        <path
          d={line((p) => p.avgMs, yMs)}
          fill="none"
          stroke={SERIES[2]}
          strokeWidth="1.25"
          strokeDasharray="4 3"
          opacity="0.85"
        />
        {hasErrors && <path d={line((p) => p.errors, yCalls)} fill="none" stroke={DANGER} strokeWidth="1.25" />}

        {points.map((p, i) => (
          <g key={i}>
            {i % labelEvery === 0 && (
              <text x={x(i)} y={H - 8} textAnchor="middle" fontSize="9" fill={MUTED}>
                {p.label}
              </text>
            )}
            <rect
              x={x(i) - slot / 2}
              y={padT}
              width={Math.max(slot, 4)}
              height={innerH}
              fill="transparent"
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
            />
            {hover === i && (
              <>
                <line x1={x(i)} x2={x(i)} y1={padT} y2={padT + innerH} stroke={MUTED} strokeWidth="1" />
                <circle cx={x(i)} cy={yCalls(p.calls)} r="3" fill={INK} />
              </>
            )}
          </g>
        ))}
      </svg>

      <div className="flex items-center gap-4 mt-1 text-[10px] text-gray-400">
        <span className="flex items-center gap-1">
          <span className="w-3 h-0.5 inline-block" style={{ background: INK }} /> Calls
        </span>
        <span className="flex items-center gap-1">
          <span className="w-3 h-0.5 inline-block" style={{ background: DANGER }} /> Errors
        </span>
        <span className="flex items-center gap-1">
          <span className="w-3 h-0.5 inline-block" style={{ background: SERIES[2] }} /> Avg latency
        </span>
      </div>

      {hover !== null && (
        <div className="absolute top-0 right-0 bg-white border border-gray-200 rounded-md shadow-sm px-2.5 py-1.5 text-[11px] pointer-events-none">
          <p className="font-medium">{points[hover].label}</p>
          <p className="text-gray-500">{formatNumber(points[hover].calls)} calls</p>
          <p className="text-gray-500">{formatNumber(points[hover].errors)} errors</p>
          <p className="text-gray-500">{formatMs(points[hover].avgMs)} avg</p>
        </div>
      )}
    </div>
  );
}

export interface BarDatum {
  label: string;
  value: number;
  sublabel?: string;
  onClick?: () => void;
}

export function BarList({
  data,
  valueFormat = formatNumber,
}: {
  data: BarDatum[];
  valueFormat?: (n: number) => string;
}) {
  if (data.length === 0) return <EmptyChart />;
  const max = Math.max(1, ...data.map((d) => d.value));
  return (
    <div className="space-y-1.5">
      {data.map((d, i) => (
        <div
          key={`${d.label}-${i}`}
          onClick={d.onClick}
          className={`group ${d.onClick ? "cursor-pointer" : ""}`}
        >
          <div className="flex items-baseline justify-between text-xs gap-2">
            <span className="truncate" title={d.label}>
              {d.label}
              {d.sublabel && <span className="text-gray-400 ml-1.5 text-[11px]">{d.sublabel}</span>}
            </span>
            <span className="tabular-nums text-gray-500 shrink-0">{valueFormat(d.value)}</span>
          </div>
          <div className="h-1.5 bg-gray-100 rounded-full mt-1 overflow-hidden">
            <div
              className="h-full rounded-full transition-all group-hover:opacity-80"
              style={{ width: `${(d.value / max) * 100}%`, background: SERIES[i % SERIES.length] }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}

export function Histogram({
  buckets,
  height = 160,
}: {
  buckets: { label: string; calls: number }[];
  height?: number;
}) {
  const [hover, setHover] = useState<number | null>(null);
  if (buckets.length === 0 || buckets.every((b) => b.calls === 0)) return <EmptyChart height={height} />;

  const max = Math.max(1, ...buckets.map((b) => b.calls));
  const total = buckets.reduce((sum, b) => sum + b.calls, 0);

  return (
    <div className="relative">
      <div className="flex items-end gap-1" style={{ height }}>
        {buckets.map((b, i) => (
          <div
            key={b.label}
            className="flex-1 flex flex-col justify-end h-full"
            onMouseEnter={() => setHover(i)}
            onMouseLeave={() => setHover(null)}
          >
            <div
              className="w-full rounded-t-sm"
              style={{
                height: `${Math.max((b.calls / max) * 100, b.calls > 0 ? 2 : 0)}%`,
                background: INK,
                opacity: hover === null || hover === i ? 0.85 : 0.35,
              }}
            />
          </div>
        ))}
      </div>
      <div className="flex gap-1 mt-1">
        {buckets.map((b) => (
          <div key={b.label} className="flex-1 text-center">
            <span className="text-[9px] text-gray-400 block truncate" title={b.label}>
              {b.label}
            </span>
          </div>
        ))}
      </div>
      {hover !== null && (
        <div className="absolute top-0 right-0 bg-white border border-gray-200 rounded-md shadow-sm px-2.5 py-1.5 text-[11px] pointer-events-none">
          <p className="font-medium">{buckets[hover].label}</p>
          <p className="text-gray-500">{formatNumber(buckets[hover].calls)} calls</p>
          <p className="text-gray-500">
            {total > 0 ? ((buckets[hover].calls / total) * 100).toFixed(1) : "0"}% of total
          </p>
        </div>
      )}
    </div>
  );
}

export function StackedBar({ segments }: { segments: { label: string; value: number; color: string }[] }) {
  const total = segments.reduce((sum, s) => sum + s.value, 0);
  if (total === 0) return <EmptyChart message="No calls in this range" />;
  return (
    <div>
      <div className="flex h-6 rounded-md overflow-hidden">
        {segments.map((s) =>
          s.value === 0 ? null : (
            <div
              key={s.label}
              className="h-full"
              style={{ width: `${(s.value / total) * 100}%`, background: s.color }}
              title={`${s.label}: ${s.value}`}
            />
          ),
        )}
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-[11px]">
        {segments.map((s) => (
          <span key={s.label} className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-sm inline-block" style={{ background: s.color }} />
            <span className="text-gray-500">{s.label}</span>
            <span className="tabular-nums">{formatNumber(s.value)}</span>
            <span className="text-gray-400">({((s.value / total) * 100).toFixed(1)}%)</span>
          </span>
        ))}
      </div>
    </div>
  );
}

export function Donut({
  slices,
  size = 140,
}: {
  slices: { label: string; value: number; color: string }[];
  size?: number;
}) {
  const total = slices.reduce((sum, s) => sum + s.value, 0);
  if (total === 0) return <EmptyChart />;

  const R = 60;
  const STROKE = 18;
  const C = 2 * Math.PI * R;

  // Arc lengths and their running start offsets are computed up front rather
  // than by mutating a counter inside map — the callback runs during render,
  // so mutation there is not safe to repeat.
  const arcs = slices.reduce<{ label: string; color: string; len: number; offset: number }[]>((acc, s) => {
    const prev = acc[acc.length - 1];
    const offset = prev ? prev.offset + prev.len : 0;
    acc.push({ label: s.label, color: s.color, len: (s.value / total) * C, offset });
    return acc;
  }, []);

  return (
    <div className="flex items-center gap-4">
      <svg viewBox="0 0 160 160" className="shrink-0" style={{ width: size, height: size }}>
        <g transform="translate(80,80) rotate(-90)">
          {arcs.map((a) => (
            <circle
              key={a.label}
              r={R}
              fill="none"
              stroke={a.color}
              strokeWidth={STROKE}
              strokeDasharray={`${a.len} ${C - a.len}`}
              strokeDashoffset={-a.offset}
            />
          ))}
        </g>
        <text x="80" y="76" textAnchor="middle" fontSize="18" fontFamily="serif" fill={INK}>
          {formatNumber(total)}
        </text>
        <text x="80" y="92" textAnchor="middle" fontSize="9" fill={MUTED}>
          calls
        </text>
      </svg>
      <div className="space-y-1 min-w-0 flex-1">
        {slices.map((s) => (
          <div key={s.label} className="flex items-center gap-2 text-xs">
            <span className="w-2 h-2 rounded-sm shrink-0" style={{ background: s.color }} />
            <span className="truncate">{s.label}</span>
            <span className="tabular-nums text-gray-500 ml-auto shrink-0">{formatNumber(s.value)}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
