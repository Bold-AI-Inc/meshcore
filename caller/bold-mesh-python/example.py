"""A runnable tour of every Mesh endpoint.

    pip install ./caller/bold-mesh-python
    cp caller/.env.example .env      # then paste your key in
    python caller/bold-mesh-python/example.py

Every model named below is a real model on this Mesh deployment. Run
list_models() (the last section) to see which ones your own key is
actually granted — calling one you weren't granted raises
MeshError("model_not_permitted").
"""

from bold.mesh import (
    MeshError,
    get_usage,
    image_block,
    list_models,
    llm_caller,
    llm_embeddings,
    llm_generate,
    text_block,
)

CHAT_MODEL = "claude-sonnet-5"
VISION_MODEL = "claude-opus-5"
SEARCH_MODEL = "claude-opus-5"
IMAGE_MODEL = "gpt-image-2.5-flare"
EMBEDDING_MODEL = "text-embedding-3-small"


def streaming_chat():
    print("--- streaming chat ---")
    with llm_caller(model=CHAT_MODEL, prompt="Say hello in one short sentence.") as stream:
        for text in stream.text_stream:
            print(text, end="", flush=True)
        final = stream.get_final_response()
    print()
    print(f"tokens in/out: {final.usage.input_tokens}/{final.usage.output_tokens}")


def blocking_chat():
    print("\n--- non-streaming chat ---")
    resp = llm_caller(
        model=CHAT_MODEL,
        prompt="Name three primary colors.",
        max_tokens=200,
        streaming=False,
    )
    print(resp.text)
    if resp.truncated:
        print("(hit max_tokens -- raise it to get the rest)")


def vision(path="photo.png"):
    print("\n--- image input ---")
    resp = llm_caller(
        model=VISION_MODEL,
        content=[text_block("What is in this image? One line."), image_block(path=path)],
        streaming=False,
    )
    print(resp.text)


def web_search():
    print("\n--- web search ---")
    resp = llm_caller(
        model=SEARCH_MODEL,
        prompt="What shipped in AI this week? Two sentences.",
        web_search=True,
        streaming=False,
    )
    print(resp.text)
    for s in resp.sources:
        print(f"  - {s.title or '(untitled)'}: {s.url}")


def generate_image():
    print("\n--- image generation ---")
    gen = llm_generate(model=IMAGE_MODEL, prompt="A watercolor painting of a lighthouse at dusk")
    gen.save("lighthouse.png")
    print(f"wrote lighthouse.png ({gen.media_type})")


def embed():
    print("\n--- embeddings ---")
    result = llm_embeddings(model=EMBEDDING_MODEL, input="The quick brown fox")
    print(f"{len(result.embedding)} dimensions, first three: {result.embedding[:3]}")


def discover():
    print("\n--- what this key can call ---")
    for m in list_models():
        search = " +web_search" if m.web_search else ""
        print(f"  {m.model:<26} {m.kind:<11} {m.status:<16} {','.join(m.formats_accepted)}{search}")

    print("\n--- today's spend ---")
    for u in get_usage():
        cap = f"/{u.daily_cap_usd}" if u.daily_cap_usd is not None else ""
        print(f"  {u.model:<26} ${u.spent_usd:.4f}{cap}")


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
