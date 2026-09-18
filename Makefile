.PHONY: dev dev-all testserve testserve-deps testserve-test

PY ?= python3
VENV := testserve/.venv
VENV_PY := $(VENV)/bin/python
TESTSERVE_PORT ?= 9000

dev:
	@trap 'kill 0' EXIT INT TERM; \
	(cd frontend && npm run dev) & \
	(cd server && air) & \
	wait

dev-all: $(VENV)/.installed
	@trap 'kill 0' EXIT INT TERM; \
	(cd frontend && npm run dev) & \
	(cd server && air) & \
	$(VENV_PY) -m uvicorn testserve.main:app --port $(TESTSERVE_PORT) --reload & \
	wait

$(VENV)/.installed: testserve/requirements.txt
	$(PY) -m venv $(VENV)
	$(VENV)/bin/pip install -q --upgrade pip
	$(VENV)/bin/pip install -q -r testserve/requirements.txt
	@touch $@

testserve-deps: $(VENV)/.installed

testserve: $(VENV)/.installed
	$(VENV_PY) -m uvicorn testserve.main:app --port $(TESTSERVE_PORT) --reload

testserve-test: $(VENV)/.installed
	@curl -sf localhost:$(TESTSERVE_PORT)/healthz >/dev/null || \
		{ echo "testserve is not running on :$(TESTSERVE_PORT) — start it with 'make testserve'"; exit 1; }
	$(VENV_PY) testserve/selftest.py
