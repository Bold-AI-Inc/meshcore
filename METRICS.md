# Mesh — Load, Latency & Resource Metrics

Run date: 2026-09-29 · Target: local `make dev-all` stack · Upstream: `testserve` dummy providers only (no real provider was called)

**No application code or configuration was changed for these tests.** Every provider, model, limit and env var was measured as it was. The load generator and samplers ran from a scratch directory outside the repo.

---

## 1. Headline numbers

| What | Result |
|---|---|
| Total requests sent during the test campaign | **≈ 13.2 million** |
| Mesh added latency (below rate limits, chat stream, p50 / p95 / p99) | **+0.8 ms / +1.1 ms / +0.8 ms** over calling testserve directly |
| End-to-end chat latency through Mesh, unsaturated (p50 / p95 / p99) | **2.2 ms / 3.7 ms / 4.0 ms** |
| Time from client send → request arrives at upstream (auth, policy, admission, template render, dispatch) | **p50 0.99 ms, p99 1.98 ms** |
| Time from upstream arrival → last byte at client (upstream work + stream translation) | **p50 0.99 ms, p99 2.49 ms** |
| Max throughput for one provider | **≈ 53 rps** (capped by the provider's configured `max_outbound_rps = 50`) |
| Max goodput for the mixed workload | **≈ 155–170 rps** (capped by the busiest provider hitting 50 rps) |
| Raw gateway capacity (no provider limiter): `/healthz` · `/v1/models` (full key auth) · bad-key reject | **≈ 117k rps · ≈ 50k rps · ≈ 97k rps** |
| Most simultaneous connections held | **5,000 concurrent users → 7,363 established sockets on :8090, 7,753 goroutines, 0 connection errors** |
| Mesh idle memory | **≈ 25–28 MB RSS, 21 threads, ~18 goroutines** |
| Mesh peak memory | **398 MB RSS** (5,000 users hammering `/v1/models`). Typical saturated chat load: 40–175 MB |
| Sustainable soak (140 rps mix, 180 s, 25,200 req) | **100 % success, p50 1.65 ms, p99 3.75 ms, max 6.8 ms, RSS flat, goroutines back to baseline** |
| Upstream request correctness (1,150 tagged requests inspected at testserve) | **0 API keys leaked into the body, 0 unrendered `{{placeholders}}`** |
| Goroutine / memory leaks after all tests | **None** — goroutines returned to 17–31, RSS returned to ≈ 55–95 MB (Go heap retention) |

---

## 2. Test setup

### Topology

```
loadgen (Go, same host)  ──►  Mesh :8090 (server/tmp/main.exe, built by air)  ──►  testserve :9000 (uvicorn, 1 worker, --reload)
                                     │
                                     └── async telemetry ──► Postgres (Supabase pooler, aws ca-central-1)
```

### Hardware

- Intel Core Ultra 7 255H, 16 logical CPUs, 22.9 GB RAM, Linux 7.0
- The load generator, Mesh, testserve, the Next.js dev frontend and air all ran on the **same machine**, so they competed for CPU. The raw-capacity numbers (§8) are therefore a **lower bound** for Mesh on its own box.

### Credentials used

- Admin login, used only to read `/admin/metrics`, `/admin/status`, `/admin/providers` and `/admin/models`
- Gateway key: the existing `testserve` user's `mesh_live_…` key

### Configuration found (unchanged)

| Setting | Value | Effect on results |
|---|---|---|
| `max_outbound_rps` on every testserve provider | **50** (token bucket, burst 50) | Hard ceiling on requests per second per provider |
| `max_concurrent_upstream` on every testserve provider | **20** | At most 20 open upstream streams per provider |
| `QUEUE_CAPACITY` (admission budget) | **200 units** (text = 1, image = 5, doc = 8, audio = 10, video = 25; generation requests held 25) | At most ~200 text requests admitted at once |
| `QUEUE_ADMIT_TIMEOUT_MS` | **4000** | Wait up to 4 s for admission (→ `503 server_busy`), then up to another 4 s for the provider limiter (→ `503 provider_busy`) |
| Retry | enabled, `max_retries = 3`, backoff 200 ms | Only applies to transport errors (see §9) |
| `log_requests` on testserve providers | **off** | Successful calls write no DB rows during the test. Denials are still logged (§12) |
| Model pricing | $0 | The $20/day cap never triggers |

### Testserve providers and models exercised

| Provider (family) | Models used |
|---|---|
| testserve-anthropic | ts-claude, ts-huge, ts-unicode, ts-keepalive, ts-empty, ts-nousage, ts-500, ts-429, ts-flaky, ts-reset, ts-badjson, ts-slow |
| testserve-openai | ts-gpt, ts-embed, ts-image |
| testserve-gemini | ts-gemini, ts-embed-gemini, ts-video |
| testserve-ollama | ts-ollama |
| testserve-customdone | ts-customdone |
| testserve-video-url | ts-video-url |

### Load patterns

- **Closed loop ("N simultaneous users"):** N workers. Each sends a request, waits for the full response (whole stream), then sends the next.
- **Open loop ("fixed arrival rate"):** requests are fired on a fixed schedule regardless of how fast responses come back. This is how real traffic arrives, and it exposes queueing.
- **Mixed workload** (weights): ts-claude 30 · ts-gpt 20 · ts-gemini 15 · ts-ollama 10 · ts-customdone 5 · ts-embed 10 · ts-embed-gemini 3 · ts-image 3 · ts-video-url 2 · `GET /v1/models` 2.

### What was measured

- **Client side, per request:** TTFB (first body byte), total time (last byte of the stream), status, bytes, whether the stream ended with the `done` frame, error code. Percentiles are p50, p75, p90, p95, p99, p99.9, max and mean.
- **Mesh process (`/proc`, every 250 ms):** CPU % (100 % = one core), RSS, threads, open FDs.
- **Testserve processes:** CPU and RSS, sampled the same way.
- **Sockets:** peak ESTABLISHED TCP connections on :8090 (client→Mesh) and :9000 (Mesh→upstream).
- **Mesh `/admin/metrics`, every 250 ms:** goroutines, admission queue in-flight cost, per-provider in-flight, telemetry counters, DB pool stats.
- **Testserve `/_debug/requests`:** the exact request Mesh sent upstream, with arrival timestamp, `key_leaked_in_body` and `placeholders_left`.

---

## 3. Request lifecycle — where the time goes

Each request carried a unique tag. Testserve records the moment the rewritten upstream request arrives, and both processes share one clock. That splits every request into two phases:

1. **Client → Mesh → upstream arrival.** TCP read, key hash lookup against the in-memory snapshot, policy / budget / rate checks, admission queue, provider limiter, content resolution, template render into the provider's wire shape (Anthropic / OpenAI / Gemini / Ollama), and the upstream dial or reuse.
2. **Upstream arrival → client receives the last byte.** Testserve builds and streams its response, then Mesh parses each SSE event, extracts the delta and usage via the configured dot-paths, and re-emits them as Mesh NDJSON frames (`{"id","delta"}` … `{"done":true,"usage"}`).

| Scenario | Samples | Phase 1: client → upstream arrival (p50 / p75 / p95 / p99 / max ms) | Phase 2: upstream → client done (p50 / p75 / p95 / p99 / max ms) | End-to-end (p50 / p95 / p99 ms) |
|---|---|---|---|---|
| ts-claude, 10 rps | 100 | 0.86 / 1.20 / 1.65 / 1.82 / 1.94 | 0.96 / 1.25 / 1.56 / 2.45 / 2.50 | 1.84 / 3.01 / 3.96 |
| ts-claude, 40 rps | 400 | 0.99 / 1.28 / 1.65 / 1.98 / 2.60 | 0.99 / 1.39 / 2.17 / 2.49 / 3.64 | 2.03 / 3.68 / 4.21 |
| ts-embed, 20 rps | 200 | 0.93 / 1.19 / 1.49 / 1.77 / 2.84 | 0.81 / 1.02 / 1.37 / 1.76 / 1.92 | 1.76 / 2.69 / 3.01 |
| ts-claude, **50 users (saturated)** | 450 | **998.6** / 999.3 / 1000.1 / 1000.5 / 1000.6 | 1.14 / 1.41 / 3.14 / 4.26 / 4.72 | 999.6 / 1001.5 / 1002.4 |

**Reading this:** when the provider has headroom, Mesh takes about 1 ms to authenticate, verify, rewrite and dispatch a request, and about 1 ms to stream the response back. When the provider's 50 rps limit is exceeded, all the extra time is spent **waiting in Mesh's provider limiter before dispatch** (phase 1). With 50 users against 50 rps that wait is exactly 50 ÷ 50 = 1 s. The response path stays about 1 ms.

---

## 4. Per-endpoint latency (below all limits, open loop at 20 rps, 300 requests each)

| Endpoint / model | p50 | p75 | p90 | p95 | p99 | max | Result |
|---|---|---|---|---|---|---|---|
| **Direct → testserve** anthropic (baseline) | 1.40 | 1.71 | 2.02 | 2.60 | 3.20 | 4.06 | 300 × 200 |
| **Direct → testserve** openai embeddings (baseline) | 1.10 | 1.48 | 1.74 | 1.98 | 2.66 | 3.30 | 300 × 200 |
| `/v1/proxy` ts-claude (anthropic SSE) | 2.19 | 2.68 | 3.13 | 3.68 | 3.97 | 5.30 | 300 × 200 |
| `/v1/proxy` ts-gpt (openai SSE) | 1.99 | 2.58 | 3.04 | 3.46 | 4.47 | 4.87 | 300 × 200 |
| `/v1/proxy` ts-gemini (gemini SSE) | 2.05 | 2.62 | 3.09 | 3.67 | 4.08 | 4.36 | 300 × 200 |
| `/v1/proxy` ts-ollama | 2.21 | 2.58 | 2.88 | 3.44 | 4.43 | 4.93 | 300 × 200 |
| `/v1/proxy` ts-customdone | 2.03 | 2.57 | 3.10 | 3.67 | 4.36 | 5.04 | 300 × 200 |
| `/v1/embeddings` ts-embed | 1.75 | 2.32 | 2.63 | 2.80 | 3.18 | 3.65 | 300 × 200 |
| `/v1/embeddings` ts-embed-gemini | 1.82 | 2.29 | 2.82 | 3.18 | 3.90 | 4.14 | 300 × 200 |
| `/v1/generate` ts-image (base64) | 1.84 | 2.30 | 2.64 | 2.81 | 3.32 | 3.60 | 300 × 200 |
| `/v1/generate` ts-video (base64) | 1.94 | 2.48 | 2.83 | 3.14 | 3.69 | 4.08 | 300 × 200 |
| `/v1/generate` ts-video-url | 1.54 | 2.13 | 2.59 | 2.95 | 3.35 | 3.94 | 300 × 200 |
| `GET /v1/models` | 0.68 | 0.86 | 1.03 | 1.11 | 1.47 | 1.76 | 300 × 200 |
| `GET /v1/usage` | 0.64 | 0.80 | 0.96 | 1.10 | 1.31 | 1.66 | 300 × 200 |
| `GET /healthz` | 0.48 | 0.62 | 0.75 | 0.80 | 1.06 | 1.50 | 300 × 200 |

All latencies are in ms. At this rate Mesh CPU averaged 1–2 % (8 % peak) and RSS stayed around 25 MB.

**Mesh overhead** (Mesh ts-claude minus direct anthropic): **+0.79 ms p50, +1.08 ms p95, +0.77 ms p99**. For embeddings: +0.65 ms p50.

The same thing measured closed loop at 1 user against the provider rate limiter gives about 20 ms. That is not overhead: it is the 50 rps token bucket spacing requests 1/50 s apart (§5).

---

## 5. One provider under increasing load (ts-claude)

### 5a. N simultaneous users (closed loop, 15 s each)

| Users | Requests | Success | Throughput | p50 | p95 | p99 | max | Mesh CPU avg | Mesh RSS peak | Goroutines peak | Sockets :8090 / :9000 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 800 | 100 % | 53.3 rps | 19.9 ms | 21.4 | 22.2 | 23.6 | 6 % | 26 MB | 26 | 7 / 1 |
| 5 | 804 | 100 % | 53.3 | 99.9 | 101.6 | 102.6 | 103.3 | 6 % | 27 MB | 42 | 11 / 5 |
| 10 | 809 | 100 % | 53.3 | 199.8 | 201.8 | 202.7 | 203.7 | 6 % | 28 MB | 62 | 16 / 10 |
| 20 | 819 | 100 % | 53.2 | 399.9 | 401.4 | 402.6 | 403.3 | 6 % | 31 MB | 103 | 27 / 20 |
| 50 | 849 | 100 % | 53.1 | 999.8 | 1001.8 | 1002.6 | 1003.5 | 6 % | 33 MB | 164 | 56 / 21 |
| 100 | 899 | 100 % | 52.9 | 1999.8 | 2001.4 | 2002.2 | 2002.7 | 6 % | 36 MB | 264 | 106 / 21 |
| 200 | 999 | 100 % | 52.6 | 3999.4 | 4001.5 | 4002.3 | 4002.9 | 6 % | 40 MB | 461 | 203 / 21 |
| 400 | 1,202 | 99.6 % | 52.3 | 7979.3 | 7981.6 | 7999.6 | 8002.7 | 6 % | 53 MB | 659 | 401 / 21 |
| 800 | 2,802 | 42.7 % | 122 (all) / 52 OK | 7979.4 | 7981.8 | 8000.6 | 8020.1 | 6 % | 76 MB | 1,075 | 819 / 20 |

- Throughput is flat at **≈ 53 rps**: the 50 rps limit, plus the initial burst of 50 spread over 15 s.
- Latency scales exactly with users ÷ 50 rps, so Mesh adds no measurable slope of its own.
- Upstream sockets never exceed 21, which matches `max_concurrent_upstream = 20` plus one reused idle connection.
- **Above 200 users** the admission queue (capacity 200) is full. Extra requests wait 4 s for admission and then up to 4 s in the provider limiter, giving about **8 s max**. Past that point Mesh fails fast with `503 server_busy` rather than queueing forever: 1,604 rejections at 800 users.

### 5b. Fixed arrival rate (open loop, 15 s each)

| Arrival rate | Requests | Success | Goodput | p50 | p75 | p95 | p99 | max | Client in-flight peak |
|---|---|---|---|---|---|---|---|---|---|
| 10 rps | 150 | 100 % | 10 | 1.97 ms | 2.56 | 3.55 | 4.09 | 4.39 | 1 |
| 25 rps | 375 | 100 % | 25 | 2.15 | 2.62 | 3.44 | 4.13 | 4.46 | 1 |
| 40 rps | 600 | 100 % | 40 | 1.69 | 2.39 | 3.00 | 3.77 | 4.61 | 1 |
| **50 rps** | 750 | 100 % | 50 | **1.87** | 2.57 | 3.34 | 4.18 | 6.80 | 1 |
| 60 rps | 900 | 100 % | 52.9 | 519 | 1,269 | 1,870 | 1,990 | 2,019 | 102 |
| 75 rps | 1,125 | 100 % | 52.3 | 2,770 | 4,643 | 6,144 | 6,444 | 6,517 | 326 |
| 100 rps | 1,500 | 79.9 % | 52 | 5,013 | 7,992 | 7,994 | 7,995 | 8,003 | 601 |
| 150 rps | 2,250 | 53.2 % | 52 | 6,981 | 7,996 | 7,997 | 8,002 | 8,004 | 801 |

This is a textbook knee. Latency stays flat at about 2 ms all the way to **50 rps**. Beyond that the backlog grows and latency climbs toward the 8 s ceiling, after which Mesh sheds load with `503`.

---

## 6. Mixed realistic traffic (all six testserve providers)

### 6a. N simultaneous users (closed loop, 15 s each)

| Users | Requests | Success | Goodput | p50 | p75 | p95 | p99 | max | Mesh CPU avg / peak | Mesh RSS peak | Goroutines peak | Sockets :8090 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 10 | 2,451 | 100 % | 161 rps | 1.98 ms | 193 | 200 | 201 | 202 | 12 / 56 % | 37 MB | 57 | 11 |
| 50 | 2,473 | 100 % | 155 | 9.2 | 860 | 1,000 | 1,001 | 1,041 | 13 / 60 % | 44 MB | 217 | 51 |
| 100 | 2,719 | 100 % | 167 | 475 | 1,019 | 1,438 | 1,521 | 1,601 | 13 / 72 % | 52 MB | 365 | 101 |
| 250 | 2,686 | 100 % | 150 | 1,459 | 2,314 | 2,680 | 2,897 | 3,093 | 11 / 72 % | 58 MB | 573 | 261 |
| 500 | 3,061 | 100 % | 164 | 3,138 | 3,619 | 4,077 | 4,297 | 4,378 | 13 / 80 % | 74 MB | 861 | 505 |
| 1,000 | 4,488 | 73.3 % | 219 | 4,001 | 4,981 | 5,239 | 5,318 | 5,373 | 14 / 108 % | 99 MB | 1,517 | 1,019 |
| 2,000 | 8,585 | 40.9 % | 234 | 3,983 | 4,898 | 6,121 | 6,578 | 6,716 | 17 / 152 % | 155 MB | 2,622 | 2,016 |
| 5,000 | 21,005 | 17.0 % | 179 | 3,937 | 4,179 | 4,501 | 4,708 | 4,834 | 23 / 216 % | 362 MB | 6,035 | 5,001 |

### 6b. Fixed arrival rate (open loop, 15 s each)

| Arrival rate | Requests | Success | p50 | p75 | p95 | p99 | max | Mesh CPU avg | Mesh RSS peak | Sockets :8090 |
|---|---|---|---|---|---|---|---|---|---|---|
| 50 rps | 750 | 100 % | 1.63 ms | 2.44 | 3.41 | 4.11 | 5.07 | 4 % | 117 MB* | 13 |
| **100 rps** | 1,500 | 100 % | **1.58** | 2.33 | 3.25 | 3.88 | 9.70 | 9 % | 43 MB | 13 |
| 200 rps | 3,000 | 100 % | 942 | 2,127 | 3,659 | 4,342 | 4,543 | 13 % | 68 MB | 572 |
| 250 rps | 3,750 | 92.1 % | 2,739 | 3,999 | 6,690 | 6,891 | 6,942 | 15 % | 99 MB | 1,126 |
| 300 rps | 4,500 | 77.8 % | 3,915 | 5,180 | 6,402 | 6,599 | 6,659 | 18 % | 113 MB | 1,382 |
| 400 rps | 6,000 | 59.4 % | 3,999 | 5,496 | 6,475 | 6,622 | 6,679 | 19 % | 134 MB | 1,781 |
| 600 rps | 9,000 | 39.8 % | 4,001 | 5,299 | 6,775 | 6,894 | 6,938 | 24 % | 176 MB | 2,567 |

\* 117 MB was the heap left over from the preceding 2,000-user run. It fell to 43 MB during this one.

**Why the mix caps at about 160 rps rather than 300:** 33 % of the mix (ts-gpt 20 + ts-embed 10 + ts-image 3) lands on **testserve-openai**, and 50 rps ÷ 0.33 ≈ 150 rps. The busiest provider sets the ceiling for everyone, because the shared 200-unit admission queue fills with requests waiting on that one provider. With 1,000+ users, requests to *idle* providers also start getting `503 server_busy`.

---

## 7. Streaming, payloads and content types

### 7a. Payload size (ts-claude, 5 rps, 50 requests each)

| Request payload | p50 | p75 | p95 | p99 | max | Mesh CPU avg / peak | Mesh RSS peak | Admission cost peak |
|---|---|---|---|---|---|---|---|---|
| 1 KB text query | 2.22 ms | 2.54 | 2.95 | 3.49 | 3.49 | 1 / 8 % | 42 MB | 0–1 |
| 10 KB text | 2.54 | 2.94 | 3.78 | 5.40 | 5.40 | 1 / 8 % | 38 MB | 1 |
| 100 KB text | 3.96 | 4.71 | 7.88 | 11.1 | 11.1 | 2 / 4 % | 42 MB | 1 |
| 1 MB text | 18.9 | 24.2 | 30.7 | 43.7 | 43.7 | 11 / 28 % | 53 MB | 4 |
| 100 KB image block (base64) | 3.90 | 5.64 | 8.32 | 11.7 | 11.7 | 2 / 8 % | 51 MB | 6 |
| 1 MB image block | 26.3 | 30.2 | 65.7 | 88.9 | 88.9 | 14 / 56 % | 53 MB | 6 |
| 5 MB image block | 100.1 | 117.4 | 146.0 | 158.3 | 158.3 | 47 / 140 % | 126 MB | 26 |
| 512 KB PDF document block | 13.95 | 17.5 | 30.0 | 36.7 | 36.7 | 8 / 24 % | 102 MB | 9 |
| 1 MB image × **20 users** | 399 | 403 | 411 | 418 | 428 | 101 / 484 % | 167 MB | 120 |

Latency grows roughly linearly with payload size, at about **17–20 ms per MB** end-to-end. That covers JSON decode, base64 handling and re-encoding into the provider's content shape, plus testserve parsing the same bytes. 20 concurrent 1 MB images briefly used 167 MB of RSS, which dropped to 59 MB afterwards.

### 7b. Stream shapes

| Model / behaviour | Load | Success | TTFB p50 | Total p50 | p95 | p99 | Mesh CPU avg |
|---|---|---|---|---|---|---|---|
| ts-huge (~200× deltas per response) | 1 user | 100 % | 6.2 ms | 17.5 ms | 29.1 | 38.6 | 83 % |
| ts-huge | 10 users | 100 % | 188 | 199 | 212 | 219 | 92 % |
| ts-huge | 20 users | 100 % | 388 | 399 | 410 | 416 | 79 % |
| ts-unicode (emoji / CJK) | 10 rps | 100 % | 2.49 | 2.58 | 4.08 | 4.36 | 2 % |
| ts-keepalive (blank `data:` lines) | 10 rps | 100 % | 2.05 | 2.13 | 3.35 | 4.31 | 1 % |
| ts-empty (zero deltas) | 10 rps | 100 % | 2.21 | 2.21 | 3.39 | 3.82 | 1 % |
| ts-nousage (no usage fields) | 10 rps | 100 % | 2.03 | 2.04 | 3.60 | 3.99 | 1 % |

Every stream ended with Mesh's `{"done":true,…}` frame. TTFB is well below total time for ts-huge, which confirms Mesh **streams through without buffering** the whole response. Per-delta parsing is the main CPU cost of Mesh under streaming load: about 80–90 % of one core at about 55 rps of 200-delta streams.

---

## 8. Raw gateway capacity (paths with no provider limiter)

These isolate Mesh itself: TCP, middleware, key hashing and snapshot lookup, JSON encoding. The load generator shares the CPU, so these are lower bounds.

| Path | Users | Throughput | p50 | p75 | p95 | p99 | p99.9 | max | Mesh CPU avg | Mesh RSS peak | Goroutines peak |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `GET /healthz` | 1 | 8,889 rps | 0.07 ms | 0.11 | 0.35 | 0.64 | 0.95 | 2.26 | 71 % | 166 MB* | 20 |
| | 10 | 71,825 | 0.09 | 0.13 | 0.33 | 0.82 | 2.08 | 11.5 | 392 % | 41 MB | 36 |
| | 50 | 114,405 | 0.23 | 0.46 | 1.40 | 2.57 | 4.15 | 49.8 | 459 % | 44 MB | 99 |
| | **100** | **116,710** | 0.45 | 1.06 | 2.78 | 4.35 | 7.14 | 33.5 | 447 % | 45 MB | 246 |
| | 500 | 94,198 | 2.83 | 7.90 | 17.1 | 26.0 | 57.7 | 86.8 | 384 % | 55 MB | 682 |
| | 1,000 | 97,879 | 2.40 | 16.3 | 36.0 | 58.2 | 86.2 | 196 | 394 % | 77 MB | 1,519 |
| | 5,000 | 80,507 | 1.06 | 98.2 | 207 | 306 | 451 | 916 | 435 % | 258 MB | 7,426 |
| `GET /v1/models` (valid key) | 1 | 3,593 | 0.19 | 0.32 | 0.73 | 1.00 | 1.48 | 1.94 | 108 % | 38 MB | 19 |
| | 10 | 26,284 | 0.25 | 0.44 | 0.96 | 1.52 | 3.16 | 17.7 | 631 % | 42 MB | 36 |
| | 50 | 41,876 | 0.73 | 1.51 | 3.60 | 5.75 | 8.98 | 30.4 | 768 % | 55 MB | 116 |
| | 100 | 44,772 | 1.06 | 2.58 | 8.57 | 12.9 | 22.2 | 46.5 | 802 % | 56 MB | 213 |
| | 500 | 49,314 | 2.34 | 8.15 | 48.3 | 70.8 | 107 | 200 | 836 % | 90 MB | 957 |
| | **1,000** | **50,395** | 4.04 | 16.0 | 92.2 | 139 | 200 | 394 | 846 % | 164 MB | 1,902 |
| | 5,000 | 42,398 | 15.9 | 54.1 | 611 | 905 | 1,540 | 2,686 | 1,113 % | 399 MB | 7,753 |
| Invalid key → 401 | 1 | 6,471 | — | — | — | — | — | — | 71 % | 42 MB | 24 |
| | 100 | 96,089 | — | — | — | — | — | — | 502 % | 50 MB | 219 |
| | 500 | 96,640 | — | — | — | — | — | — | 528 % | 66 MB | 715 |
| | 1,000 | 77,487 | — | — | — | — | — | — | 472 % | 87 MB | 1,223 |

\* Heap left over from the preceding run. It fell to 41 MB within the run.

- `/v1/models` costs about 2× the CPU per request of `/healthz`. That extra is the per-request key hash plus snapshot resolution plus the JSON body (21 models), and it amounts to roughly **0.17 ms of CPU per authenticated request**.
- Past about 100–500 users, throughput plateaus and tail latency grows. That is CPU saturation shared with the load generator, not errors: **0 failures** on every row.
- Reject latency at 20 rps (§9): invalid key p50 0.63 ms / p99 1.16 ms, unknown model p50 0.62 ms / p99 1.25 ms.

---

## 9. Failure modes and rejections

### 9a. Upstream failure scenarios (5 users, 10 s)

| Scenario | Requests | Client sees | Latency | Upstream attempts per request | Notes |
|---|---|---|---|---|---|
| ts-500 (provider returns 500) | 554 | `502 upstream_unavailable` + `provider_status: 500` | p50 1.66 ms, p99 3.04 ms (at 20 rps) | **1** | No retry |
| ts-429 (provider returns 429 + Retry-After) | 554 | `502 upstream_unavailable` + `provider_status: 429` | p50 1.75 ms, p99 3.45 ms (at 20 rps) | **1** | No retry; Retry-After ignored |
| ts-flaky (first 2 calls fail) | 554 | 552 × 200, 2 × 502 | p50 99.8 ms (limiter-paced at 5 users) | 1 | The first 2 failures pass straight through |
| ts-reset (connection drops mid-stream) | 554 | 200 with partial deltas, then `{"done":true,"incomplete":true}` | p50 99.8 ms | 1 | Graceful partial result |
| ts-badjson (unparseable SSE) | 554 | 200, bad lines skipped, `done` frame sent | p50 99.9 ms | 1 | No crash |
| ts-slow (5 s upstream), 10 users | 40 | 100 % 200 | p50 5,004 ms, max 5,007 ms | 1 | Mesh adds ~4 ms on a 5 s call |
| ts-slow, **50 users** | 230 | 100 × 200, **130 × `503 provider_busy`** | 200s: 6.0–7.0 s | 1 | Only 20 upstream slots (`max_concurrent_upstream`). Waiters give up after 4 s |

(The p50 of ≈ 100 ms at 5 users is the 50 rps limiter spacing, 5 ÷ 50 s, not failure handling.)

### 9b. Latency of rejected requests

| Rejection | Status | p50 | p75 | p95 | p99 | max |
|---|---|---|---|---|---|---|
| Invalid / revoked key | 401 `invalid_or_revoked_key` | 0.63 ms | 0.76 | 1.01 | 1.16 | 2.05 |
| Model not granted to key | 403 `model_not_permitted` | 0.62 | 0.76 | 1.03 | 1.25 | 1.33 |
| Upstream 500 | 502 `upstream_unavailable` | 1.66 | 2.20 | 2.77 | 3.04 | 4.01 |
| Upstream 429 | 502 `upstream_unavailable` | 1.75 | 2.27 | 2.84 | 3.45 | 3.72 |
| Overload, admission queue full | 503 `server_busy` | 4,000 | 4,001 | 4,001 | 4,001 | 4,001 |
| Overload, provider limiter full | 503 `provider_busy` | ≈ 4,000 after admission | — | — | — | ~8,000 total |

---

## 10. Connections

| Test | Result |
|---|---|
| Max simultaneous client users tested | **5,000** (`/healthz`, `/v1/models`, mixed) |
| Peak ESTABLISHED sockets on Mesh :8090 | **7,363** (5,000 users on `/healthz`, including connections cycling) |
| Peak Mesh open FDs / goroutines | 7,394 FDs / 7,753 goroutines |
| Connection errors (refused / reset / EMFILE / timeouts) | **0** in every run |
| Mesh → testserve sockets | Peak 83 (mixed traffic, 6 providers × `max_concurrent_upstream` 20 = 120 cap); ≤ 21 for a single provider (pooled keep-alive) |
| Adaptive back-pressure | At 5,000 users goroutines passed 90 % of `QUEUE_GOROUTINE_CEILING` (5000), so the **queue load factor went 1 → 4**. Every request then cost 4× admission units, and the 200-unit queue admitted only ~50 text requests at a time. This, together with the provider limits, explains the 17 % success rate of the 5,000-user mix. It is Mesh protecting itself as designed |
| Memory per held connection | ≈ 50 KB RSS at 5,000 connections (≈ 258 MB for `/healthz`, ≈ 399 MB for `/v1/models`) |
| New TCP connection per request, `/v1/models`, 50 users | 28,736 rps, p50 1.40 ms, p99 5.69 ms (vs 41,876 rps / 0.73 ms with keep-alive) |
| New TCP connection per request, ts-claude, 40 rps | p50 2.52 ms, p99 4.18 ms (vs 1.69 / 3.77 ms with keep-alive), so **≈ +0.5–0.8 ms per new connection** |

---

## 11. Soak tests and resource behaviour over time

| Soak | Duration | Requests | Success | p50 | p75 | p95 | p99 | max | Mesh CPU avg / peak | Mesh RSS start → peak → end | Goroutines peak |
|---|---|---|---|---|---|---|---|---|---|---|---|
| **Mix at 140 rps (below capacity)** | 180 s | 25,200 | **100 %** | **1.65 ms** | 2.36 | 3.21 | **3.75** | 6.79 | 12 / 32 % | 93.8 → 94.0 → 54.9 MB | 22 |
| Mix at 200 rps (above capacity) | 120 s | 24,000 | 84.1 % | 4,001 | 5,519 | 7,852 | 7,943 | 7,999 | 15 / 56 % | 64 → 109 → 94 MB | 1,379 |

- 25,200 requests in 3 minutes with flat latency, flat memory and flat goroutines, with no drift over time.
- **Overall resource envelope across all tests:** Mesh idle at 25–28 MB / 21 threads / ~18 goroutines. Normal load (≤ 200 rps chat) at 30–60 MB and 5–15 % of one core. Worst case of 5,000 simultaneous connections at 399 MB / about 11 cores (shared with the load generator). Mesh never grew beyond 28 OS threads.
- After all tests, goroutines were back to 17–31 and RSS to 55–95 MB (the Go GC returns heap to the OS lazily). **No leaks.**

---

## 12. Telemetry and database

| Counter (end of campaign) | Value |
|---|---|
| `mesh_telemetry_written_total` | **132,611 rows** written to `request_logs` |
| `mesh_telemetry_dropped_denied_total` | **4,192,838** denial events dropped |
| `mesh_telemetry_dropped_success_total` | 0 |
| `mesh_telemetry_insert_failures_total` | 0 |
| Gateway DB pool | max 16, only 3 connections ever opened, 1 empty-acquire |

- `log_requests` is **off** on all testserve providers, so successful calls never touched Postgres. The latencies above therefore reflect Mesh's in-memory data path only, as the README describes.
- **Denials (401 / 403 / 503) are always logged**, regardless of `log_requests`. During the invalid-key floods (up to 97k rps) the 4,096-slot denial channel overflowed. Mesh dropped the excess (by design: non-blocking) instead of slowing requests down, which is why 4.19 M events were dropped. It still wrote about 132k rows at a sustained rate to the remote Supabase database with zero insert failures.
- ⚠️ **Side effect:** those ≈ 132,600 denial rows (`invalid_or_revoked_key`, `model_not_permitted`, `server_busy`, `provider_busy` for the testserve user and unknown keys) are now in your `request_logs` table and will show in the admin Logs page. They will age out under `LOG_RETENTION_DAYS` (default 30), or you can delete them for today's time window.

---

## 13. Correctness of the rewritten upstream request

Testserve inspected every request Mesh sent it (1,150 tagged requests across the anthropic and openai shapes, plus spot checks):

| Check | Result |
|---|---|
| Provider key sent in header (`x-api-key` / `Authorization`) | ✅ always present |
| Provider key leaked into the request body | ✅ **0 / 1,150** |
| Unrendered `{{placeholder}}` left in the body | ✅ **0 / 1,150** |
| Model rewrite (`ts-claude` → upstream `dummy-chat`) | ✅ correct |
| Content converted to provider shape (`{"query"}` → `messages[0].content[{type:text}]`, `max_tokens` default 4096) | ✅ correct |
| Streaming frames re-emitted as Mesh NDJSON with usage | ✅ every 2xx stream ended with `done` + usage |

---

## 14. Findings and bottlenecks

1. **Mesh itself is not the bottleneck.** Its data-path overhead is about 1 ms each way, it streams without buffering, and it handles 50k authenticated rps / 117k rps health checks on a laptop shared with the load generator.
2. **Throughput is set entirely by provider limits.** 50 rps per provider (a token bucket with burst 50) means about 53 rps sustained per provider. The six testserve providers could in theory reach 300 rps together, but real mixes cap at the rate of the busiest provider (≈ 160 rps here).
3. **One saturated provider can starve the others.** The admission queue (200 units) is global, so requests piling up behind one provider fill it and cause `503 server_busy` on providers that still have headroom (seen at ≥ 1,000 users in the mix).
4. **Worst-case wait is about 8 s before a 503**: up to 4 s for admission plus up to 4 s for the provider limiter (`QUEUE_ADMIT_TIMEOUT_MS = 4000` applies to both).
5. **Retries only cover transport errors.** Upstream HTTP 500 and 429 are passed straight through as `502` with one attempt, even with `retry_enabled` and `max_retries = 3`, and `Retry-After` is not honoured (`server/internal/gateway/adapter.go`, `doUpstreamRequestWithRetry`). This is noted as observed behaviour only; nothing was changed.
6. **The denial telemetry channel overflows under floods** (4.19 M drops). That is by design, but it means a bad-key attack is under-counted in the logs.
7. **Memory scales with open connections** (≈ 50 KB each) and with in-flight payload size. The 5 MB image test peaked at 126 MB RSS, and the heap never approached `QUEUE_HEAP_CEILING_MB` (1024). The **goroutine ceiling** did engage: at 5,000 simultaneous users goroutines exceeded 4,500 (90 % of 5,000), and the admission load factor jumped to 4×, cutting admitted concurrency to a quarter. On a busy real deployment, `QUEUE_GOROUTINE_CEILING` is the first self-protection limit you will hit. Each held client connection costs about 1–1.5 goroutines.
8. **testserve's limits (not Mesh's):** one Python worker running with `--reload`. It reached ≈ 1 GB RSS because its debug log retains the last 500 request bodies, including the 5 MB images. It was never the throughput limiter here.

---

## Appendix A — Methodology notes and caveats

- Latency is wall-clock from the client socket: TTFB is the first body byte, total is the last byte. Percentiles use nearest-rank. The main tables count successful requests only; §9b covers rejects.
- CPU % is sampled from `/proc/<pid>/stat` every 250 ms (100 % = one core). A single 250 ms window can overshoot 1,600 % from tick rounding, so treat **averages** as authoritative and peaks as indicative.
- The "phase" split in §3 compares the client send time with testserve's arrival timestamp on the same host clock. It carries about ±0.1 ms of clock-read jitter.
- Mesh ran as the air dev build (`go build`, no PGO), not a production binary. The frontend dev server was also running.
- The hang scenario (`ts-hang`, 1 h) was not run, because it would only hold a connection until the generation timeout.
- Every test used the one existing `testserve` gateway key, so per-key/per-user limits (`calls_per_hour_limit` = none) were not a factor.

## Appendix B — Full raw result tables

Every run, with all percentiles (p50, p75, p90, p95, p99, p99.9, max), status breakdowns and the full resource sample.

### P0 — Idle baseline

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Idle | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| idle | 10 | 100 | 1 | 0.51 | 0.51 | 0.63 | 0.67 | 0.75 | 0.75 | 0.75 | 0.75 | 200:10 | — |

Resources (sampled every 250 ms during the run).

| Idle | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| idle | 0 | 4 | 27.3 → 27.3 → 25.0 | 21 | 14 | 20 | 0 | 0 | 0/4 | 87.0 | 2 | 1 | 1 |

### P1 — Per-endpoint latency (open loop, 20 rps, 15 s each)

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Endpoint | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| direct_anthropic | 300 | 100 | 20 | 1.4 | 1.4 | 1.71 | 2.02 | 2.6 | 3.2 | 4.06 | 4.06 | 200:300 | — |
| direct_openai_embed | 300 | 100 | 20 | 1.1 | 1.1 | 1.48 | 1.74 | 1.98 | 2.66 | 3.3 | 3.3 | 200:300 | — |
| chat_ts-claude | 300 | 100 | 20 | 2.17 | 2.19 | 2.68 | 3.13 | 3.68 | 3.97 | 5.3 | 5.3 | 200:300 | — |
| chat_ts-gpt | 300 | 100 | 20 | 1.95 | 1.99 | 2.58 | 3.04 | 3.46 | 4.47 | 4.87 | 4.87 | 200:300 | — |
| chat_ts-gemini | 300 | 100 | 20 | 2 | 2.05 | 2.62 | 3.09 | 3.67 | 4.08 | 4.36 | 4.36 | 200:300 | — |
| chat_ts-ollama | 300 | 100 | 20 | 2.15 | 2.21 | 2.58 | 2.88 | 3.44 | 4.43 | 4.93 | 4.93 | 200:300 | — |
| chat_ts-customdone | 300 | 100 | 20 | 1.98 | 2.03 | 2.57 | 3.1 | 3.67 | 4.36 | 5.04 | 5.04 | 200:300 | — |
| embed_ts-embed | 300 | 100 | 20 | 1.75 | 1.75 | 2.32 | 2.63 | 2.8 | 3.18 | 3.65 | 3.65 | 200:300 | — |
| embed_ts-embed-gemini | 300 | 100 | 20 | 1.82 | 1.82 | 2.29 | 2.82 | 3.18 | 3.9 | 4.14 | 4.14 | 200:300 | — |
| gen_ts-image | 300 | 100 | 20 | 1.84 | 1.84 | 2.3 | 2.64 | 2.81 | 3.32 | 3.6 | 3.6 | 200:300 | — |
| gen_ts-video | 300 | 100 | 20 | 1.94 | 1.94 | 2.48 | 2.83 | 3.14 | 3.69 | 4.08 | 4.08 | 200:300 | — |
| gen_ts-video-url | 300 | 100 | 20 | 1.54 | 1.54 | 2.13 | 2.59 | 2.95 | 3.35 | 3.94 | 3.94 | 200:300 | — |
| models | 300 | 100 | 20 | 0.68 | 0.68 | 0.86 | 1.03 | 1.11 | 1.47 | 1.76 | 1.76 | 200:300 | — |
| usage | 300 | 100 | 20 | 0.63 | 0.64 | 0.8 | 0.96 | 1.1 | 1.31 | 1.66 | 1.66 | 200:300 | — |
| healthz | 300 | 100 | 20 | 0.48 | 0.48 | 0.62 | 0.75 | 0.8 | 1.06 | 1.5 | 1.5 | 200:300 | — |
| badkey | 300 | 0 | 20 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:300 | invalid_or_revoked_key:300 |
| forbidden_model | 300 | 0 | 20 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 403:300 | model_not_permitted:300 |

Resources (sampled every 250 ms during the run).

| Endpoint | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| direct_anthropic | 0 | 4 | 25.0 → 25.0 → 24.2 | 21 | 13 | 19 | 0 | 0 | 3/12 | 87.0 | 2 | 2 | 1 |
| direct_openai_embed | 0 | 4 | 24.2 → 24.5 → 24.5 | 21 | 13 | 19 | 0 | 0 | 2/8 | 87.0 | 2 | 1 | 1 |
| chat_ts-claude | 2 | 8 | 24.5 → 25.5 → 25.5 | 21 | 14 | 20 | 1 | 1 | 3/8 | 87.0 | 2 | 1 | 1 |
| chat_ts-gpt | 2 | 8 | 25.5 → 25.5 → 25.2 | 21 | 14 | 21 | 1 | 1 | 3/8 | 87.0 | 2 | 1 | 1 |
| chat_ts-gemini | 2 | 8 | 25.2 → 25.3 → 25.2 | 21 | 14 | 20 | 1 | 1 | 3/8 | 87.0 | 2 | 1 | 1 |
| chat_ts-ollama | 2 | 8 | 25.2 → 25.3 → 25.0 | 21 | 14 | 21 | 1 | 1 | 3/8 | 87.0 | 2 | 1 | 1 |
| chat_ts-customdone | 2 | 8 | 25.0 → 25.2 → 25.1 | 21 | 14 | 20 | 1 | 1 | 3/8 | 87.0 | 2 | 1 | 1 |
| embed_ts-embed | 2 | 8 | 25.1 → 25.2 → 24.8 | 21 | 14 | 20 | 1 | 1 | 2/12 | 87.0 | 2 | 1 | 1 |
| embed_ts-embed-gemini | 2 | 8 | 24.8 → 25.0 → 24.6 | 21 | 14 | 21 | 1 | 1 | 2/8 | 87.0 | 2 | 1 | 1 |
| gen_ts-image | 2 | 8 | 24.6 → 24.9 → 24.9 | 21 | 14 | 20 | 25 | 1 | 2/8 | 87.0 | 2 | 1 | 1 |
| gen_ts-video | 2 | 8 | 24.9 → 24.9 → 24.9 | 21 | 14 | 21 | 25 | 1 | 2/8 | 87.0 | 2 | 1 | 1 |
| gen_ts-video-url | 1 | 8 | 24.9 → 24.9 → 24.9 | 21 | 14 | 20 | 25 | 1 | 2/8 | 87.0 | 2 | 1 | 1 |
| models | 1 | 8 | 24.9 → 25.2 → 24.8 | 21 | 14 | 19 | 0 | 0 | 0/4 | 87.0 | 2 | 1 | 1 |
| usage | 1 | 8 | 24.8 → 24.8 → 24.5 | 21 | 13 | 18 | 0 | 0 | 0/4 | 87.0 | 2 | 0 | 1 |
| healthz | 1 | 4 | 24.5 → 24.5 → 24.5 | 21 | 13 | 18 | 0 | 0 | 0/4 | 87.0 | 2 | 0 | 1 |
| badkey | 1 | 4 | 24.5 → 25.2 → 25.2 | 21 | 13 | 20 | 0 | 0 | 0/4 | 87.0 | 2 | 0 | 1 |
| forbidden_model | 1 | 4 | 25.2 → 25.4 → 25.4 | 21 | 19 | 31 | 0 | 0 | 0/4 | 87.0 | 7 | 0 | 1 |

### P2 — Single provider (ts-claude), closed loop, N simultaneous users, 15 s each

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Users | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c1 | 800 | 100 | 53.3 | 19.92 | 19.94 | 20.48 | 20.93 | 21.41 | 22.19 | 23.6 | 23.6 | 200:800 | — |
| c5 | 804 | 100 | 53.3 | 99.85 | 99.86 | 100.45 | 101.11 | 101.57 | 102.62 | 103.33 | 103.33 | 200:804 | — |
| c10 | 809 | 100 | 53.3 | 199.8 | 199.82 | 200.68 | 201.38 | 201.83 | 202.66 | 203.7 | 203.7 | 200:809 | — |
| c20 | 819 | 100 | 53.2 | 399.88 | 399.89 | 400.49 | 401.04 | 401.36 | 402.59 | 403.25 | 403.25 | 200:819 | — |
| c50 | 849 | 100 | 53.1 | 999.77 | 999.79 | 1000.64 | 1001.37 | 1001.78 | 1002.62 | 1003.47 | 1003.47 | 200:849 | — |
| c100 | 899 | 100 | 52.9 | 1999.75 | 1999.78 | 2000.46 | 2001.12 | 2001.42 | 2002.15 | 2002.69 | 2002.69 | 200:899 | — |
| c200 | 999 | 100 | 52.6 | 3999.39 | 3999.43 | 4000.31 | 4000.97 | 4001.49 | 4002.29 | 4002.89 | 4002.89 | 200:999 | — |
| c400 | 1202 | 99.58 | 52.3 | 7979.23 | 7979.29 | 7980.36 | 7981.11 | 7981.61 | 7999.59 | 8001.72 | 8002.73 | 200:1197, 503:5 | server_busy:4, provider_busy:1 |
| c800 | 2802 | 42.68 | 122 | 7979.37 | 7979.42 | 7980.5 | 7981.28 | 7981.83 | 8000.64 | 8003.21 | 8020.06 | 200:1196, 503:1606 | server_busy:1604, provider_busy:2 |

Resources (sampled every 250 ms during the run).

| Users | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c1 | 6 | 20 | 25.4 → 26.2 → 26.0 | 21 | 20 | 26 | 1 | 1 | 6/20 | 87.0 | 7 | 1 | 1 |
| c5 | 6 | 16 | 26.0 → 27.2 → 26.5 | 21 | 28 | 42 | 5 | 1 | 6/20 | 87.0 | 11 | 5 | 5 |
| c10 | 6 | 28 | 26.5 → 28.4 → 26.5 | 21 | 38 | 62 | 10 | 1 | 6/16 | 87.0 | 16 | 10 | 10 |
| c20 | 6 | 24 | 26.5 → 30.7 → 27.6 | 21 | 59 | 103 | 20 | 1 | 6/20 | 87.0 | 27 | 20 | 20 |
| c50 | 6 | 28 | 27.6 → 33.1 → 29.0 | 21 | 88 | 164 | 50 | 1 | 6/20 | 87.0 | 56 | 21 | 50 |
| c100 | 6 | 28 | 29.0 → 36.4 → 31.5 | 21 | 138 | 264 | 100 | 1 | 6/20 | 87.0 | 106 | 21 | 100 |
| c200 | 6 | 28 | 31.5 → 40.3 → 34.7 | 21 | 235 | 461 | 200 | 1 | 6/24 | 87.0 | 203 | 21 | 200 |
| c400 | 6 | 39 | 34.7 → 52.6 → 41.5 | 21 | 433 | 659 | 200 | 1 | 6/20 | 87.0 | 401 | 21 | 400 |
| c800 | 6 | 48 | 41.5 → 75.8 → 60.0 | 21 | 850 | 1075 | 200 | 1 | 6/32 | 87.0 | 819 | 20 | 800 |

### P3 — Single provider (ts-claude), open loop, fixed arrival rate, 15 s each

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Arrival rate | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| rps10 | 150 | 100 | 10 | 1.9 | 1.97 | 2.56 | 3.05 | 3.55 | 4.09 | 4.39 | 4.39 | 200:150 | — |
| rps25 | 375 | 100 | 25 | 2.11 | 2.15 | 2.62 | 3.09 | 3.44 | 4.13 | 4.46 | 4.46 | 200:375 | — |
| rps40 | 600 | 100 | 40 | 1.66 | 1.69 | 2.39 | 2.7 | 3 | 3.77 | 4.61 | 4.61 | 200:600 | — |
| rps50 | 750 | 100 | 50 | 1.83 | 1.87 | 2.57 | 3.05 | 3.34 | 4.18 | 6.8 | 6.8 | 200:750 | — |
| rps60 | 900 | 100 | 52.9 | 519.37 | 519.37 | 1269.29 | 1720.86 | 1870.33 | 1989.81 | 2019.31 | 2019.31 | 200:900 | — |
| rps75 | 1125 | 100 | 52.3 | 2770.39 | 2770.39 | 4643.37 | 5768.64 | 6144.18 | 6444.01 | 6509.56 | 6516.66 | 200:1125 | — |
| rps100 | 1500 | 79.93 | 65.3 | 5012.62 | 5012.74 | 7991.81 | 7993.58 | 7994.15 | 7995.29 | 8002.66 | 8002.91 | 200:1199, 503:301 | server_busy:301 |
| rps150 | 2250 | 53.16 | 97.9 | 6980.92 | 6980.96 | 7995.52 | 7996.41 | 7996.87 | 8002.04 | 8003.58 | 8003.66 | 200:1196, 503:1054 | server_busy:1051, provider_busy:3 |

Resources (sampled every 250 ms during the run).

| Arrival rate | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| rps10 | 1 | 8 | 60.0 → 60.1 → 47.2 | 21 | 14 | 20 | 1 | 1 | 1/4 | 87.0 | 2 | 1 | 1 |
| rps25 | 3 | 8 | 47.2 → 34.9 → 32.8 | 21 | 14 | 20 | 0 | 0 | 3/12 | 87.0 | 2 | 1 | 1 |
| rps40 | 4 | 12 | 32.8 → 32.8 → 32.4 | 21 | 14 | 20 | 0 | 0 | 5/16 | 87.0 | 2 | 1 | 1 |
| rps50 | 5 | 12 | 32.4 → 32.4 → 32.0 | 21 | 14 | 20 | 1 | 1 | 6/20 | 87.0 | 2 | 1 | 1 |
| rps60 | 6 | 20 | 32.0 → 35.1 → 34.9 | 21 | 115 | 220 | 100 | 1 | 7/20 | 87.0 | 103 | 1 | 102 |
| rps75 | 6 | 20 | 34.9 → 45.8 → 40.8 | 21 | 339 | 544 | 200 | 1 | 6/20 | 87.0 | 327 | 1 | 326 |
| rps100 | 7 | 24 | 40.8 → 58.8 → 53.5 | 21 | 614 | 820 | 200 | 1 | 7/28 | 87.0 | 602 | 1 | 601 |
| rps150 | 8 | 28 | 53.5 → 72.5 → 62.2 | 21 | 814 | 1020 | 200 | 1 | 6/28 | 87.0 | 802 | 1 | 801 |

### P4 — Mixed realistic traffic, closed loop, N simultaneous users, 15 s each

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Users | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c10 | 2451 | 100 | 161.4 | 1.91 | 1.98 | 193.45 | 199.19 | 200.19 | 201.34 | 202.02 | 202.44 | 200:2451 | — |
| c50 | 2473 | 100 | 154.7 | 9.02 | 9.2 | 859.87 | 997.84 | 999.67 | 1001.29 | 1020.59 | 1040.64 | 200:2473 | — |
| c100 | 2719 | 100 | 167 | 475.18 | 475.2 | 1019.08 | 1334.77 | 1437.78 | 1520.63 | 1599.21 | 1601.25 | 200:2719 | — |
| c250 | 2686 | 100 | 150.2 | 1458.82 | 1458.83 | 2313.5 | 2572.37 | 2679.73 | 2896.5 | 3021.55 | 3092.93 | 200:2686 | — |
| c500 | 3061 | 100 | 163.8 | 3138.02 | 3138.02 | 3619.4 | 3996.91 | 4077.28 | 4296.6 | 4340.04 | 4378.2 | 200:3061 | — |
| c1000 | 4488 | 73.26 | 223.1 | 4001.13 | 4001.15 | 4980.99 | 5180.61 | 5238.88 | 5317.63 | 5340.89 | 5373.21 | 200:3288, 503:1200 | server_busy:1200 |
| c2000 | 8585 | 40.94 | 395.7 | 3983.35 | 3983.35 | 4897.67 | 5739.25 | 6120.77 | 6578.47 | 6678.95 | 6716.07 | 200:3515, 503:5070 | server_busy:5070 |

Resources (sampled every 250 ms during the run).

| Users | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c10 | 12 | 56 | 62.2 → 37.3 → 33.4 | 21 | 32 | 57 | 82 | 1 | 14/36 | 87.0 | 11 | 10 | 10 |
| c50 | 13 | 60 | 33.4 → 44.4 → 37.5 | 21 | 112 | 217 | 194 | 3 | 15/44 | 87.1 | 51 | 50 | 50 |
| c100 | 13 | 72 | 37.5 → 51.9 → 38.0 | 21 | 194 | 365 | 200 | 2 | 13/44 | 87.1 | 101 | 82 | 100 |
| c250 | 11 | 72 | 38.0 → 58.2 → 51.4 | 21 | 349 | 573 | 200 | 17 | 11/40 | 87.2 | 261 | 77 | 250 |
| c500 | 13 | 80 | 51.5 → 74.0 → 58.2 | 23 | 585 | 861 | 200 | 14 | 14/44 | 87.2 | 505 | 73 | 500 |
| c1000 | 14 | 108 | 58.2 → 99.3 → 79.9 | 23 | 1115 | 1517 | 200 | 2 | 13/60 | 87.2 | 1019 | 83 | 1000 |
| c2000 | 17 | 152 | 79.9 → 154.5 → 116.9 | 23 | 2106 | 2622 | 200 | 8 | 14/108 | 87.2 | 2016 | 77 | 2000 |

### P5 — Mixed realistic traffic, open loop, fixed arrival rate, 15 s each

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Arrival rate | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| rps50 | 750 | 100 | 50 | 1.59 | 1.63 | 2.44 | 2.97 | 3.41 | 4.11 | 5.07 | 5.07 | 200:750 | — |
| rps100 | 1500 | 100 | 100 | 1.53 | 1.58 | 2.33 | 2.92 | 3.25 | 3.88 | 5.24 | 9.7 | 200:1500 | — |
| rps200 | 3000 | 100 | 153.9 | 941.9 | 941.95 | 2126.96 | 3006.69 | 3658.99 | 4341.91 | 4522.64 | 4543.06 | 200:3000 | — |
| rps250 | 3750 | 92.05 | 171.9 | 2738.88 | 2738.88 | 3998.93 | 5547.3 | 6690.25 | 6890.67 | 6934.41 | 6941.74 | 200:3452, 503:298 | server_busy:298 |
| rps300 | 4500 | 77.76 | 208.5 | 3915.41 | 3915.47 | 5180.45 | 6235.62 | 6401.69 | 6599.36 | 6652.73 | 6659.26 | 200:3499, 503:1001 | server_busy:1001 |
| rps400 | 6000 | 59.38 | 278 | 3998.63 | 3998.7 | 5495.57 | 6377.82 | 6474.6 | 6621.79 | 6670.55 | 6678.65 | 200:3563, 503:2437 | server_busy:2437 |
| rps600 | 9000 | 39.79 | 417 | 4000.72 | 4000.79 | 5299.04 | 6694.89 | 6775.28 | 6894.28 | 6930 | 6937.72 | 200:3581, 503:5419 | server_busy:5419 |

Resources (sampled every 250 ms during the run).

| Arrival rate | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| rps50 | 4 | 12 | 116.9 → 117.2 → 43.5 | 23 | 29 | 41 | 25 | 1 | 6/16 | 87.2 | 13 | 2 | 1 |
| rps100 | 9 | 24 | 43.1 → 43.3 → 42.7 | 23 | 29 | 31 | 1 | 1 | 11/24 | 87.2 | 13 | 1 | 2 |
| rps200 | 13 | 36 | 42.7 → 67.8 → 60.8 | 23 | 611 | 773 | 200 | 10 | 13/40 | 87.2 | 572 | 24 | 560 |
| rps250 | 15 | 40 | 60.8 → 99.2 → 80.6 | 23 | 1153 | 1491 | 200 | 4 | 15/48 | 87.2 | 1126 | 27 | 1114 |
| rps300 | 18 | 64 | 80.6 → 112.8 → 86.6 | 23 | 1416 | 1876 | 200 | 15 | 17/80 | 87.2 | 1382 | 25 | 1370 |
| rps400 | 19 | 72 | 86.6 → 133.5 → 114.4 | 23 | 1804 | 2334 | 200 | 2 | 16/96 | 87.2 | 1781 | 27 | 1770 |
| rps600 | 24 | 96 | 114.4 → 176.2 → 165.5 | 23 | 2584 | 3222 | 200 | 2 | 17/76 | 87.2 | 2567 | 18 | 2566 |

### P6 — Gateway paths without a provider limiter (raw Mesh capacity), 10 s each

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Path / users | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| healthz_c1 | 88886 | 100 | 8888.6 | 0.07 | 0.07 | 0.11 | 0.22 | 0.35 | 0.64 | 0.95 | 2.26 | 200:88886 | — |
| healthz_c10 | 718262 | 100 | 71824.6 | 0.09 | 0.09 | 0.13 | 0.2 | 0.33 | 0.82 | 2.08 | 11.47 | 200:718262 | — |
| healthz_c50 | 1144168 | 100 | 114404.5 | 0.23 | 0.23 | 0.46 | 0.97 | 1.4 | 2.57 | 4.15 | 49.83 | 200:1144168 | — |
| healthz_c100 | 1167291 | 100 | 116709.9 | 0.45 | 0.45 | 1.06 | 2.03 | 2.78 | 4.35 | 7.14 | 33.52 | 200:1167291 | — |
| healthz_c500 | 942116 | 100 | 94197.8 | 2.83 | 2.83 | 7.9 | 13.23 | 17.07 | 25.96 | 57.68 | 86.76 | 200:942116 | — |
| healthz_c1000 | 979507 | 100 | 97878.8 | 2.4 | 2.4 | 16.31 | 28.65 | 36.02 | 58.23 | 86.22 | 196.04 | 200:979507 | — |
| models_c1 | 35930 | 100 | 3593 | 0.19 | 0.19 | 0.32 | 0.59 | 0.73 | 1 | 1.48 | 1.94 | 200:35930 | — |
| models_c10 | 262877 | 100 | 26283.9 | 0.25 | 0.25 | 0.44 | 0.74 | 0.96 | 1.52 | 3.16 | 17.69 | 200:262877 | — |
| models_c50 | 418813 | 100 | 41876.1 | 0.73 | 0.73 | 1.51 | 2.64 | 3.6 | 5.75 | 8.98 | 30.35 | 200:418813 | — |
| models_c100 | 447826 | 100 | 44772.4 | 1.05 | 1.06 | 2.58 | 6.1 | 8.57 | 12.85 | 22.17 | 46.54 | 200:447826 | — |
| models_c500 | 493741 | 100 | 49314.2 | 2.31 | 2.34 | 8.15 | 38.49 | 48.29 | 70.8 | 106.83 | 200.43 | 200:493741 | — |
| models_c1000 | 505640 | 100 | 50394.8 | 4 | 4.04 | 16 | 76.87 | 92.16 | 138.98 | 199.5 | 393.68 | 200:505640 | — |
| badkey_c1 | 64709 | 0 | 6470.9 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:64709 | invalid_or_revoked_key:64709 |
| badkey_c10 | 656254 | 0 | 65624.7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:656254 | invalid_or_revoked_key:656254 |
| badkey_c50 | 893627 | 0 | 89360.3 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:893627 | invalid_or_revoked_key:893627 |
| badkey_c100 | 966463 | 0 | 96089.2 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:966463 | invalid_or_revoked_key:966463 |
| badkey_c500 | 966995 | 0 | 96640.1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:966995 | invalid_or_revoked_key:966995 |
| badkey_c1000 | 776401 | 0 | 77487.1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 401:776401 | invalid_or_revoked_key:776401 |

Resources (sampled every 250 ms during the run).

| Path / users | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| healthz_c1 | 71 | 180 | 165.5 → 165.5 → 40.8 | 23 | 14 | 20 | 0 | 0 | 0/4 | 87.2 | 2 | 2 | 1 |
| healthz_c10 | 392 | 1040 | 40.8 → 39.8 → 38.8 | 23 | 22 | 36 | 0 | 0 | 0/4 | 87.2 | 11 | 0 | 10 |
| healthz_c50 | 459 | 1212 | 38.8 → 44.2 → 41.7 | 26 | 71 | 99 | 0 | 0 | 0/4 | 87.2 | 60 | 0 | 50 |
| healthz_c100 | 447 | 1160 | 41.7 → 45.3 → 42.2 | 26 | 147 | 246 | 0 | 0 | 0/4 | 87.2 | 136 | 0 | 100 |
| healthz_c500 | 384 | 936 | 42.2 → 55.0 → 52.7 | 26 | 556 | 682 | 0 | 0 | 0/4 | 87.2 | 545 | 0 | 500 |
| healthz_c1000 | 394 | 1004 | 52.7 → 77.4 → 77.4 | 26 | 1370 | 1519 | 0 | 0 | 0/4 | 87.2 | 1359 | 0 | 1000 |
| models_c1 | 108 | 268 | 77.4 → 38.3 → 38.1 | 26 | 13 | 19 | 0 | 0 | 0/4 | 87.2 | 2 | 0 | 1 |
| models_c10 | 631 | 1492 | 38.1 → 42.2 → 40.8 | 26 | 22 | 36 | 0 | 0 | 0/4 | 87.2 | 11 | 0 | 10 |
| models_c50 | 768 | 1856 | 40.8 → 54.9 → 47.9 | 26 | 64 | 116 | 0 | 0 | 0/4 | 87.2 | 53 | 0 | 50 |
| models_c100 | 802 | 1880 | 47.9 → 56.0 → 54.7 | 26 | 117 | 213 | 0 | 0 | 0/4 | 87.2 | 106 | 0 | 100 |
| models_c500 | 836 | 2120 | 54.7 → 89.7 → 79.8 | 26 | 523 | 957 | 0 | 0 | 0/4 | 87.2 | 512 | 0 | 500 |
| models_c1000 | 846 | 2300 | 62.8 → 163.8 → 116.9 | 27 | 1159 | 1902 | 0 | 0 | 0/4 | 87.2 | 1148 | 0 | 1000 |
| badkey_c1 | 71 | 192 | 112.2 → 112.2 → 41.8 | 27 | 13 | 24 | 0 | 0 | 0/4 | 87.2 | 2 | 0 | 1 |
| badkey_c10 | 435 | 1104 | 43.2 → 44.7 → 41.2 | 27 | 22 | 32 | 0 | 0 | 0/4 | 87.2 | 11 | 0 | 10 |
| badkey_c50 | 487 | 1320 | 43.0 → 50.1 → 47.1 | 27 | 75 | 89 | 0 | 0 | 0/4 | 87.2 | 64 | 0 | 50 |
| badkey_c100 | 502 | 1308 | 41.6 → 49.8 → 47.1 | 27 | 121 | 219 | 0 | 0 | 0/4 | 87.2 | 110 | 0 | 100 |
| badkey_c500 | 528 | 1332 | 43.4 → 66.3 → 64.9 | 27 | 520 | 715 | 0 | 0 | 0/4 | 87.2 | 509 | 0 | 500 |
| badkey_c1000 | 472 | 1192 | 43.6 → 87.1 → 83.4 | 28 | 1014 | 1223 | 0 | 0 | 0/4 | 87.2 | 1003 | 0 | 1000 |

### P7 — Payload size (open loop 5 rps, 10 s; last row 20 users)

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Payload | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| payload_1k | 50 | 100 | 5 | 2.21 | 2.22 | 2.54 | 2.83 | 2.95 | 3.49 | 3.49 | 3.49 | 200:50 | — |
| payload_10k | 50 | 100 | 5 | 2.54 | 2.54 | 2.94 | 3.36 | 3.78 | 5.4 | 5.4 | 5.4 | 200:50 | — |
| payload_100k | 50 | 100 | 5 | 3.78 | 3.96 | 4.71 | 6.76 | 7.88 | 11.1 | 11.1 | 11.1 | 200:50 | — |
| payload_1024k | 50 | 100 | 5 | 18.9 | 18.9 | 24.16 | 29.8 | 30.68 | 43.73 | 43.73 | 43.73 | 200:50 | — |
| image_100k | 50 | 100 | 5 | 3.83 | 3.9 | 5.64 | 6.96 | 8.32 | 11.67 | 11.67 | 11.67 | 200:50 | — |
| image_1024k | 50 | 100 | 5 | 26.3 | 26.33 | 30.2 | 39.89 | 65.72 | 88.85 | 88.85 | 88.85 | 200:50 | — |
| image_5120k | 50 | 100 | 5 | 100.1 | 100.1 | 117.44 | 131.53 | 146 | 158.27 | 158.27 | 158.27 | 200:50 | — |
| document_512k | 50 | 100 | 5 | 13.76 | 13.95 | 17.5 | 22.62 | 29.96 | 36.73 | 36.73 | 36.73 | 200:50 | — |
| image_1024k_c20 | 818 | 100 | 53.1 | 399 | 399.05 | 403.47 | 408.2 | 411.16 | 417.9 | 428.23 | 428.23 | 200:818 | — |

Resources (sampled every 250 ms during the run).

| Payload | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| payload_1k | 1 | 8 | 42.0 → 42.0 → 37.8 | 28 | 14 | 20 | 0 | 0 | 1/4 | 87.2 | 2 | 1 | 1 |
| payload_10k | 1 | 8 | 37.8 → 38.3 → 38.3 | 28 | 14 | 20 | 0 | 0 | 1/8 | 87.4 | 2 | 1 | 1 |
| payload_100k | 2 | 4 | 38.3 → 41.7 → 40.7 | 28 | 14 | 20 | 1 | 1 | 1/4 | 92.5 | 2 | 1 | 1 |
| payload_1024k | 11 | 28 | 40.7 → 53.2 → 49.6 | 28 | 14 | 21 | 4 | 1 | 5/12 | 145.5 | 2 | 1 | 1 |
| image_100k | 2 | 8 | 49.6 → 50.8 → 41.6 | 28 | 14 | 21 | 6 | 1 | 1/8 | 149.4 | 2 | 1 | 1 |
| image_1024k | 14 | 56 | 41.6 → 53.0 → 51.5 | 28 | 14 | 20 | 6 | 1 | 6/20 | 221.3 | 2 | 1 | 1 |
| image_5120k | 47 | 140 | 51.5 → 126.2 → 84.9 | 28 | 14 | 21 | 26 | 1 | 21/48 | 569.9 | 2 | 1 | 1 |
| document_512k | 8 | 24 | 84.9 → 101.6 → 46.1 | 28 | 14 | 21 | 9 | 1 | 3/8 | 585.2 | 2 | 1 | 1 |
| image_1024k_c20 | 101 | 484 | 46.1 → 167.2 → 58.7 | 28 | 52 | 96 | 120 | 2 | 54/196 | 1003.3 | 21 | 20 | 20 |

### P8 — Streaming shapes

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Stream shape | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| huge_c1 | 548 | 100 | 54.8 | 6.24 | 17.46 | 20.94 | 25.81 | 29.12 | 38.61 | 48.81 | 48.81 | 200:548 | — |
| huge_c10 | 559 | 100 | 54.9 | 187.86 | 199.01 | 203.64 | 208.26 | 211.99 | 219.18 | 227.06 | 227.06 | 200:559 | — |
| huge_c20 | 569 | 100 | 54.7 | 388.13 | 398.94 | 402.78 | 407.1 | 410.08 | 415.54 | 422.62 | 422.62 | 200:569 | — |
| chat_ts-unicode | 100 | 100 | 10 | 2.49 | 2.58 | 2.93 | 3.3 | 4.08 | 4.36 | 5.07 | 5.07 | 200:100 | — |
| chat_ts-keepalive | 100 | 100 | 10 | 2.05 | 2.13 | 2.61 | 3.03 | 3.35 | 4.31 | 4.79 | 4.79 | 200:100 | — |
| chat_ts-empty | 100 | 100 | 10 | 2.21 | 2.21 | 2.62 | 3.01 | 3.39 | 3.82 | 3.95 | 3.95 | 200:100 | — |
| chat_ts-nousage | 100 | 100 | 10 | 2.03 | 2.04 | 2.55 | 3.04 | 3.6 | 3.99 | 5.16 | 5.16 | 200:100 | — |

Resources (sampled every 250 ms during the run).

| Stream shape | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| huge_c1 | 83 | 272 | 58.7 → 41.6 → 41.6 | 28 | 15 | 23 | 1 | 1 | 47/152 | 997.9 | 2 | 2 | 1 |
| huge_c10 | 92 | 360 | 41.6 → 46.0 → 41.0 | 28 | 32 | 57 | 10 | 2 | 51/216 | 589.0 | 11 | 10 | 10 |
| huge_c20 | 79 | 252 | 41.1 → 49.0 → 42.5 | 28 | 52 | 97 | 20 | 1 | 46/120 | 589.5 | 21 | 20 | 20 |
| chat_ts-unicode | 2 | 8 | 42.5 → 42.6 → 39.5 | 28 | 15 | 22 | 0 | 0 | 2/8 | 589.5 | 2 | 2 | 1 |
| chat_ts-keepalive | 1 | 4 | 39.5 → 40.0 → 39.6 | 28 | 14 | 20 | 1 | 1 | 2/8 | 589.5 | 2 | 1 | 1 |
| chat_ts-empty | 1 | 4 | 39.6 → 40.1 → 39.9 | 28 | 14 | 21 | 0 | 0 | 2/8 | 589.5 | 2 | 1 | 1 |
| chat_ts-nousage | 1 | 8 | 39.9 → 40.0 → 39.7 | 28 | 14 | 20 | 1 | 1 | 2/4 | 589.5 | 2 | 1 | 1 |

### P9 — Upstream failure modes

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Failure mode | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| chat_ts-500 | 554 | 0 | 54.9 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 502:554 | upstream_unavailable:554 |
| chat_ts-429 | 554 | 0 | 54.9 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 502:554 | upstream_unavailable:554 |
| chat_ts-flaky | 554 | 99.64 | 54.9 | 99.78 | 99.82 | 100.47 | 101.12 | 101.52 | 102.12 | 103.05 | 103.05 | 200:552, 502:2 | upstream_unavailable:2 |
| chat_ts-reset | 554 | 100 | 54.9 | 97.64 | 99.79 | 101.04 | 102.45 | 103.31 | 106.68 | 126.45 | 126.45 | 200:554 | — |
| chat_ts-badjson | 554 | 100 | 54.9 | 99.85 | 99.85 | 100.49 | 101.01 | 101.34 | 101.88 | 102.3 | 102.3 | 200:554 | — |
| slow_c10 | 40 | 100 | 2 | 5003.69 | 5003.69 | 5005.35 | 5005.87 | 5006.2 | 5006.56 | 5006.56 | 5006.56 | 200:40 | — |
| slow_c50 | 230 | 43.48 | 9.2 | 6010.76 | 6010.89 | 6996.34 | 7000.69 | 7001.35 | 7005.18 | 7005.29 | 7005.29 | 200:100, 503:130 | provider_busy:130 |

Resources (sampled every 250 ms during the run).

| Failure mode | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| chat_ts-500 | 5 | 16 | 39.7 → 41.5 → 41.5 | 28 | 22 | 37 | 5 | 1 | 5/16 | 589.5 | 6 | 5 | 5 |
| chat_ts-429 | 5 | 16 | 41.5 → 42.1 → 40.7 | 28 | 22 | 37 | 5 | 1 | 5/12 | 589.5 | 6 | 5 | 5 |
| chat_ts-flaky | 7 | 20 | 40.8 → 41.3 → 39.7 | 28 | 22 | 37 | 5 | 1 | 7/16 | 589.5 | 6 | 5 | 5 |
| chat_ts-reset | 10 | 32 | 39.7 → 40.7 → 40.2 | 28 | 18 | 29 | 5 | 1 | 20/56 | 600.0 | 6 | 1 | 5 |
| chat_ts-badjson | 6 | 16 | 40.2 → 41.7 → 39.9 | 28 | 22 | 37 | 5 | 0 | 6/16 | 600.0 | 6 | 5 | 5 |
| slow_c10 | 0 | 4 | 39.9 → 42.5 → 42.1 | 28 | 32 | 58 | 10 | 10 | 0/4 | 600.0 | 11 | 10 | 10 |
| slow_c50 | 1 | 16 | 42.1 → 46.4 → 47.1 | 28 | 82 | 157 | 50 | 20 | 0/4 | 600.0 | 51 | 20 | 50 |

### P10 — Connection limits and connection reuse

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Connection test | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| healthz_c5000 | 809565 | 100 | 80506.8 | 1.06 | 1.06 | 98.17 | 185.85 | 206.95 | 306.17 | 450.92 | 915.79 | 200:809565 | — |
| models_c5000 | 432526 | 100 | 42398 | 15.83 | 15.89 | 54.11 | 516.04 | 610.75 | 905.49 | 1539.51 | 2685.66 | 200:432526 | — |
| mix_c5000 | 21005 | 17.03 | 1064.9 | 3937.41 | 3937.45 | 4179.26 | 4435.67 | 4500.66 | 4707.6 | 4820.85 | 4834.11 | 200:3578, 503:17427 | server_busy:17427 |
| newconn_claude_rps40 | 600 | 100 | 40 | 2.39 | 2.52 | 3.13 | 3.6 | 3.83 | 4.18 | 5.38 | 5.38 | 200:600 | — |
| newconn_models_c50 | 287371 | 100 | 28735.5 | 1.39 | 1.4 | 2.19 | 3.18 | 3.89 | 5.69 | 8.61 | 23.07 | 200:287371 | — |

Resources (sampled every 250 ms during the run).

| Connection test | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| healthz_c5000 | 435 | 2368 | 47.1 → 257.5 → 249.8 | 28 | 7394 | 7426 | 0 | 0 | 0/4 | 600.0 | 7363 | 20 | 5000 |
| models_c5000 | 1113 | 5912 | 249.8 → 398.5 → 361.8 | 28 | 5054 | 7753 | 1 | 1 | 1/4 | 600.0 | 5042 | 1 | 5000 |
| mix_c5000 | 23 | 216 | 361.8 → 322.9 → 322.9 | 28 | 5038 | 6035 | 200 | 3 | 15/40 | 600.0 | 5001 | 26 | 5000 |
| newconn_claude_rps40 | 5 | 16 | 322.9 → 322.9 → 154.2 | 28 | 19 | 32 | 1 | 1 | 5/16 | 600.0 | 2 | 7 | 1 |
| newconn_models_c50 | 708 | 2140 | 154.2 → 72.7 → 64.3 | 28 | 65 | 89 | 0 | 0 | 0/4 | 600.0 | 48 | 1 | 50 |

### P11 — Over-capacity soak (200 rps mix, 120 s)

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Soak | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| soak_mix_rps200_120s | 24000 | 84.1 | 187.8 | 4000.5 | 4000.6 | 5519.06 | 7737.72 | 7851.58 | 7942.65 | 7991.56 | 7999.44 | 200:20185, 503:3815 | server_busy:3814, provider_busy:1 |

Resources (sampled every 250 ms during the run).

| Soak | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| soak_mix_rps200_120s | 15 | 56 | 64.3 → 108.9 → 93.8 | 28 | 1012 | 1379 | 200 | 18 | 16/56 | 600.0 | 995 | 36 | 994 |

### P12 — Sustainable soak (140 rps mix, 180 s)

Latency in ms (successful requests only; see §9 for the latency of rejected requests).

| Soak | Requests | OK % | Throughput rps | TTFB p50 | p50 | p75 | p90 | p95 | p99 | p99.9 | max | Statuses | Errors |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| soak_mix_rps140_180s | 25200 | 100 | 140 | 1.6 | 1.65 | 2.36 | 2.91 | 3.21 | 3.75 | 4.49 | 6.79 | 200:25200 | — |

Resources (sampled every 250 ms during the run).

| Soak | Mesh CPU avg % | Mesh CPU peak % | Mesh RSS start→peak→end MB | Mesh threads pk | Mesh FDs pk | Goroutines pk | Queue in-flight pk | Provider in-flight pk (sum) | testserve CPU avg/peak % | testserve RSS pk MB | Conns :8090 pk | Conns :9000 pk | Client in-flight pk |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| soak_mix_rps140_180s | 12 | 32 | 93.8 → 94.0 → 54.9 | 28 | 14 | 22 | 25 | 1 | 16/40 | 600.0 | 2 | 1 | 1 |
