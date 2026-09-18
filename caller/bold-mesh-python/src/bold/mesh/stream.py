import json
from dataclasses import dataclass
from typing import Iterator, List, Optional

import requests

from .types import MeshResponse, Source, Usage, merge_sources


@dataclass
class MeshStreamEvent:
    """One event out of a MeshStream.

    type is one of:
      "delta"        -- `text` is the raw chunk exactly as the provider
                        emitted it (no reformatting).
      "sources"      -- `sources` holds only the citations discovered since
                        the previous sources event (web_search=True only).
      "message_stop" -- the stream is finished; `usage` carries the final
                        token counts. Same event shape as Anthropic's
                        message_stop / Gemini's final chunk.
    """

    type: str
    text: Optional[str] = None
    usage: Optional[Usage] = None
    sources: Optional[List[Source]] = None


class MeshStream:
    """Returned by llm_caller(..., streaming=True) (the default). Use it as
    a context manager and iterate it for events, or use .text_stream for
    just the text:

        with llm_caller(model="claude-sonnet-5", prompt="hello") as stream:
            for text in stream.text_stream:
                print(text, end="", flush=True)
            final = stream.get_final_response()
            print(final.usage.input_tokens, final.usage.output_tokens)

    With web_search=True, .sources fills in as citations arrive and is
    complete once iteration finishes.
    """

    def __init__(self, response, model: str):
        self._response = response
        self._model = model
        self._accumulated = ""
        self._final: Optional[MeshResponse] = None
        self._request_id: Optional[str] = None
        self.sources: List[Source] = []
        self.queries: List[str] = []
        self.usage = Usage()
        self.search_suggestions_html: Optional[str] = None
        self.truncated = False
        self.incomplete = False

    def __enter__(self) -> "MeshStream":
        return self

    def __exit__(self, exc_type, exc_val, exc_tb) -> bool:
        self.close()
        return False

    def __iter__(self) -> Iterator[MeshStreamEvent]:
        if self._final is not None:
            return
        try:
            yield from self._events()
        finally:
            # The gateway always sends a `done` line, but a connection that
            # drops mid-stream never gets there. Assemble what we have so
            # get_final_response() returns a usable object instead of None.
            if self._final is None:
                self.incomplete = True
                self._finish()

    def _events(self) -> Iterator[MeshStreamEvent]:
        try:
            for line in self._response.iter_lines(decode_unicode=True):
                if not line:
                    continue
                try:
                    chunk = json.loads(line)
                except ValueError:
                    # A partial or non-JSON line can only mean the connection
                    # died mid-write; treat it the same as a dropped stream.
                    continue
                self._request_id = chunk.get("id", self._request_id)

                if chunk.get("sources"):
                    incoming = [Source._from_dict(s) for s in chunk["sources"]]
                    before = len(self.sources)
                    self.sources = merge_sources(self.sources, incoming)
                    new = self.sources[before:]
                    if new and not chunk.get("done"):
                        yield MeshStreamEvent(type="sources", sources=new)

                if chunk.get("done"):
                    self.usage = Usage._from_dict(chunk.get("usage"))
                    self.queries = list(chunk.get("queries") or [])
                    self.search_suggestions_html = chunk.get("search_suggestions_html")
                    self.truncated = bool(chunk.get("truncated"))
                    self.incomplete = bool(chunk.get("incomplete"))
                    self._finish()
                    yield MeshStreamEvent(type="message_stop", usage=self.usage)
                    return

                delta = chunk.get("delta")
                if delta:
                    self._accumulated += delta
                    yield MeshStreamEvent(type="delta", text=delta)
        except requests.exceptions.RequestException:
            # The response was already a 200 and text may have arrived, so a
            # transport failure here means a truncated reply, not a failed
            # call. End the stream and let .incomplete carry the news rather
            # than raising a requests-specific exception at the caller.
            return

    def _finish(self) -> None:
        self._final = MeshResponse(
            id=self._request_id or "",
            model=self._model,
            text=self._accumulated,
            usage=self.usage,
            sources=self.sources,
            queries=self.queries,
            search_suggestions_html=self.search_suggestions_html,
            truncated=self.truncated,
            incomplete=self.incomplete,
        )

    @property
    def text_stream(self) -> Iterator[str]:
        """Just the text deltas, in order -- skips every other event."""
        for event in self:
            if event.type == "delta":
                yield event.text

    def get_final_response(self) -> MeshResponse:
        """Drains the rest of the stream (if not already consumed) and
        returns the assembled MeshResponse -- full text, token usage, web
        sources, and the truncated/incomplete flags."""
        if self._final is None:
            for _ in self:
                pass
        return self._final

    def close(self) -> None:
        self._response.close()
