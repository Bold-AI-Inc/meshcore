"use client";

import { useEffect, useMemo, useState } from "react";

export type RangePreset = "24h" | "7d" | "30d" | "90d";

export const RANGE_PRESETS: { value: RangePreset; label: string }[] = [
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
];

const HOURS: Record<RangePreset, number> = { "24h": 24, "7d": 168, "30d": 720, "90d": 2160 };

export function useTimeRange(initial: RangePreset = "7d") {
  const [preset, setPreset] = useState<RangePreset>(initial);
  const [nowMinute, setNowMinute] = useState(() => Math.floor(Date.now() / 60000));

  useEffect(() => {
    const id = setInterval(() => setNowMinute(Math.floor(Date.now() / 60000)), 60000);
    return () => clearInterval(id);
  }, []);

  const range = useMemo(() => {
    const to = new Date(nowMinute * 60000);
    const from = new Date(to.getTime() - HOURS[preset] * 3600 * 1000);
    return { from: from.toISOString(), to: to.toISOString() };
  }, [preset, nowMinute]);

  const hourly = HOURS[preset] <= 48;

  return { preset, setPreset, range, hourly };
}
