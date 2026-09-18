# Contributing

## Setup

Install dependencies in `frontend/` and `server/`, then run:

```
make dev
```

## Scope

Mesh is built for small teams running a single instance with one database. Keep changes aligned with that scope. If a change adds clustering, multi database support, or other heavy infra, open an issue first to discuss it.

## Workflow

1. Fork and branch from `main`.
2. Keep changes focused, one concern per pull request.
3. Match existing code style in the package you are editing.
4. Verify your change locally with `make dev` before opening a pull request. There is no unit test suite yet, so manual verification and the `testserve` comparison below are what you have.
5. Describe what changed and why in the pull request description.

## Before changing the gateway data path

Not required, but useful if you are touching anything on the request path, routing, retries, streaming, template rendering, the queue, the concurrency limiter.

1. Add a test model in `testserve/seed/providers.json` against the scenario you care about, or use an existing `-ok` model as a baseline.
2. Bring up the stack with test provider running:
   ```
   make dev-all
   ```
3. Run `testserve/selftest.py` to confirm the provider config resolves correctly before you change anything.
4. Hit the gateway through one of the `caller/` SDKs or curl, and record latency and `testserve`'s `/_debug/stats` (`rps`, `peak_in_flight`) as your baseline.
5. Make your change.
6. Repeat the same calls against the same test model and compare the two runs, latency, peak in flight, and anything in `/_debug/requests` that changed shape.

Include that before and after comparison in the pull request description when the change touches the data path. It is not a hard gate, but it catches regressions that unit tests miss.

## Reporting issues

Open an issue with steps to reproduce, expected behavior, and actual behavior. For security issues, do not open a public issue, contact the maintainers directly.

## License

By contributing, you agree your contributions are licensed under Apache 2.0.
