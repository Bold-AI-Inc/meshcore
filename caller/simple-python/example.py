"""Runnable examples for the one-file Python client.

    pip install requests
    export MESH_API_KEY=mesh_live_xxxxxxxx
    export MESH_SERVER_PATH=https://your-mesh-server.example.com
    python example.py

Every model named below is a real model on this Mesh deployment. The first
section prints the ones your own key is actually granted -- calling one you
weren't granted raises MeshError("model_not_permitted").
"""

from mesh import MeshError, llm_caller, llm_embeddings, llm_generate, models, save_output, usage

CHAT_MODEL = "claude-sonnet-5"
VISION_MODEL = "claude-opus-5"
SEARCH_MODEL = "claude-opus-5"
IMAGE_MODEL = "gpt-image-2.5-flare"
EMBEDDING_MODEL = "text-embedding-3-small"


def discover():
    print("--- what this key can call ---")
    for m in models():
        search = " +web_search" if m["web_search"] else ""
        formats = ",".join(m["formats_accepted"])
        print(f"  {m['model']:<26} {m['kind']:<11} {m['status']:<16} {formats}{search}")

    print("\n--- today's spend ---")
    for u in usage():
        cap = f"/{u['daily_cap_usd']}" if u["daily_cap_usd"] is not None else ""
        print(f"  {u['model']:<26} ${u['spent_usd']:.4f}{cap}")


def streaming_chat():
    print("\n--- streaming chat ---")
    stream = llm_caller(CHAT_MODEL, "Say hello in one short sentence.")
    for chunk in stream:
        print(chunk, end="", flush=True)
    print()
    print(f"tokens in/out: {stream.usage['input_tokens']}/{stream.usage['output_tokens']}")


def blocking_chat():
    print("\n--- non-streaming chat ---")
    result = llm_caller(CHAT_MODEL, "Name three primary colors.", max_tokens=200, streaming=False)
    print(result["text"])
    if result["truncated"]:
        print("(hit max_tokens -- raise it to get the rest)")


def vision(path="photo.png"):
    print("\n--- image input ---")
    result = llm_caller(VISION_MODEL, "What is this, in one line?", images=[path], streaming=False)
    print(result["text"])


def web_search():
    print("\n--- web search ---")
    result = llm_caller(SEARCH_MODEL, "What shipped in AI this week? Two sentences.",
                        web_search=True, streaming=False)
    print(result["text"])
    for s in result["sources"]:
        print(f"  - {s.get('title') or '(untitled)'}: {s['url']}")


def generate_image():
    print("\n--- image generation ---")
    gen = llm_generate(IMAGE_MODEL, "A watercolor painting of a lighthouse at dusk")
    save_output(gen["output"], "lighthouse.png")
    print(f"wrote lighthouse.png ({gen['media_type']})")


def embed():
    print("\n--- embeddings ---")
    result = llm_embeddings(EMBEDDING_MODEL, "The quick brown fox")
    print(f"{len(result['embedding'])} dimensions, first three: {result['embedding'][:3]}")


if __name__ == "__main__":
    try:
        discover()
        streaming_chat()
        blocking_chat()
        embed()
        # Uncomment the ones you have access to / have a file for:
        # vision("photo.png")
        # web_search()
        # generate_image()
    except MeshError as e:
        print(f"\nMesh rejected the call: {e.code}")
        if e.resume_at:
            print(f"retry after {e.resume_at}")
        print(e.body)
