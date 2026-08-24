# Design: Roundtrip Reduction

## Context

The stack: `emb` server (Go, RESP via redcon) + Ruby client (`gems/emb`, ConnectionPool
wrapping RedisClient). Current round-trip behavior:

```
eager:    Emb[:m]["a"] ──▶ EMB ─┐  N commands → N round trips → N parses
          Emb[:m]["b"] ──▶ EMB ─┼─ and N concurrent ONNX runs (worker pool = cores)
          Emb[:m]["c"] ──▶ EMB ─┘
lazy:     l1,l2,l3 ─▶ one EMB.MULTI ─▶ 1 round trip, server fans out a goroutine per pair

Server facts (verified):
- batcher window off by default (timeoutMS > 0 enables; workers auto = cores)
- EMB.MULTI spawns a goroutine per pair; each pair = tokenizer + ONNX Run
- request parsing runs on redcon's per-connection goroutines — shared CPU with inference
- ONNX intra_op_threads default = all cores → parsing can starve/be starved
```

Constraint from the proposal: **the Go implementation is taken as given** — server levers
are config + docs only. The client owns round trips; the deployer owns topology.

Machine facts (reference, M1 Pro): 10 CPUs = 4 performance + 6 efficiency. macOS exposes
no hardware-affinity tooling (`taskset` is Linux-only), so the CPU-budget mechanism on
macOS is `GOMAXPROCS` (Go scheduler) + ONNX `intra_op_threads` plus a process-level load
budget for the benchmark client; Linux gets true pinning via `taskset -c`.

## Goals / Non-Goals

**Goals:**
- Minimize round trips per unit of work: prove lazy batching; close the eager-burst gap.
- Stable inference throughput while the request path is CPU-busy (stability gate).
- Give the operator config-level tools (batcher window, thread partitioning) and a
  horizontal scale path.
- Numbers: end-to-end overhead ratio reported; pool/driver decisions evidence-based.

**Non-Goals:**
- Rewriting server internals (no Go changes).
- A cluster/load-balancing client this iteration (documented pattern only).
- Protocol changes (stay RESP2; vectors are binary blobs already, text args are small).
- Cross-thread batching or a flush API (already rejected in the batching change).

## Decisions

### D1: Measurement first — a client benchmark harness gates every other decision

`gems/emb/bench/` (plain Ruby script, runnable via `just bench-ruby` and `rake bench`):
scenarios (sequential eager, lazy batched, eager-pipelined, N-thread concurrency) against
a local server; reports req/s, p50/p99, and the overhead ratio:
`(e2e_time − inference_time) / inference_time` using the single-embed caches-warm
baseline as `inference_time`. A stability scenario runs a parse-heavy synthetic load
(high-arg commands) in the background while measuring inference p50/p99.

- **Alternative:** pyperf/microbenchmarks in isolation. Rejected — the question is the
  end-to-end contract, and isolated numbers don't tell us what the user experiences.
- **Gate:** pool default, driver, and pipelining decisions below are conditional on
  harness results; without numbers the change ships the harness + docs only.

### D2: Lazy batching is the primary round-trip reduction (shipped; now proven)

The batch-loader work already collapses N calls to one `EMB.MULTI`. The harness turns
"one round trip per request" from a claim into a measured number, per scenario and
request size.

### D3: Eager pipelining — convenience only if numbers justify

`RedisClient` supports `pipelined`; a burst of eager calls (issued without the lazy API,
e.g. cross-call-site feelers) can coalesce into one packet. Design: evaluate three
options in the harness: (a) plain eager per call, (b) `client.pipelined {}` documented
pattern, (c) a first-class `client.pipelined {}` convenience on `Emb::Client`.

**Measured (M4, 2026-08):** pipelined is the fastest client path (p50 ~7.4–7.9ms vs eager
~8.3–8.6ms, req/s 130–135 vs 115–118, ~15–20% win), and the raw
`pool.with { conn.pipelined { ... } }` pattern already expresses it with no new API.
**Decision:** document the pattern (b); do not ship a convenience method — the win
overlaps the lazy API and the pattern is four lines of documented code.

### D4: Driver — hiredis evaluated, pure-Ruby stays default unless clearly better

hiredis = C RESP parser (lower parse CPU, higher throughput; native dep in deployment).
Design: benchmark both on the same harness; adopt hiredis as the gem default only if
req/s improves ≥ threshold (e.g. 15%) at the target concurrency with no pathological
p99. Otherwise document `driver: :hiredis` as an on-demand tuning knob.

**Measured (M4, 2026-08):** hiredis wins only the all-round-trip eager path (+12%
req/s) and is ~neutral for batched/pipelined/threaded (server-bound); below the 15%
gate. Building it also required OpenSSL headers (native-build friction).
**Decision:** keep pure-Ruby default; document hiredis + the required
`require "hiredis-client"` as a knob.

### D5: Pool default — tuned, then set on evidence

Measure pool sizes {1, 2, 4, 8, 16} against concurrent MB-sized batches; pick the size
where p50 stops improving (knee) and set it as the gem default (current: 5). Document
`pool:` and the ConnectionPool checkout `timeout:` in the README with the harness numbers.

**Measured (M4, 2026-08):** pool sweep {1,2,4,8,16} — single-connection regimes move
≤10% (eager 104→116 req/s, lazy 110→120); threaded moves ~25% but p99 stays poor
(144–185ms) across all sizes (server-side 10-session thread thrash, not client pool).
**Decision:** keep the default of 5; document pool as a per-workload knob with the sweep
numbers. Small pools are fine for inference-bound workloads.

### D6: Server isolation via config (no Go code) — batcher window + thread partition

Two config-only levers, demonstrated in an example config:
- `batching: {timeout: 1, max_batch: 64}` per model: concurrent pairs coalesce into
  shared ONNX runs (workers=1 with a window) → fewer context switches, better cache
  reuse, and inference becomes *scheduled work with a window* rather than N concurrent
  races for cores — the direct answer to "request CPU must not clog inference".
- `intra_op_threads: <cores−2>`: reserve cores for parsing/dispatch so a busy request
  path doesn't starve ONNX, and vice versa.

**Measured (M4, 2026-08, interleaved):**
- Throughput: batching config wins decisively — single-embed baseline 4.1–4.5ms vs
  8.6–12.9ms (the default's 10 sessions × 10 ONNX threads thrash), threaded p99 48–54ms
  vs 66–84ms, pipelined ~147 vs ~130 req/s.
- Stability under parse load: batching config is WORSE every comparison — realistic
  load (1×100 pairs) 1.32–2.03 vs default 1.16–1.58; storm load (2×401 pairs) 2.57–2.67
  vs default 1.87–1.93. The request-path goroutine storm (goroutine per MULTI pair,
  registry-miss churn) competes harder with the single batcher session than with the
  worker-pool fan-out.
- The stability gate (p99 ratio ≤ 1.5) flaked on both configs in a noisy machine
  (p99 over 100 rounds ≈ max), passing reliably only at realistic load on the default
  config (1.16).

**Decision:** ship the batching config as a *throughput* example (it is verified: faster
and calmer tails for bursty work) with the stability caveat documented; the gate's
storm-fail mode is a server-side goroutine-fan-out property and is out of scope (no Go
changes) — recorded as a follow-up in the README's horizontal-scaling/limitations text.

### D7: Horizontal scaling — documented pattern, stateless instances

emb instances are stateless (model in memory; LRU cache per instance). Scale-out paths,
documented in README/BENCHMARK: (1) model sharding — each instance serves a subset of
models, client routes by model; (2) text-keyed sharding behind an L4/LB for
same-model scale; (3) per-instance cache means warm-up per box (optionally `-cache`).
Lazy batching stays per-server (a thread's loaders target one client) — batching and
horizontal scale compose: each server receives one MULTI per request.

- **Non-goal:** a client that splits one MULTI across servers — pairs are already
  parallel inside one instance; splitting adds fan-out complexity for a bounded gain.

### D8: Partition the machine — app CPUs vs benchmark CPUs

Measurement today is noisy: the server, harness threads, and `redis-benchmark` share all
10 CPUs, so the stability gate flaked on a busy machine and before/after numbers mix
harness-side noise with the real improvement. Fix: run every benchmark under a fixed CPU
partition, published with each result set.

- **App partition (server):** `GOMAXPROCS=6` + `intra_op_threads: 4` (reserve 2 cores
  for parse/dispatch) — the server's entire budget lives here, matching the D6
  `cores−2` guidance.
- **Benchmark partition (tooling):** the remaining 4 CPUs for the harness, its thread
  scenarios, the parse-load generator, and `redis-benchmark` clients.
- **Mechanism:** Linux `taskset -c <list>` on both sides; macOS falls back to process
  budgets (`GOMAXPROCS`, `intra_op_threads`, `nice`) — no affinity syscalls exposed.
- **What it validates:** the round-trip improvement (eager → lazy batching, pipelining,
  batching config) is measured with the server alone on its partition, so harness-side
  steals are gone. The stability scenario still exercises request-path parse inside the
  server partition — clients send the load from the benchmark partition, but parsing
  consumes the *server's* budget — so the gate keeps its meaning: inference stable while
  the request path is busy within the server's own budget.
- **Not a tuning lever:** the partition is methodology, not a production recommendation
  — production shares cores between clients and servers as the deployer decides.

## Risks / Trade-offs

- **[Numbers don't justify changes]** If the harness shows the client is already
  inference-bound, pool/driver/pipelining changes may ship as docs-only.
  → Mitigation: D1 makes every decision conditional; the change's value = measured
  baseline + stability gate + guidance.
- **[Batcher window adds latency]** A windowed batch adds up to `timeout` ms latency for
  solo requests in exchange for throughput stability.
  → Mitigation: document per-model choice; `timeout: 1` is sub-ms on localhost; stability
  scenario includes the solo-request case.
- **[hiredis native dep]** Deployment friction.
  → Mitigation: keep pure-Ruby default; measure before adopting; document the knob.
- **[Stability gate flakiness]** Synthetic load must be reproducible across machines.
  → Mitigation: gates expressed as ratios (p99_with_load / p99_idle) not absolutes; the
  harness records raw numbers for the machine's BENCHMARK.md entry. Run under a fixed CPU
  partition (D8) so the harness and load generator never compete with the server for
  cores — this is the measured cause of the flake.
- **[Scope creep toward cluster client]** Easy to keep going: LB, retries, re-sharding.
  → Mitigation: explicitly non-goal; the deliverable is the documented pattern.

## Migration Plan

- Harness + docs land first (all scenarios runnable, stable on the dev machine).
- Then evidence-based decisions land as small commits (pool default, driver, pipelining),
  each gated by the harness; each individually revertible.
- Server example config + README guidance ship with the docs; no server behavior change
  unless the operator opts in via config, so rollback = revert config.

## Open Questions

- Acceptable overhead-ratio threshold and stability ratio for this machine (target:
  p99_with_load / p99_idle ≤ 1.5× at the documented concurrency)? Settle from the first
  harness run rather than pre-committing.
  **Settled (measured):** overhead ratios published from runs (eager ~±10%, lazy
  −1…−17%, pipelined −13…−30%); stability threshold 1.5 with realistic load
  (1 worker × 100 pairs) — passes on default config (1.16), documented alongside the
  storm-load fail (1.9×) as a server-side limitation.
- Is `client.pipelined {}` worth shipping, or is the documented pattern enough? Resolved
  by D3's gate after measuring eager bursts.
  **Settled:** documented pattern only — measured best client path but expressible
  without new API.
- Should the default pool size become machine-relative (cores × k) or stay absolute?
  **Settled:** stays absolute 5 — pool is not the bottleneck (see D5).
- **Follow-up (server, needs Go):** bound the goroutine-per-pair fan-out in `handleEMBMULTI`
  (or route MULTI pairs through the batcher) so request-path storms cannot destabilize
  inference — the measured fail mode that config cannot fix.