/**
 * Minimal .env loader -- no dependency on the `dotenv` package, just enough
 * to set MESH_API_KEY / MESH_SERVER_PATH from a .env file if they aren't
 * already in process.env. Runs once per process; explicit environment
 * variables always win over anything found in the file. No-ops outside
 * Node (e.g. bundled for a browser), since there's no filesystem there.
 */

let loaded = false;

export function loadDotenvOnce(): void {
  if (loaded) return;
  loaded = true;

  if (typeof process === "undefined" || !process.versions?.node) return;

  const path = process.env.MESH_DOTENV_PATH || ".env";
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const fs = require("fs") as typeof import("fs");
  if (!fs.existsSync(path)) return;

  const content = fs.readFileSync(path, "utf-8");
  for (const rawLine of content.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (!line || line.startsWith("#") || !line.includes("=")) continue;
    const idx = line.indexOf("=");
    const key = line.slice(0, idx).trim();
    let value = line.slice(idx + 1).trim();
    if (value.length >= 2 && value[0] === value[value.length - 1] && (value[0] === '"' || value[0] === "'")) {
      value = value.slice(1, -1);
    }
    if (process.env[key] === undefined) process.env[key] = value;
  }
}
