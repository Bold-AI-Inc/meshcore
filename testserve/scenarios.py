from . import state
from .config import FORCE_SCENARIO

SCENARIOS = (
    "ok", "slow", "hang", "error-400", "error-429", "error-500", "conn-reset",
    "badjson", "nousage", "empty", "unicode", "huge", "flaky", "keepalive", "customdone",
)


def resolve(model):
    forced = state.override["scenario"] or FORCE_SCENARIO
    if forced:
        return forced
    name = model or ""
    for s in sorted(SCENARIOS, key=len, reverse=True):
        if name.endswith("-" + s):
            return s
    return "ok"
