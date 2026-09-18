"use client";

import { useState } from "react";

type Lang = "typescript" | "python" | "curl";

const LABELS: Record<Lang, string> = {
  typescript: "TypeScript",
  python: "Python",
  curl: "cURL",
};

const ORDER: Lang[] = ["typescript", "python", "curl"];

export function CodeTabs({
  typescript,
  python,
  curl,
  defaultTab,
  defaultCollapsed = false,
}: {
  typescript?: string;
  python?: string;
  curl?: string;
  defaultTab?: Lang;
  defaultCollapsed?: boolean;
}) {
  const snippets: Partial<Record<Lang, string>> = { typescript, python, curl };
  const available = ORDER.filter((lang) => snippets[lang] !== undefined);
  const initialTab = defaultTab && available.includes(defaultTab) ? defaultTab : available[0];

  const [tab, setTab] = useState<Lang>(initialTab);
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <div className="my-3 overflow-hidden rounded-lg border border-gray-700 bg-[#0d1117] shadow-sm">
      <div className="flex items-center justify-between border-b border-gray-700 bg-[#161b22] pr-2">
        <div className="flex">
          {available.map((lang) => (
            <button
              key={lang}
              type="button"
              onClick={() => setTab(lang)}
              className={`cursor-pointer px-4 py-2 text-xs font-semibold tracking-wide uppercase transition-colors ${
                tab === lang
                  ? "border-b-2 border-blue-400 bg-white/5 text-white"
                  : "border-b-2 border-transparent text-gray-300 hover:text-white"
              }`}
            >
              {LABELS[lang]}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={() => setCollapsed((c) => !c)}
          aria-expanded={!collapsed}
          className="cursor-pointer rounded px-2 py-1 text-xs font-medium text-gray-300 hover:bg-white/10 hover:text-white"
        >
          {collapsed ? "Show code ▾" : "Hide code ▴"}
        </button>
      </div>
      {!collapsed && (
        <pre className="thin-scroll overflow-x-auto p-4 font-mono text-[13px] leading-relaxed whitespace-pre text-white">
          <code>{snippets[tab]}</code>
        </pre>
      )}
    </div>
  );
}
