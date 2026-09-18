from fastapi import FastAPI

from . import media, state
from .routes import anthropic, debug, embeddings, files, gemini, generation, ollama, openai

media.ensure()

app = FastAPI(title="testserve", docs_url="/_docs", redoc_url=None)
PATHS = []

for module in (anthropic, openai, gemini, ollama, embeddings, generation, files, debug):
    app.include_router(module.router)
    PATHS += [r.path for r in module.router.routes]


@app.get("/")
async def root():
    return {"service": "testserve", "routes": sorted(PATHS), "stats": state.stats}


@app.get("/healthz")
async def healthz():
    return {"ok": True}
