"""Result types.

Everything here is built with a `_from_dict` classmethod rather than
`Cls(**payload)` on purpose: the gateway is free to add fields to a
response (it already sends `web_search`, `output_media_type`, `kind`,
`usage`, `truncated` on payloads that once didn't have them), and a client
that explodes on an unknown key is a client that breaks the next time the
server ships anything. Unknown keys are ignored; missing keys fall back to
a sane default.
"""

import base64
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

import requests


@dataclass
class Usage:
    """Token counts the provider reported for one call. Either side is 0 if
    the provider has no usage path configured for it."""

    input_tokens: int = 0
    output_tokens: int = 0
    web_searches: int = 0
    seconds: float = 0.0

    @classmethod
    def _from_dict(cls, payload: Optional[dict]) -> "Usage":
        payload = payload or {}
        return cls(
            input_tokens=int(payload.get("input_tokens") or 0),
            output_tokens=int(payload.get("output_tokens") or 0),
            web_searches=int(payload.get("web_searches") or 0),
            seconds=float(payload.get("seconds") or 0.0),
        )


@dataclass
class Source:
    """One web-search citation. `url` is always present; the rest depend on
    what the provider supplied."""

    url: str
    title: Optional[str] = None
    cited_text: Optional[str] = None
    page_age: Optional[str] = None

    @classmethod
    def _from_dict(cls, payload: dict) -> "Source":
        return cls(
            url=payload.get("url") or "",
            title=payload.get("title"),
            cited_text=payload.get("cited_text"),
            page_age=payload.get("page_age"),
        )


@dataclass
class MeshResponse:
    """The fully-assembled result of a (streamed or non-streamed) call to
    llm_caller: the joined text plus everything the final `done` line
    carried."""

    id: str
    model: str
    text: str
    usage: Usage
    sources: List[Source] = field(default_factory=list)
    queries: List[str] = field(default_factory=list)
    search_suggestions_html: Optional[str] = None
    #: True when the reply stopped because it hit max_tokens, not because
    #: the model was finished — raise max_tokens and call again.
    truncated: bool = False
    #: True when the upstream connection dropped before the provider
    #: signalled completion. The text so far is still valid, just partial.
    incomplete: bool = False


@dataclass
class MediaOutput:
    type: str  # "url" | "base64"
    data: str

    @classmethod
    def _from_dict(cls, payload: dict) -> "MediaOutput":
        return cls(type=payload.get("type") or "base64", data=payload.get("data") or "")

    def save(self, path: str, timeout: Optional[float] = 300) -> None:
        """Writes this output to a local file. For "base64", decodes and
        writes the bytes directly. For "url", downloads it first. Mesh
        itself never stores the generated file — it is in the response and
        nowhere else."""
        if self.type == "base64":
            with open(path, "wb") as f:
                f.write(base64.standard_b64decode(self.data))
        elif self.type == "url":
            resp = requests.get(self.data, timeout=timeout)
            resp.raise_for_status()
            with open(path, "wb") as f:
                f.write(resp.content)
        else:
            raise ValueError(f"unknown output type {self.type!r}")


@dataclass
class GenerateResult:
    id: str
    model: str
    media_type: str
    output: MediaOutput
    outputs: List[MediaOutput] = field(default_factory=list)
    usage: Usage = field(default_factory=Usage)

    @classmethod
    def _from_dict(cls, payload: dict) -> "GenerateResult":
        outputs = [MediaOutput._from_dict(o) for o in payload.get("outputs") or []]
        output = MediaOutput._from_dict(payload.get("output") or {})
        return cls(
            id=payload.get("id") or "",
            model=payload.get("model") or "",
            media_type=payload.get("media_type") or "application/octet-stream",
            output=output,
            outputs=outputs or [output],
            usage=Usage._from_dict(payload.get("usage")),
        )

    def save(self, path: str, index: int = 0) -> None:
        """save(path) writes outputs[0] (== output). Pass index= for a
        provider configured to return more than one item per call."""
        self.outputs[index].save(path)


@dataclass
class EmbeddingResult:
    id: str
    model: str
    embedding: List[float]
    usage: Usage = field(default_factory=Usage)

    @classmethod
    def _from_dict(cls, payload: dict) -> "EmbeddingResult":
        return cls(
            id=payload.get("id") or "",
            model=payload.get("model") or "",
            embedding=list(payload.get("embedding") or []),
            usage=Usage._from_dict(payload.get("usage")),
        )


@dataclass
class ModelUsage:
    """The live counters GET /v1/models reports per model."""

    calls_this_hour: int = 0
    calls_per_hour_limit: Optional[int] = None
    spent_today_usd: float = 0.0
    daily_cap_usd: Optional[float] = None

    @classmethod
    def _from_dict(cls, payload: Optional[dict]) -> "ModelUsage":
        payload = payload or {}
        return cls(
            calls_this_hour=int(payload.get("calls_this_hour") or 0),
            calls_per_hour_limit=payload.get("calls_per_hour_limit"),
            spent_today_usd=float(payload.get("spent_today_usd") or 0.0),
            daily_cap_usd=payload.get("daily_cap_usd"),
        )


@dataclass
class ModelInfo:
    """One entry from GET /v1/models.

    kind is "chat" (POST /v1/proxy), "embedding" (POST /v1/embeddings) or
    "generation" (POST /v1/generate). status is "available",
    "rate_limited", "budget_exceeded" or "outside_hours".
    """

    model: str
    kind: str
    formats_accepted: List[str] = field(default_factory=lambda: ["text"])
    web_search: bool = False
    status: str = "available"
    usage: ModelUsage = field(default_factory=ModelUsage)
    output_media_type: Optional[str] = None
    resume_at: Optional[str] = None
    allowed_hours: Optional[Dict[str, Any]] = None

    @property
    def available(self) -> bool:
        return self.status == "available"

    @classmethod
    def _from_dict(cls, payload: dict) -> "ModelInfo":
        return cls(
            model=payload.get("model") or "",
            kind=payload.get("kind") or "chat",
            formats_accepted=list(payload.get("formats_accepted") or ["text"]),
            web_search=bool(payload.get("web_search")),
            status=payload.get("status") or "available",
            usage=ModelUsage._from_dict(payload.get("usage")),
            output_media_type=payload.get("output_media_type"),
            resume_at=payload.get("resume_at"),
            allowed_hours=payload.get("allowed_hours"),
        )


@dataclass
class UsageInfo:
    """One entry from GET /v1/usage. daily_cap_usd / remaining_usd are None
    when no daily cap is set for that model."""

    model: str
    kind: str = "chat"
    spent_usd: float = 0.0
    daily_cap_usd: Optional[float] = None
    remaining_usd: Optional[float] = None

    @classmethod
    def _from_dict(cls, payload: dict) -> "UsageInfo":
        return cls(
            model=payload.get("model") or "",
            kind=payload.get("kind") or "chat",
            spent_usd=float(payload.get("spent_usd") or 0.0),
            daily_cap_usd=payload.get("daily_cap_usd"),
            remaining_usd=payload.get("remaining_usd"),
        )


def merge_sources(existing: List[Source], incoming: List[Source]) -> List[Source]:
    """Merges by url, in first-seen order. Mesh streams newly-discovered
    sources as it finds them and then repeats the complete set on the final
    `done` line, so a naive += would duplicate every one. The repeat is often
    richer than the mid-stream version (it carries `cited_text`), so a source
    we already have is topped up field by field rather than dropped."""
    merged = list(existing)
    index = {s.url: i for i, s in enumerate(merged)}
    for source in incoming:
        position = index.get(source.url)
        if position is None:
            index[source.url] = len(merged)
            merged.append(source)
            continue
        current = merged[position]
        merged[position] = Source(
            url=current.url,
            title=current.title or source.title,
            cited_text=current.cited_text or source.cited_text,
            page_age=current.page_age or source.page_age,
        )
    return merged
