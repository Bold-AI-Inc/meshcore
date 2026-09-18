export default function DocsPage() {
  return (
    <div className="max-w-2xl space-y-4 text-sm">
      <h1 className="text-lg font-serif">Provider setup docs</h1>

      <Section title="How verification works">
        When you click Save on a new provider, Mesh sends one tiny test request per model to the
        provider&apos;s real endpoint using the templates you entered. If every model responds
        successfully, the provider and its models are saved in one transaction. If any model
        fails, nothing is written to the database — you&apos;ll see the exact error returned by
        the provider next to that model.
      </Section>

      <Section title="Placeholders">
        Use these inside the endpoint URL, header template, and body template — they&apos;re
        substituted before the request is sent:
        <ul className="list-disc pl-5 mt-1 space-y-0.5">
          <li><code>{"{{api_key}}"}</code> — the API key you entered</li>
          <li><code>{"{{model}}"}</code> — the resolved model id for the model being called (some providers, like Gemini, need this in the URL itself, not just the body)</li>
          <li><code>{"{{query}}"}</code> — every text block in the request, concatenated (a plain-text-only call, or a client still using the old <code>query</code> field, produces the same thing)</li>
          <li><code>{"{{content_json}}"}</code> — the full multimodal content array/parts, already rendered into this provider&apos;s own shape per its <b>provider family</b> — see the Multimodal section below. Unlike the others, this one is spliced in raw (unescaped) as a JSON structure, not a plain string.</li>
          <li><code>{"{{input_json}}"}</code> — embedding request bodies only: the input text, JSON-encoded</li>
          <li><code>{"{{prompt_json}}"}</code> — generation request bodies only: the prompt text, JSON-encoded</li>
          <li><code>{"{{max_tokens_json}}"}</code> — chat request bodies only: the caller&apos;s optional <code>max_tokens</code> field on <code>POST /v1/proxy</code>, or <code>1024</code> if they didn&apos;t set one. Use this instead of hardcoding a number so callers who need a longer reply aren&apos;t stuck with whatever ceiling was typed in here once — this is exactly what was truncating some Anthropic replies before this field existed.</li>
        </ul>
        <p className="mt-2 text-gray-500">
          <code>{"{{content_json}}"}</code>/<code>{"{{input_json}}"}</code>/<code>{"{{prompt_json}}"}</code>/
          <code>{"{{max_tokens_json}}"}</code> still need to be written <b>quoted</b> in the template you type
          here — e.g. <code>{'"content": "{{content_json}}"'}</code>, not{" "}
          <code>{'"content": {{content_json}}'}</code> — even though the value they render to is
          unquoted JSON. The template itself is stored as-is and must be valid JSON on its own;
          Mesh strips the quotes back off automatically when it splices in the real array/object/number.
        </p>
      </Section>

      <Section title="How token usage is captured and billed">
        <p className="mb-2">
          Mesh does not count words or tokens itself. Pricing is still entered as{" "}
          <b>$ per 1,000 units</b> on each model, but the unit is now a <b>token</b>, and the
          token count comes directly from the number the provider reports in its own response —
          the same number that provider bills you on.
        </p>
        <p className="mb-2">
          <b>Usage input tokens path</b> and <b>Usage output tokens path</b> on the provider form
          work exactly like <code>Response delta path</code>: a dot-separated path (numeric
          segments index into arrays) that Mesh checks against every SSE event&apos;s JSON while
          streaming. Whichever event actually contains that path resolves; every other event is
          skipped for that path. Both fields are optional — leave either blank and that side of
          billing stays 0 for calls to this provider until it&apos;s configured, the same way an
          unset <code>response_delta_path</code> would yield no text.
        </p>
        <p className="mb-2">
          Providers place usage in different places, so the last value seen by the time the
          stream ends is what gets kept and billed:
        </p>
        <ul className="list-disc pl-5 mt-1 space-y-1">
          <li>
            <b>Anthropic</b> splits usage across two events — input tokens on the very first
            event (<code>message_start</code>), output tokens on the last one (
            <code>message_delta</code>), right before <code>message_stop</code>. No request
            changes needed; usage is always included.
          </li>
          <li>
            <b>OpenAI</b> only sends usage at all if the request body includes{" "}
            <code>{'"stream_options": {"include_usage": true}'}</code> (already in the preset
            below). When present, every chunk&apos;s <code>usage</code> is <code>null</code>{" "}
            except the very last one, which carries the full count. If the stream is cut off
            before that last chunk, no usage is ever seen for that call.
          </li>
          <li>
            <b>Gemini</b> sends a cumulative <code>usageMetadata</code> block on every chunk, so
            the last chunk&apos;s value (which is what&apos;s kept) is already the running total
            — no request changes needed.
          </li>
        </ul>
        <p className="mt-2">
          Captured counts are stored per call as <code>tokens_in</code>/<code>tokens_out</code>{" "}
          on <code>request_logs</code>, alongside the <code>input_cost</code>/
          <code>output_cost</code> computed from them — visible per-call and in aggregate on each
          user&apos;s activity logs.
        </p>
      </Section>

      <Section title="Per-user-per-model hourly call cap">
        <p className="mb-2">
          Separate from a provider&apos;s own <b>Max calls/hour</b> (which caps total traffic to
          that provider across every user), each user can also have a per-model hourly call cap —
          set it from that user&apos;s <b>Model access</b> panel, in the same editor as the daily
          budget and allowed-hours schedule. Leave it blank for unlimited (the default — nothing
          changes for existing users until you set one).
        </p>
        <p>
          Enforcement happens in-memory on every <code>/v1/proxy</code>/<code>/v1/embeddings</code>{" "}
          call, the same way the daily budget and schedule checks do — no extra database round
          trip. A user who hits it gets <code>user_model_hourly_limit_reached</code> with a{" "}
          <code>resume_at</code> until the top of the next hour.
        </p>
      </Section>

      <Section title="Multimodal — images, documents, audio, video">
        <p className="mb-2">
          Requests can carry <code>content</code> blocks (text/image/document/audio/video)
          instead of a plain <code>query</code> string. Two things control whether that actually
          works end-to-end:
        </p>
        <ul className="list-disc pl-5 space-y-1">
          <li>
            <b>Provider family</b> (set on the provider form) — tells Mesh how to render those
            blocks into <i>this</i> provider&apos;s own content shape, substituted wherever the
            body template references <code>{"{{content_json}}"}</code>. <code>generic</code>{" "}
            (the default) means text-only — every provider created before this feature existed
            keeps working unchanged, since it never references <code>{"{{content_json}}"}</code>{" "}
            at all. Pick <code>anthropic</code>/<code>openai</code>/<code>gemini</code>/
            <code>ollama</code> to enable multimodal for a real integration — the Quick Fill
            presets below already set the right family and template.
          </li>
          <li>
            <b>Per-model capability flags</b> (Accepts image/document/audio/video, set on each
            model) — gate which content types <i>that specific model</i> is allowed to receive. A
            request sending a type the model isn&apos;t flagged for is rejected with{" "}
            <code>content_type_not_supported_by_model</code> before Mesh ever contacts the
            provider — set these to match what the underlying model actually supports (e.g. most
            Claude models take image + document but not audio/video).
          </li>
        </ul>
        <p className="mt-2 text-gray-500">
          Per-content-type size caps (20 MB image / 50 MB document / 75 MB audio / 200 MB video)
          are enforced server-side regardless of these settings — an oversized block is rejected
          with a 400 before it&apos;s ever forwarded upstream, never silently truncated.
        </p>
      </Section>

      <Section title="Embeddings">
        <p className="mb-2">
          A provider can additionally support embeddings — set on the provider form:{" "}
          <b>embedding endpoint URL</b>, <b>embedding request body template</b> (use{" "}
          <code>{"{{input_json}}"}</code> for the input text, alongside the usual{" "}
          <code>{"{{api_key}}"}</code>/<code>{"{{model}}"}</code>), and <b>embedding response
          vector path</b> — a dot-path resolving to the vector array in the response, e.g.{" "}
          <code>data.0.embedding</code> for OpenAI&apos;s single-input response shape.
        </p>
        <p>
          Then add an embedding <i>model</i> under that provider (Kind = &quot;embedding&quot; on
          the model form) — it&apos;s access-controlled, quota-checked, and billed exactly like a
          chat model, just callable via <code>POST /v1/embeddings</code> instead of{" "}
          <code>/v1/proxy</code>. v1 supports one text input per call — no batch input.
        </p>
      </Section>

      <Section title="Image & video generation">
        <p className="mb-2">
          A provider can additionally support generation — set on the provider form:{" "}
          <b>generation endpoint URL</b>, <b>generation request body template</b> (use{" "}
          <code>{"{{prompt_json}}"}</code> for the prompt text, alongside the usual{" "}
          <code>{"{{api_key}}"}</code>/<code>{"{{model}}"}</code>), and <b>generation response
          media path</b> — a dot-path resolving to the generated media in the response: either a
          single string, or an array of strings (multiple items). It does <i>not</i> descend into
          an array of objects — for a shape like OpenAI&apos;s <code>data: [{'{"b64_json": "..."}'}]</code>{" "}
          with <code>n &gt; 1</code>, point the path at a specific index (
          <code>data.0.b64_json</code>) rather than at <code>data</code> itself.
        </p>
        <p className="mb-2">
          Each resolved string is classified automatically: an <code>http://</code>/
          <code>https://</code> value is returned to the caller as{" "}
          <code>{'{"type": "url", "data": "..."}'}</code>, anything else as{" "}
          <code>{'{"type": "base64", "data": "..."}'}</code>. Mesh does not decode, store, or
          otherwise touch the media bytes themselves — it forwards the resolved string straight
          through to the caller, who saves it client-side (see <code>/docs</code> for the
          cURL/SDK pattern).
        </p>
        <p>
          Then add a generation <i>model</i> under that provider — Kind = &quot;image
          generation&quot; or &quot;video generation&quot; on the model form, with an{" "}
          <b>output media type</b> (a fixed MIME type, e.g. <code>image/png</code> or{" "}
          <code>video/mp4</code>, returned in every response so callers know what they received).
          It&apos;s access-controlled, quota-checked, and logged exactly like a chat or embedding
          model, just callable via <code>POST /v1/generate</code>. Like embeddings, verification
          on save tests the generation endpoint specifically — not the provider&apos;s chat
          endpoint — so a broken generation config is caught before anything is written.
        </p>
      </Section>

      <Section title="The request queue">
        <p className="mb-2">
          Every <code>/v1/proxy</code>/<code>/v1/embeddings</code>/<code>/v1/generate</code> call
          passes through a bounded admission gate before Mesh does any real work — this is what
          stands between a burst of concurrent requests (especially multimodal ones, which hold
          far more memory per in-flight call than plain text) and the process running out of
          memory. A request that can&apos;t be admitted quickly is rejected with{" "}
          <code>server_busy</code> rather than queued indefinitely.
        </p>
        <p className="mb-2">
          Admission cost is weighted by content: an image/document/audio/video request reserves
          more of the shared budget than a plain text one, so a flood of media requests
          automatically admits fewer concurrent calls than a flood of text ones would. Every{" "}
          <code>/v1/generate</code> call is charged at video cost regardless of image vs. video
          kind — a deliberately conservative weight, since generation holds a connection open far
          longer than a chat/embedding call and can return a large response. A background check
          also multiplies cost under real memory/goroutine pressure, as a backstop on top of the
          weighted budget.
        </p>
        <p>
          Tunable via environment variables on the server (defaults are reasonable for most
          deployments — only change these if you&apos;ve actually measured a need to):{" "}
          <code>QUEUE_CAPACITY</code>, <code>QUEUE_ADMIT_TIMEOUT_MS</code>,{" "}
          <code>QUEUE_GOROUTINE_CEILING</code>, <code>QUEUE_HEAP_CEILING_MB</code>, and{" "}
          <code>MAX_REQUEST_BODY_MB</code> (the hard cap on a single request&apos;s raw body
          size, checked before any JSON decoding).
        </p>
      </Section>

      <Section title="Every call is logged — success, upstream failure, or denial">
        <p className="mb-2">
          Every request that reaches <code>/v1/proxy</code>/<code>/v1/embeddings</code>/
          <code>/v1/generate</code> produces a row in that user&apos;s activity log — not just the
          ones that reach a provider. A request rejected for a bad key, a blocked model, a
          rate/budget/schedule limit, an unsupported content type, or <code>server_busy</code> is
          recorded with{" "}
          <code>outcome: &quot;denied&quot;</code> and the exact <code>deny_reason</code> code,
          visible from that user&apos;s log view with the same filters as everything else. A
          request whose bearer key doesn&apos;t resolve to any user at all still gets logged (by
          source IP), just without a user to attribute it to.
        </p>
      </Section>

      <Section title="Quick fill">
        The Anthropic / OpenAI / Gemini / Ollama buttons at the top of the Add Provider form
        pre-fill the endpoint, headers, body template (already wired for multimodal via{" "}
        <code>{"{{content_json}}"}</code>), provider family, and one example model — you only need
        to paste your API key (skip it entirely for a local Ollama/OpenAI-compatible server) and
        adjust pricing.
      </Section>

      <Section title="A note on the body templates below">
        The per-provider templates in this section show the plain <code>{"{{query}}"}</code> form
        for clarity — the Quick Fill buttons above actually save{" "}
        <code>{"{{content_json}}"}</code> in the <code>content</code> field instead (e.g. Anthropic:{" "}
        <code>{'"content": "{{content_json}}"'}</code>), which is multimodal-capable and behaves
        identically to <code>{"{{query}}"}</code> for a text-only call. If you&apos;re hand-writing
        a template instead of using Quick Fill and want multimodal support, use{" "}
        <code>{"{{content_json}}"}</code> the same way.
      </Section>

      <Section title="Anthropic (Claude)">
        <p className="mb-2">
          Reference: <DocLink href="https://platform.claude.com/docs/en/api/messages">platform.claude.com/docs/en/api/messages</DocLink>
        </p>
        <Code>{`Endpoint:  https://api.anthropic.com/v1/messages
Method:    POST
Auth:      header  x-api-key: {{api_key}}
Required:  header  anthropic-version: 2023-06-01   (fixed version string, not a date to update)

Header template:
{
  "x-api-key": "{{api_key}}",
  "anthropic-version": "2023-06-01"
}

Body template:
{
  "model": "{{model}}",
  "max_tokens": 1024,
  "stream": true,
  "messages": [{"role": "user", "content": "{{query}}"}]
}

Response delta path:  delta.text
(each SSE content_block_delta event's JSON has {"delta":{"type":"text_delta","text":"..."}})

Usage input tokens path:   message.usage.input_tokens   (only present on message_start)
Usage output tokens path:  usage.output_tokens          (only present on message_delta)

Example model id: claude-opus-4-8`}</Code>
      </Section>

      <Section title="OpenAI">
        <p className="mb-2">
          Reference: <DocLink href="https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create">developers.openai.com — Create chat completion</DocLink>
        </p>
        <Code>{`Endpoint:  https://api.openai.com/v1/chat/completions
Method:    POST
Auth:      header  Authorization: Bearer {{api_key}}

Header template:
{
  "Authorization": "Bearer {{api_key}}"
}

Body template:
{
  "model": "{{model}}",
  "messages": [{"role": "user", "content": "{{query}}"}],
  "stream": true,
  "stream_options": {"include_usage": true}
}

Response delta path:  choices.0.delta.content
(stream ends with a literal "data: [DONE]" line)

Usage input tokens path:   usage.prompt_tokens
Usage output tokens path:  usage.completion_tokens
(both only present on the final chunk — requires stream_options.include_usage above,
otherwise OpenAI never sends usage at all)

Example model ids: gpt-4o, gpt-4.1
(newer names like gpt-5.x change often — confirm against OpenAI's own model list before using one)`}</Code>
      </Section>

      <Section title="Gemini">
        <p className="mb-2">
          Reference: <DocLink href="https://ai.google.dev/api/generate-content">ai.google.dev/api/generate-content</DocLink>
        </p>
        <Code>{`Endpoint:  https://generativelanguage.googleapis.com/v1beta/models/{{model}}:streamGenerateContent?alt=sse
Method:    POST
Auth:      header  x-goog-api-key: {{api_key}}
Note:      the model id goes in the URL path, not the body — {{model}} works in the endpoint
           URL field here, not just headers/body. alt=sse is required for proper SSE framing.

Header template:
{
  "x-goog-api-key": "{{api_key}}"
}

Body template:
{
  "contents": [{"role": "user", "parts": [{"text": "{{query}}"}]}]
}

Response delta path:  candidates.0.content.parts.0.text
(each chunk carries only its own delta text — concatenate chunks to get the full reply; the
stream simply closes at the end, there's no explicit "done" marker)

Usage input tokens path:   usageMetadata.promptTokenCount
Usage output tokens path:  usageMetadata.candidatesTokenCount
(sent cumulatively on every chunk, so the last chunk's value is already the running total)

Example model ids: gemini-2.5-pro, gemini-2.5-flash
(newer 3.x names appear frequently — confirm against Google's own model list before using one)`}</Code>
      </Section>

      <Section title="Resolved model id">
        This must be exactly what the provider&apos;s API expects — no aggregator-style prefixes.
        For Mistral&apos;s own API, use <code>mistral-large-latest</code>, not{" "}
        <code>mistral/mistral-large-latest</code> (that slash-prefixed form is an OpenRouter
        convention, not something Mistral&apos;s own endpoint understands).
      </Section>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white rounded-lg shadow-sm p-4">
      <h2 className="text-sm font-medium mb-2">{title}</h2>
      <div className="text-gray-600">{children}</div>
    </div>
  );
}

function Code({ children }: { children: string }) {
  return (
    <pre className="thin-scroll bg-gray-50 border border-gray-200 rounded-md p-3 text-xs font-mono whitespace-pre-wrap overflow-x-auto">
      {children}
    </pre>
  );
}

function DocLink({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noreferrer" className="underline hover:text-gray-900">
      {children}
    </a>
  );
}
