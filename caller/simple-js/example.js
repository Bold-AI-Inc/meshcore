/**
 * Runnable examples for the one-file JS client.
 *
 *   export MESH_API_KEY=mesh_live_xxxxxxxx
 *   export MESH_SERVER_PATH=https://your-mesh-server.example.com
 *   node example.js
 *
 * Every model named below is a real model on this Mesh deployment. The first
 * section prints the ones your own key is actually granted -- calling one you
 * weren't granted throws MeshError("model_not_permitted").
 */

const {
  MeshError,
  llmCaller,
  llmEmbeddings,
  llmGenerate,
  models,
  saveOutput,
  usage,
} = require("./mesh");

const CHAT_MODEL = "claude-sonnet-5";
const VISION_MODEL = "claude-opus-5";
const SEARCH_MODEL = "claude-opus-5";
const IMAGE_MODEL = "gpt-image-2.5-flare";
const EMBEDDING_MODEL = "text-embedding-3-small";

async function discover() {
  console.log("--- what this key can call ---");
  for (const m of await models()) {
    const search = m.web_search ? " +web_search" : "";
    console.log(
      `  ${m.model.padEnd(26)} ${m.kind.padEnd(11)} ${m.status.padEnd(16)} ${m.formats_accepted.join(",")}${search}`,
    );
  }

  console.log("\n--- today's spend ---");
  for (const u of await usage()) {
    const cap = u.daily_cap_usd !== null ? `/${u.daily_cap_usd}` : "";
    console.log(`  ${u.model.padEnd(26)} $${u.spent_usd.toFixed(4)}${cap}`);
  }
}

async function streamingChat() {
  console.log("\n--- streaming chat ---");
  const stream = llmCaller(CHAT_MODEL, "Say hello in one short sentence.");
  for await (const chunk of stream) process.stdout.write(chunk);
  console.log(`\ntokens in/out: ${stream.usage.input_tokens}/${stream.usage.output_tokens}`);
}

async function blockingChat() {
  console.log("\n--- non-streaming chat ---");
  const result = await llmCaller(CHAT_MODEL, "Name three primary colors.", {
    maxTokens: 200,
    streaming: false,
  });
  console.log(result.text);
  if (result.truncated) console.log("(hit maxTokens -- raise it to get the rest)");
}

async function vision(filePath = "photo.png") {
  console.log("\n--- image input ---");
  const result = await llmCaller(VISION_MODEL, "What is this, in one line?", {
    images: [filePath],
    streaming: false,
  });
  console.log(result.text);
}

async function webSearch() {
  console.log("\n--- web search ---");
  const result = await llmCaller(SEARCH_MODEL, "What shipped in AI this week? Two sentences.", {
    webSearch: true,
    streaming: false,
  });
  console.log(result.text);
  for (const s of result.sources) console.log(`  - ${s.title || "(untitled)"}: ${s.url}`);
}

async function generateImage() {
  console.log("\n--- image generation ---");
  const gen = await llmGenerate(IMAGE_MODEL, "A watercolor painting of a lighthouse at dusk");
  await saveOutput(gen.output, "lighthouse.png");
  console.log(`wrote lighthouse.png (${gen.media_type})`);
}

async function embed() {
  console.log("\n--- embeddings ---");
  const result = await llmEmbeddings(EMBEDDING_MODEL, "The quick brown fox");
  console.log(`${result.embedding.length} dimensions, first three:`, result.embedding.slice(0, 3));
}

async function main() {
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

main();
