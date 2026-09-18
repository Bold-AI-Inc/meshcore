/**
 * A runnable tour of every Mesh endpoint.
 *
 *   cd caller/bold-mesh-typescript
 *   npm install
 *   cp ../.env.example .env       # then paste your key in
 *   npm run example
 *
 * Every model named below is a real model on this Mesh deployment. Run
 * listModels() (the last section) to see which ones your own key is actually
 * granted — calling one you weren't granted throws
 * MeshError("model_not_permitted").
 */

import {
  MeshError,
  getUsage,
  imageBlock,
  listModels,
  llmCaller,
  llmEmbeddings,
  llmGenerate,
  saveMediaOutput,
  textBlock,
  type ContentBlock,
} from "./index";

const CHAT_MODEL = "claude-sonnet-5";
const VISION_MODEL = "claude-opus-5";
const SEARCH_MODEL = "claude-opus-5";
const IMAGE_MODEL = "gpt-image-2.5-flare";
const EMBEDDING_MODEL = "text-embedding-3-small";

async function streamingChat(): Promise<void> {
  console.log("--- streaming chat ---");
  const stream = llmCaller({ model: CHAT_MODEL, prompt: "Say hello in one short sentence." });
  for await (const text of stream.textStream) {
    process.stdout.write(text);
  }
  const final = await stream.finalResponse();
  console.log(`\ntokens in/out: ${final.usage.inputTokens}/${final.usage.outputTokens}`);
}

async function blockingChat(): Promise<void> {
  console.log("\n--- non-streaming chat ---");
  const resp = await llmCaller({
    model: CHAT_MODEL,
    prompt: "Name three primary colors.",
    maxTokens: 200,
    streaming: false,
  });
  console.log(resp.text);
  if (resp.truncated) console.log("(hit maxTokens -- raise it to get the rest)");
}

async function vision(path = "photo.png"): Promise<void> {
  console.log("\n--- image input ---");
  const resp = await llmCaller({
    model: VISION_MODEL,
    content: [textBlock("What is in this image? One line."), (await imageBlock({ path })) as ContentBlock],
    streaming: false,
  });
  console.log(resp.text);
}

async function webSearch(): Promise<void> {
  console.log("\n--- web search ---");
  const resp = await llmCaller({
    model: SEARCH_MODEL,
    prompt: "What shipped in AI this week? Two sentences.",
    webSearch: true,
    streaming: false,
  });
  console.log(resp.text);
  for (const s of resp.sources) console.log(`  - ${s.title ?? "(untitled)"}: ${s.url}`);
}

async function generateImage(): Promise<void> {
  console.log("\n--- image generation ---");
  const gen = await llmGenerate({
    model: IMAGE_MODEL,
    prompt: "A watercolor painting of a lighthouse at dusk",
  });
  await saveMediaOutput(gen.output, "lighthouse.png");
  console.log(`wrote lighthouse.png (${gen.media_type})`);
}

async function embed(): Promise<void> {
  console.log("\n--- embeddings ---");
  const result = await llmEmbeddings({ model: EMBEDDING_MODEL, input: "The quick brown fox" });
  console.log(`${result.embedding.length} dimensions, first three:`, result.embedding.slice(0, 3));
}

async function discover(): Promise<void> {
  console.log("\n--- what this key can call ---");
  for (const m of await listModels()) {
    const search = m.web_search ? " +web_search" : "";
    console.log(
      `  ${m.model.padEnd(26)} ${m.kind.padEnd(11)} ${m.status.padEnd(16)} ${m.formats_accepted.join(",")}${search}`,
    );
  }

  console.log("\n--- today's spend ---");
  for (const u of await getUsage()) {
    const cap = u.daily_cap_usd !== null ? `/${u.daily_cap_usd}` : "";
    console.log(`  ${u.model.padEnd(26)} $${u.spent_usd.toFixed(4)}${cap}`);
  }
}

async function main(): Promise<void> {
  try {
    await discover();
    await streamingChat();
    await blockingChat();
    await embed();
    // Uncomment the ones you have access to / have a file for:
    // await vision("photo.png");
    // await webSearch();
    // await generateImage();
  } catch (err) {
    if (err instanceof MeshError) {
      console.error(`\nMesh rejected the call: ${err.code}`);
      if (err.resumeAt) console.error(`retry after ${err.resumeAt}`);
      console.error(err.body);
    } else {
      throw err;
    }
  }
}

void main();
