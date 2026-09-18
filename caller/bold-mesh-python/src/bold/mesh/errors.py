class MeshError(Exception):
    """Raised for any non-2xx response from the Mesh gateway.

    `code` is the machine-readable error code from the response body (e.g.
    "budget_exceeded", "invalid_or_revoked_key"); `body` is the full parsed
    JSON error response, which for some codes carries extra fields such as
    `resume_at`, `limit`, `cap`, `content_type` or `provider_status`.
    """

    def __init__(self, code: str, body: dict, status_code: int = 0):
        super().__init__(code)
        self.code = code
        self.body = body or {}
        self.status_code = status_code

    @property
    def resume_at(self):
        """RFC3339 timestamp at which this limit lifts, when the server sent
        one (rate limits, budget caps, schedule windows). None otherwise."""
        return self.body.get("resume_at")

    def __str__(self) -> str:
        extra = {k: v for k, v in self.body.items() if k != "error"}
        return f"{self.code} {extra}" if extra else self.code


class MeshConfigError(Exception):
    """Raised when MESH_API_KEY (env var, .env file, or api_key= argument)
    can't be resolved."""
