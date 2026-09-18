import base64
import struct
from pathlib import Path

DIR = Path(__file__).parent / "files"

PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)


def _box(kind, payload):
    return struct.pack(">I", len(payload) + 8) + kind + payload


def _mp4():
    ftyp = _box(b"ftyp", b"isom" + struct.pack(">I", 512) + b"isomiso2mp41")
    mdat = _box(b"mdat", b"testserve dummy video payload " * 16)
    return ftyp + _box(b"free", b"") + mdat


def _wav(ms=200, rate=8000):
    body = b"\x00\x00" * (rate * ms // 1000)
    header = (
        b"RIFF" + struct.pack("<I", 36 + len(body)) + b"WAVEfmt "
        + struct.pack("<IHHIIHH", 16, 1, 1, rate, rate * 2, 2, 16)
        + b"data" + struct.pack("<I", len(body))
    )
    return header + body


def _pdf():
    return (
        b"%PDF-1.4\n"
        b"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
        b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 72 72]>>endobj\n"
        b"trailer<</Root 1 0 R/Size 4>>\n%%EOF\n"
    )


FILES = {
    "dummy.png": ("image/png", PNG),
    "dummy.mp4": ("video/mp4", _mp4()),
    "dummy.wav": ("audio/wav", _wav()),
    "dummy.pdf": ("application/pdf", _pdf()),
}


def ensure():
    DIR.mkdir(exist_ok=True)
    for name, (_, data) in FILES.items():
        path = DIR / name
        if not path.exists():
            path.write_bytes(data)


def read(name):
    path = DIR / name
    if path.exists():
        return path.read_bytes()
    return FILES[name][1]


def media_type(name):
    if name in FILES:
        return FILES[name][0]
    return "application/octet-stream"


def exists(name):
    return name in FILES or (DIR / name).exists()


def b64(name):
    return base64.b64encode(read(name)).decode()
