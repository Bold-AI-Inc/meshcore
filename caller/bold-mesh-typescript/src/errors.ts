/** Raised for any non-2xx response from the Mesh gateway. `code` is the
 * machine-readable error code from the response body (e.g. "budget_exceeded",
 * "invalid_or_revoked_key"); `body` is the full parsed JSON error response,
 * which for some codes carries extra fields such as `resume_at`, `limit`,
 * `cap`, `content_type` or `provider_status`. */
export class MeshError extends Error {
  code: string;
  body: Record<string, unknown>;
  statusCode: number;

  constructor(code: string, body: unknown, statusCode = 0) {
    super(code);
    this.name = "MeshError";
    this.code = code;
    this.body = (body && typeof body === "object" ? body : {}) as Record<string, unknown>;
    this.statusCode = statusCode;
  }

  /** RFC3339 timestamp at which this limit lifts, when the server sent one
   * (rate limits, budget caps, schedule windows). */
  get resumeAt(): string | undefined {
    const value = this.body.resume_at;
    return typeof value === "string" ? value : undefined;
  }
}

/** Raised when MESH_API_KEY (env var, .env file, or apiKey option) can't be
 * resolved. */
export class MeshConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "MeshConfigError";
  }
}
