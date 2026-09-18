"""bold-mesh -- a thin Python client for the Mesh AI gateway.

    from bold.mesh import llm_caller

    resp = llm_caller(model="claude-sonnet-5", prompt="hello", streaming=False)
    print(resp.text)
"""

from .blocks import audio_block, document_block, image_block, text_block, video_block
from .client import (
    DEFAULT_MAX_TOKENS,
    DEFAULT_SERVER_PATH,
    get_usage,
    list_models,
    llm_caller,
    llm_embeddings,
    llm_generate,
)
from .errors import MeshConfigError, MeshError
from .stream import MeshStream, MeshStreamEvent
from .types import (
    EmbeddingResult,
    GenerateResult,
    MediaOutput,
    MeshResponse,
    ModelInfo,
    ModelUsage,
    Source,
    Usage,
    UsageInfo,
)

__version__ = "1.0.0"

__all__ = [
    "llm_caller",
    "llm_generate",
    "llm_embeddings",
    "list_models",
    "get_usage",
    "text_block",
    "image_block",
    "document_block",
    "audio_block",
    "video_block",
    "MeshStream",
    "MeshStreamEvent",
    "MeshResponse",
    "Usage",
    "Source",
    "MediaOutput",
    "GenerateResult",
    "EmbeddingResult",
    "ModelInfo",
    "ModelUsage",
    "UsageInfo",
    "MeshError",
    "MeshConfigError",
    "DEFAULT_MAX_TOKENS",
    "DEFAULT_SERVER_PATH",
    "__version__",
]
