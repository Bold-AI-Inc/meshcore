import os

PORT = int(os.getenv("TESTSERVE_PORT", "9000"))
BASE_URL = os.getenv("TESTSERVE_BASE_URL", f"http://localhost:{PORT}")
API_KEY = os.getenv("TESTSERVE_API_KEY", "testserve-key")
REQUIRE_KEY = os.getenv("TESTSERVE_REQUIRE_KEY", "0") == "1"
DELTA_DELAY_MS = float(os.getenv("TESTSERVE_DELTA_DELAY_MS", "0"))
SLOW_DELAY_MS = float(os.getenv("TESTSERVE_SLOW_DELAY_MS", "5000"))
HANG_SECONDS = float(os.getenv("TESTSERVE_HANG_SECONDS", "3600"))
CHAOS_RATE = float(os.getenv("TESTSERVE_CHAOS_RATE", "0"))
FLAKY_FAILURES = int(os.getenv("TESTSERVE_FLAKY_FAILURES", "2"))
EMBEDDING_DIM = int(os.getenv("TESTSERVE_EMBEDDING_DIM", "16"))
LOG_LIMIT = int(os.getenv("TESTSERVE_LOG_LIMIT", "500"))
FORCE_SCENARIO = os.getenv("TESTSERVE_FORCE_SCENARIO", "")
