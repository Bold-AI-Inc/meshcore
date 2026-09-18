"""Minimal .env loader -- no python-dotenv dependency, just enough to set
MESH_API_KEY / MESH_SERVER_PATH from a .env file if they aren't already in
the process environment. Runs once per process; explicit environment
variables always win over anything found in the file.
"""

import os

_loaded = False


def load_dotenv_once(path: str = None) -> None:
    global _loaded
    if _loaded:
        return
    _loaded = True

    dotenv_path = path or os.environ.get("MESH_DOTENV_PATH", ".env")
    if not os.path.isfile(dotenv_path):
        return

    with open(dotenv_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, _, value = line.partition("=")
            key = key.strip()
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in ("'", '"'):
                value = value[1:-1]
            os.environ.setdefault(key, value)
