import type { Metadata } from "next";
import { CodeTabs } from "./CodeTabs";

export const metadata: Metadata = {
  title: "Mesh API docs",
  description: "How to call the Mesh gateway",
};

export default function PublicDocsPage() {
  return (
    <div className="mx-auto max-w-3xl space-y-6 px-6 py-10 text-sm">
      <header>
        <h1 className="font-serif text-xl">Mesh — how to call it</h1>
        <p className="mt-1 text-gray-600">
          Mesh is one HTTP endpoint in front of every model your team is allowed to use. Same
          request shape for Claude, GPT, embeddings and image generation; your admin decides
          which models your key can reach and what you may spend. Read this page top to bottom
          and you will have made a call in about five minutes.
        </p>
      </header>

      <DownloadCard />

      <TableOfContents />

      <Section id="setup" title="1. Set up">
        <p className="mb-2">
          You need two things: your Mesh API key (your admin issued it — it looks like{" "}
          <code>mesh_live_xxxxxxxx</code>) and the address of your Mesh server. Put both in a{" "}
          <code>.env</code> file next to your code, or export them as environment variables.
          Both SDKs pick them up automatically.
        </p>
        <Code>{`# .env
MESH_API_KEY=mesh_live_xxxxxxxx
MESH_SERVER_PATH=https://your-mesh-server.example.com`}</Code>
        <p className="mt-2 mb-1 text-gray-500">Then install whichever client you want:</p>
        <CodeTabs
          python={`# Option A -- the package (typed results, file helpers)
pip install ./caller/bold-mesh-python

# Option B -- zero install, just copy one file into your project
cp caller/simple-python/mesh.py .
pip install requests`}
          typescript={`// Option A -- the package (full type definitions)
npm install ./caller/bold-mesh-typescript

// Option B -- zero install, just copy one file into your project (Node 18+)
cp caller/simple-js/mesh.js .`}
          curl={`export MESH="https://your-mesh-server.example.com"
export KEY="mesh_live_xxxxxxxx"

# nothing to install -- curl and jq are enough`}
        />
        <p className="mt-2 text-gray-500">
          Your key goes in the <code>Authorization</code> header on every endpoint,{" "}
          <code>GET</code> included. There is deliberately no query-string form: a key in a URL
          ends up in proxy logs, browser history and <code>Referer</code> headers. Treat the key
          like a password.
        </p>
      </Section>

      <Section id="first-call" title="2. Your first call">
        <p className="mb-2">
          <code>POST /v1/proxy?model=&lt;name&gt;</code> takes a prompt and streams the reply
          back, one JSON object per line.
        </p>
        <CodeTabs
          python={`from bold.mesh import llm_caller

with llm_caller(model="claude-sonnet-5", prompt="Say hello in one sentence.") as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)
    final = stream.get_final_response()

print()
print(final.usage.input_tokens, final.usage.output_tokens)`}
          typescript={`import { llmCaller } from "bold-mesh";

const stream = llmCaller({ model: "claude-sonnet-5", prompt: "Say hello in one sentence." });
for await (const text of stream.textStream) {
  process.stdout.write(text);
}
const final = await stream.finalResponse();
console.log(final.usage.inputTokens, final.usage.outputTokens);`}
          curl={`curl -N "$MESH/v1/proxy?model=claude-sonnet-5" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "Say hello in one sentence."}'`}
        />
        <p className="mt-2 mb-1 text-gray-500">
          Don&apos;t want a stream? Ask for the whole reply at once:
        </p>
        <CodeTabs
          python={`resp = llm_caller(model="claude-sonnet-5", prompt="Say hello.", streaming=False)
print(resp.text)`}
          typescript={`const resp = await llmCaller({ model: "claude-sonnet-5", prompt: "Say hello.", streaming: false });
console.log(resp.text);`}
        />
        <p className="mt-3 mb-1 font-medium">What comes back on the wire</p>
        <Code>{`{"id":"req_01J...","delta":"Hello"}
{"id":"req_01J...","delta":" there!"}
{"id":"req_01J...","done":true,"usage":{"input_tokens":12,"output_tokens":34}}`}</Code>
        <p className="mt-2 text-gray-500">
          Concatenate every <code>delta</code> in order to get the full reply; the SDKs do this
          for you. <code>-N</code> disables curl&apos;s buffering so you see lines as they
          arrive. The final line may also carry <code>truncated</code> (the reply hit{" "}
          <code>max_tokens</code> instead of finishing — raise it and call again) or{" "}
          <code>incomplete</code> (the connection dropped early; the text so far is still
          valid).
        </p>
        <p className="mt-2 mb-1 text-gray-500">
          <code>max_tokens</code> caps the reply and defaults to <strong>4096</strong>:
        </p>
        <CodeTabs
          defaultTab="python"
          python={`llm_caller(model="claude-opus-5", prompt="Write a detailed essay.", max_tokens=8192)`}
          typescript={`llmCaller({ model: "claude-opus-5", prompt: "Write a detailed essay.", maxTokens: 8192 });`}
          curl={`curl -N "$MESH/v1/proxy?model=claude-opus-5" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "Write a detailed essay.", "max_tokens": 8192}'`}
        />
      </Section>

      <Section id="models" title="3. Which models to ask for">
        <p className="mb-2">
          These are the models configured on this Mesh. Your key may not be granted all of them
          — <code>GET /v1/models</code> (section 8) returns exactly the ones you can call, and
          asking for one you weren&apos;t granted returns <code>model_not_permitted</code>.
        </p>
        <table className="w-full border-collapse text-xs">
          <thead>
            <tr className="border-b border-gray-200 text-left">
              <th className="py-1 pr-3 font-medium">Model</th>
              <th className="py-1 pr-3 font-medium">Call it with</th>
              <th className="py-1 pr-3 font-medium">Accepts</th>
              <th className="py-1 font-medium">Web search</th>
            </tr>
          </thead>
          <tbody className="text-gray-600">
            {MODEL_ROWS.map((row) => (
              <tr key={row.model} className="border-b border-gray-100 align-top">
                <td className="py-1.5 pr-3 font-mono whitespace-nowrap">{row.model}</td>
                <td className="py-1.5 pr-3 font-mono whitespace-nowrap">{row.endpoint}</td>
                <td className="py-1.5 pr-3">{row.accepts}</td>
                <td className="py-1.5">{row.search ? "yes" : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-2 text-gray-500">
          Pick by cost and speed: <code>claude-haiku-4-5</code> for cheap high-volume work,{" "}
          <code>claude-sonnet-5</code> as the everyday default, <code>claude-opus-5</code> when
          the answer has to be right. Models an admin blocked or retired never appear in{" "}
          <code>/v1/models</code> at all.
        </p>
      </Section>

      <Section id="files" title="4. Images, documents, audio & video">
        <p className="mb-2">
          Send <code>content</code> instead of <code>query</code> — an ordered list of typed
          blocks, mixing text and files in one call. The SDK block builders read the file, guess
          its MIME type and base64-encode it for you.
        </p>
        <CodeTabs
          python={`from bold.mesh import llm_caller, text_block, image_block, document_block

resp = llm_caller(model="claude-opus-5", content=[
    text_block("What is in this image?"),
    image_block(path="photo.png"),
], streaming=False)
print(resp.text)

# several files at once -- pass a list
resp = llm_caller(model="claude-opus-5", content=[
    text_block("Compare these and summarize the report"),
    image_block(path=["photo.jpg", "ninja.jpg"]),
    document_block(path="report.pdf"),
], streaming=False)`}
          typescript={`import { llmCaller, textBlock, imageBlock, documentBlock } from "bold-mesh";

const resp = await llmCaller({
  model: "claude-opus-5",
  content: [textBlock("What is in this image?"), await imageBlock({ path: "photo.png" })],
  streaming: false,
});
console.log(resp.text);

// several files at once -- pass an array
await llmCaller({
  model: "claude-opus-5",
  content: [
    textBlock("Compare these and summarize the report"),
    await imageBlock({ path: ["photo.jpg", "ninja.jpg"] }),
    await documentBlock({ path: "report.pdf" }),
  ],
  streaming: false,
});`}
          curl={`IMG_B64=$(base64 -w0 photo.png)      # macOS: base64 -i photo.png

curl -N "$MESH/v1/proxy?model=claude-opus-5" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "content": [
      {"type": "text", "text": "What is in this image?"},
      {"type": "image", "media_type": "image/png", "data": "'"$IMG_B64"'"}
    ]
  }'`}
        />
        <p className="mt-3 mb-1 font-medium">Block shapes</p>
        <Code>{`{"type": "text",     "text": "..."}
{"type": "image",    "media_type": "image/png",       "data": "<base64>"}
{"type": "document", "media_type": "application/pdf",  "data": "<base64>"}
{"type": "audio",    "media_type": "audio/mpeg",       "data": "<base64>"}
{"type": "video",    "media_type": "video/mp4",        "data": "<base64>"}`}</Code>
        <p className="mt-1 text-gray-500">
          Every non-text block takes either <code>media_type</code> + <code>data</code> (inline
          base64) or <code>&quot;url&quot;: &quot;https://...&quot;</code> for a remote file —
          one or the other, not both. Set either <code>query</code> or <code>content</code> on a
          request, never both.
        </p>
        <p className="mt-2 mb-1 font-medium">Limits, per block</p>
        <ul className="list-disc space-y-0.5 pl-5">
          <li>Image — 20 MB</li>
          <li>Document — 50 MB</li>
          <li>Audio — 75 MB</li>
          <li>Video — 200 MB</li>
        </ul>
        <p className="mt-2 text-gray-500">
          Sending a type a model doesn&apos;t accept is rejected with{" "}
          <code>content_type_not_supported_by_model</code> before Mesh forwards anything — check{" "}
          <code>formats_accepted</code> in <code>/v1/models</code> first. An oversized block is
          rejected the same way, as <code>invalid_request</code>; compress or downsample on your
          side rather than retrying the same file.
        </p>
      </Section>

      <Section id="search" title="5. Web search">
        <p className="mb-2">
          Set <code>web_search</code> and the model can search the web and cite what it found.
          Only works on models with <code>web_search: true</code> in <code>/v1/models</code> —
          on this deployment, the Claude models and <code>gpt-5.6-luna-search</code>. Anywhere
          else you get <code>web_search_not_supported_by_model</code> rather than a silently
          unsearched answer.
        </p>
        <CodeTabs
          python={`resp = llm_caller(
    model="claude-opus-5",
    prompt="What shipped in AI this week?",
    web_search=True,
    streaming=False,
)
print(resp.text)
for s in resp.sources:
    print(s.title, s.url)`}
          typescript={`const resp = await llmCaller({
  model: "claude-opus-5",
  prompt: "What shipped in AI this week?",
  webSearch: true,
  streaming: false,
});
console.log(resp.text);
for (const s of resp.sources) console.log(s.title, s.url);`}
          curl={`curl -N "$MESH/v1/proxy?model=claude-opus-5" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "What shipped in AI this week?", "web_search": true}'`}
        />
        <p className="mt-2 mb-1">
          Sources arrive on their own line as soon as the provider finds them — before the text
          that cites them — so you can show &quot;found 2 sources&quot; while the answer is
          still streaming:
        </p>
        <Code>{`{"id":"req_01J...","sources":[{"url":"https://example.com/a","title":"Example A"}]}
{"id":"req_01J...","delta":"This week "}
{"id":"req_01J...","done":true,"usage":{"input_tokens":6039,"output_tokens":931,"web_searches":2},"sources":[...]}`}</Code>
        <p className="mt-2 text-gray-500">
          The <code>done</code> line always repeats the complete source list, so ignoring the
          mid-stream lines costs you nothing — but a client reading both must deduplicate by{" "}
          <code>url</code>. Each source has <code>url</code>, plus <code>title</code>,{" "}
          <code>cited_text</code> and <code>page_age</code> where the provider supplies them.
          Searches bill on top of tokens against the same daily cap;{" "}
          <code>usage.web_searches</code> says how many ran.
        </p>
      </Section>

      <Section id="generation" title="6. Image generation">
        <p className="mb-2">
          <code>POST /v1/generate?model=&lt;name&gt;</code> — one prompt in, one image out, as a
          single JSON response rather than a stream. Use it with the models{" "}
          <code>/v1/models</code> reports as <code>kind: &quot;generation&quot;</code>. It is
          synchronous and can take minutes, so give it a generous timeout instead of assuming
          it&apos;s stuck.
        </p>
        <CodeTabs
          python={`from bold.mesh import llm_generate

gen = llm_generate(model="gpt-image-2.5-flare", prompt="A watercolor lighthouse at dusk")
gen.save("lighthouse.png")`}
          typescript={`import { llmGenerate, saveMediaOutput } from "bold-mesh";

const gen = await llmGenerate({ model: "gpt-image-2.5-flare", prompt: "A watercolor lighthouse at dusk" });
await saveMediaOutput(gen.output, "lighthouse.png");`}
          curl={`curl -s "$MESH/v1/generate?model=gpt-image-2.5-flare" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"prompt": "A watercolor lighthouse at dusk"}' \\
  | jq -r '.output.data' | base64 -d > lighthouse.png`}
        />
        <p className="mt-2 mb-1">Response:</p>
        <Code>{`{
  "id": "req_01J...",
  "model": "gpt-image-2.5-flare",
  "media_type": "image/png",
  "output": {"type": "base64", "data": "<base64-encoded image bytes>"},
  "outputs": [{"type": "base64", "data": "<base64-encoded image bytes>"}],
  "usage": {"input_tokens": 0, "output_tokens": 0}
}`}</Code>
        <p className="mt-2 text-gray-500">
          <code>output</code> is either <code>{'{"type": "base64", "data": "..."}'}</code> (raw
          bytes) or <code>{'{"type": "url", "data": "https://..."}'}</code> (a link the upstream
          provider hosts, not Mesh). <code>outputs</code> is the same as a list, for models that
          return more than one. <strong>Mesh does not store the generated file anywhere</strong>{" "}
          — it is in this response and nowhere else, so save it right then.{" "}
          <code>media_type</code> tells you the right file extension.
        </p>
      </Section>

      <Section id="embeddings" title="7. Embeddings">
        <p className="mb-2">
          <code>POST /v1/embeddings?model=&lt;name&gt;</code> — one text in, one vector out. One
          input per call; loop for a batch.
        </p>
        <CodeTabs
          python={`from bold.mesh import llm_embeddings

result = llm_embeddings(model="text-embedding-3-small", input="The quick brown fox")
print(len(result.embedding))`}
          typescript={`import { llmEmbeddings } from "bold-mesh";

const result = await llmEmbeddings({ model: "text-embedding-3-small", input: "The quick brown fox" });
console.log(result.embedding.length);`}
          curl={`curl "$MESH/v1/embeddings?model=text-embedding-3-small" \\
  -H "Authorization: Bearer $KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"input": "The quick brown fox"}'`}
        />
        <Code>{`{"id":"req_01J...","model":"text-embedding-3-small","embedding":[0.0123,-0.045],"usage":{"input_tokens":5}}`}</Code>
      </Section>

      <Section id="access" title="8. What you can call, and what you've spent">
        <p className="mb-2">
          <code>GET /v1/models</code> is the authoritative list for your key: what you may call,
          what each model accepts, and whether it is usable right now. Cheaper to check than to
          be rejected.
        </p>
        <CodeTabs
          python={`from bold.mesh import list_models, get_usage

for m in list_models():
    print(m.model, m.kind, m.status, m.formats_accepted, m.web_search)

for u in get_usage():
    print(u.model, u.spent_usd, u.remaining_usd)`}
          typescript={`import { listModels, getUsage } from "bold-mesh";

for (const m of await listModels()) console.log(m.model, m.kind, m.status, m.web_search);
for (const u of await getUsage()) console.log(u.model, u.spent_usd, u.remaining_usd);`}
          curl={`curl "$MESH/v1/models" -H "Authorization: Bearer $KEY"
curl "$MESH/v1/usage"  -H "Authorization: Bearer $KEY"`}
        />
        <Code>{`{
  "models": [
    {
      "model": "claude-sonnet-5",
      "kind": "chat",
      "formats_accepted": ["text", "image", "document"],
      "web_search": true,
      "status": "available",
      "usage": {
        "calls_this_hour": 12,
        "calls_per_hour_limit": 100,
        "spent_today_usd": 1.42,
        "daily_cap_usd": 20
      },
      "allowed_hours": {"from": "09:00", "to": "18:00", "timezone": "America/New_York", "days": [1,2,3,4,5]}
    }
  ]
}`}</Code>
        <ul className="mt-2 list-disc space-y-0.5 pl-5">
          <li>
            <code>kind</code> — <code>chat</code> (<code>/v1/proxy</code>),{" "}
            <code>embedding</code> (<code>/v1/embeddings</code>) or <code>generation</code> (
            <code>/v1/generate</code>).
          </li>
          <li>
            <code>status</code> — <code>available</code>, <code>rate_limited</code>,{" "}
            <code>budget_exceeded</code> or <code>outside_hours</code>. Anything but{" "}
            <code>available</code> also carries <code>resume_at</code>.
          </li>
        </ul>
        <p className="mt-2 mb-1">
          <code>GET /v1/usage</code> is today&apos;s cumulative spend per model (for per-call
          token counts, read <code>usage</code> off the response instead):
        </p>
        <Code>{`{"usage":[{"model":"claude-sonnet-5","kind":"chat","spent_usd":1.42,"daily_cap_usd":20,"remaining_usd":18.58}]}`}</Code>
        <p className="mt-2 text-gray-500">
          <code>daily_cap_usd</code> and <code>remaining_usd</code> are <code>null</code> when
          no cap is set for that model.
        </p>
      </Section>

      <Section id="errors" title="9. When something goes wrong">
        <p className="mb-2">
          Every failure is structured JSON — <code>{'{"error": "<code>", ...extra}'}</code> — so
          you always know why, not just that it failed. Both SDKs raise it as{" "}
          <code>MeshError</code> with <code>.code</code>, <code>.body</code> and{" "}
          <code>.resume_at</code>/<code>.resumeAt</code>.
        </p>
        <CodeTabs
          python={`from bold.mesh import MeshError

try:
    resp = llm_caller(model="claude-opus-5", prompt="hi", streaming=False)
except MeshError as e:
    print(e.code)        # e.g. "budget_exceeded"
    print(e.resume_at)   # when the limit lifts, if the server said`}
          typescript={`import { MeshError } from "bold-mesh";

try {
  const resp = await llmCaller({ model: "claude-opus-5", prompt: "hi", streaming: false });
} catch (err) {
  if (err instanceof MeshError) console.error(err.code, err.resumeAt);
  else throw err;
}`}
        />
        <table className="mt-3 w-full border-collapse text-xs">
          <thead>
            <tr className="border-b border-gray-200 text-left">
              <th className="py-1 pr-3 font-medium">Code</th>
              <th className="py-1 font-medium">What to do</th>
            </tr>
          </thead>
          <tbody className="text-gray-600">
            {ERROR_ROWS.map((row) => (
              <tr key={row.code} className="border-b border-gray-100 align-top">
                <td className="py-1.5 pr-3 font-mono whitespace-nowrap">{row.code}</td>
                <td className="py-1.5">{row.meaning}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="mt-3 mb-1 font-medium">Being a good citizen under load</p>
        <p className="mb-2 text-gray-500">
          Mesh admits requests through a capacity-aware queue and rejects fast rather than
          queuing you silently for an unpredictable time. So <code>server_busy</code> is normal
          under load, not a bug — back off a second or two with jitter and retry. Where a
          response carries <code>resume_at</code>, use it instead of guessing. Stagger large
          multimodal or generation jobs rather than firing them all at once: those cost far more
          server-side per call than plain text.
        </p>
      </Section>

      <footer className="border-t border-gray-100 pt-4 text-xs text-gray-400">
        Need a model you don&apos;t have, a higher cap, or different hours? That&apos;s an admin
        decision — talk to whoever runs Mesh for your team. This page only covers the public
        API.
      </footer>
    </div>
  );
}

function DownloadCard() {
  return (
    <div className="rounded-lg bg-white p-4 shadow-sm">
      <h2 className="mb-2 text-sm font-medium">Get the client code</h2>
      <div className="text-gray-700">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <p className="max-w-md">
            Python and TypeScript SDKs, one-file drop-in clients for both, a raw HTTP reference,
            and runnable examples for every endpoint on this page.
          </p>
          <a
            href="/mesh-caller.zip"
            download
            className="shrink-0 rounded-md bg-black px-4 py-1.5 text-xs font-medium text-white transition-colors hover:bg-gray-800"
          >
            Download mesh-caller.zip
          </a>
        </div>
        <p className="mt-2 text-gray-500">
          Unzip it, open <code>README.md</code>, and run the example for your language — the
          first thing it prints is every model your key can actually call.
        </p>
      </div>
    </div>
  );
}

const MODEL_ROWS: { model: string; endpoint: string; accepts: string; search: boolean }[] = [
  { model: "claude-opus-5", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-sonnet-5", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-haiku-4-5", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-opus-4-8", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-opus-4-7", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-opus-4-6", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "claude-sonnet-4-6", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "gpt-5.6-sol", endpoint: "/v1/proxy", accepts: "text, image, document", search: false },
  { model: "gpt-5.6-luna", endpoint: "/v1/proxy", accepts: "text, image, document", search: false },
  { model: "gpt-5.6-terra", endpoint: "/v1/proxy", accepts: "text, image, document", search: false },
  { model: "gpt-5.6-luna-search", endpoint: "/v1/proxy", accepts: "text, image, document", search: true },
  { model: "gpt-4o", endpoint: "/v1/proxy", accepts: "text, image, document", search: false },
  { model: "gpt-image-2.5-flare", endpoint: "/v1/generate", accepts: "text prompt → image/png", search: false },
  { model: "gpt-image-2.5-sunburst", endpoint: "/v1/generate", accepts: "text prompt → image/png", search: false },
  { model: "text-embedding-3-small", endpoint: "/v1/embeddings", accepts: "text → vector", search: false },
  { model: "text-embedding-3-large", endpoint: "/v1/embeddings", accepts: "text → vector", search: false },
];

const ERROR_ROWS: { code: string; meaning: string }[] = [
  {
    code: "invalid_or_revoked_key",
    meaning: "Your key is missing, wrong, revoked, or your account/key has expired. Check MESH_API_KEY.",
  },
  {
    code: "model_not_permitted",
    meaning: "You were never granted this model. Call /v1/models to see what you have.",
  },
  { code: "model_blocked", meaning: "An admin disabled this model globally. Use another one." },
  {
    code: "budget_exceeded",
    meaning: "You hit your daily spend cap for this model. Includes cap and resume_at.",
  },
  {
    code: "user_model_hourly_limit_reached",
    meaning: "Your per-model hourly call cap is used up. Includes resume_at and limit — wait it out.",
  },
  {
    code: "provider_hourly_limit_reached",
    meaning: "The whole provider's hourly quota is used up, not just yours. Includes resume_at.",
  },
  {
    code: "outside_allowed_hours",
    meaning: "Your schedule doesn't permit calls right now. Includes resume_at.",
  },
  { code: "paused_by_admin", meaning: "An admin paused the entire gateway. Nothing to do but wait." },
  {
    code: "content_type_not_supported_by_model",
    meaning: "You sent an image/document/audio/video block to a model that doesn't take that type. Includes content_type.",
  },
  {
    code: "web_search_not_supported_by_model",
    meaning: "web_search: true on a model it isn't enabled for. Check web_search in /v1/models.",
  },
  {
    code: "embeddings_not_supported_by_provider",
    meaning: "You called /v1/embeddings with a model that isn't an embedding model.",
  },
  {
    code: "generation_not_supported_by_provider",
    meaning: "You called /v1/generate with a model that isn't a generation model.",
  },
  {
    code: "invalid_request",
    meaning: "Malformed body — both query and content set, neither set, max_tokens ≤ 0, an oversized block, or a block missing fields.",
  },
  { code: "server_busy", meaning: "Mesh is at capacity. Back off a second or two with jitter and retry." },
  { code: "provider_busy", meaning: "That provider's concurrency limit is saturated. Same handling." },
  {
    code: "upstream_unavailable",
    meaning: "The provider itself failed. Includes provider_status and provider_message. Retry or try another model.",
  },
];

const SECTIONS: { id: string; label: string }[] = [
  { id: "setup", label: "Set up" },
  { id: "first-call", label: "Your first call" },
  { id: "models", label: "Which models to ask for" },
  { id: "files", label: "Images, documents & more" },
  { id: "search", label: "Web search" },
  { id: "generation", label: "Image generation" },
  { id: "embeddings", label: "Embeddings" },
  { id: "access", label: "Access & spend" },
  { id: "errors", label: "When something goes wrong" },
];

function TableOfContents() {
  return (
    <nav className="rounded-lg bg-white p-4 shadow-sm">
      <p className="mb-2 text-xs font-medium tracking-wide text-gray-400 uppercase">On this page</p>
      <ol className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
        {SECTIONS.map((s, i) => (
          <li key={s.id}>
            <a href={`#${s.id}`} className="text-gray-600 hover:text-gray-900 hover:underline">
              {i + 1}. {s.label}
            </a>
          </li>
        ))}
      </ol>
    </nav>
  );
}

function Section({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  return (
    <div id={id} className="scroll-mt-4 rounded-lg bg-white p-4 shadow-sm">
      <h2 className="mb-2 text-sm font-medium">{title}</h2>
      <div className="text-gray-700">{children}</div>
    </div>
  );
}

function Code({ children }: { children: string }) {
  return (
    <pre className="thin-scroll overflow-x-auto rounded-md border border-gray-200 bg-gray-50 p-3 font-mono text-xs whitespace-pre-wrap">
      {children}
    </pre>
  );
}
