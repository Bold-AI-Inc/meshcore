import time
from collections import deque

from .config import LOG_LIMIT

_log = deque(maxlen=LOG_LIMIT)
_flaky = {}
stats = {"total": 0, "in_flight": 0, "peak_in_flight": 0, "started_at": time.time()}
override = {"scenario": ""}


def record(entry):
    stats["total"] += 1
    entry["seq"] = stats["total"]
    _log.append(entry)
    return entry


def entries(limit=50, path=None, model=None):
    items = [
        e for e in _log
        if (path is None or path in e["path"]) and (model is None or model == e["model"])
    ]
    return items[-limit:][::-1]


def last():
    return _log[-1] if _log else None


def clear():
    _log.clear()
    _flaky.clear()
    stats.update(total=0, peak_in_flight=stats["in_flight"])


def flaky_count(key):
    _flaky[key] = _flaky.get(key, 0) + 1
    return _flaky[key]


def enter():
    stats["in_flight"] += 1
    stats["peak_in_flight"] = max(stats["peak_in_flight"], stats["in_flight"])


def leave():
    stats["in_flight"] -= 1
