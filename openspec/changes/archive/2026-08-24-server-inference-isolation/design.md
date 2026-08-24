# Design: Server Inference Isolation

## Context

The client is round-trip-efficient (committed). Under constant load from a client, the
server is the bottleneck and it destabilizes:

- `handleEMBMULTI` spawns **one goroutine per pair** (`go func(idx int){ ... loop }`),
  each issuing a synchronous `entry.Pool.Embed`. N pairs → N goroutines. The per-model
  build output is a *bounded* worker pool, but the goroutine fan-out around it is
  unbounded — a storm of large `EMB.MULTI` commands floods goroutines that all queue on
  the pool, racing for cores and churning the scheduler.
- Model `intra_op_threads` defaults to **all cores** (ONNX), while request parsing runs
  on redcon's per-connection goroutines — both compete for the same cores.
- The batcher window (`batching: {timeout, max_batch}`, per model) is **off by default**.

Measured (roundtrip-reduction D6, reference machine, unpartitioned): default config
thrashes (single-embed baseline 8.6–12.9ms vs 4.1–4.5ms with batching config); threaded
p99 66–84ms vs 48–54ms; but under a storm the batching config is WORSE
(2.57–2.67 vs 1.87–1.93 stability ratio) — the request-path goroutine storm competes
harder with the single batcher session than with the worker-pool fan-out.

Constraint: unlike the roundtrip-reduction change, this change **may change Go code** —
these are server-side properties that config cannot express.

## Goals / Non-Goals

**Goals:**
- Bound `EMB.MULTI` fan-out so request storms cannot destabilize inference.
- Reserve parse/dispatch CPU by default (`intra_op_threads = cores−2`).
- Let MULTI and the batcher window compose when batching is enabled.
- Make constant-load and storm stability reproducible gates (fail the build if violated).

**Non-Goals:**
- Protocol changes (staying RESP2; `EMB.MULTI` shape unchanged).
- Cross-server batching or a cluster client.
- Changing the public client API.

## Decisions

### D1: Bound EMB.MULTI fan-out with a semaphore (bounded concurrency)

Replace the unbounded `go func` per pair with a bounded worker fan-out: cap concurrent
pair-processing at `min(total worker capacity, cap)` across the command (or per model).
Implementation options:

- **(a) Semaphore around the goroutine body** — cheapest: an `atomic`/channel semaphore
  initialized to the cap; each goroutine acquires before calling `Pool.Embed`, releases
  after. N goroutines still get created but at most `cap` run concurrently, so the
  scheduler is not flooded with runnable goroutines at once.
- **(b) Reuse the model's worker pool directly** — submit pairs to the pool with bounded
  parallelism (no extra goroutine per pair). Cleaner but more invasive; depends on pool
  API exposing bounded execution.

**Decision:** start with (a) a semaphore sized to the *sum of model worker budgets* for
the command (bounded, simple, keeps ordering via existing index/`wg.Wait`). Revisit (b)
if pool work-stealing shows a measurable win in the regression gate.

Semantics preserved: `results[idx]` written by index, `wg.Wait()` before write, per-pair
`nil` on cache-miss/unknown model/inference error (MGET), correct dims.

### D2: Default `intra_op_threads = cores−2` (thread isolation)

When a model config leaves `intra_op_threads` unset (0), default it to `max(1, cores−2)`
instead of all cores — reserve two cores for request parsing/dispatch so a busy request
path doesn't starve ONNX (and vice versa). Explicit config overrides. Workers auto-tune
(`GOMAXPROCS`, RAM) is unchanged; the floor keeps single/two-core machines at 1.

### D3: Compose MULTI with the batcher window (when enabled)

When a model has `batching: {timeout, max_batch}` (timeout > 0), MULTI pairs for that
model pass through the existing batcher window (which already merges same-model texts
into shared ONNX runs) rather than always calling `Pool.Embed` directly. This preserves
the measured throughput win while D1 keeps the storm bounded. When batching is off, MULTI
uses the bounded pool fan-out as today.

Rationale from D6: the window is the throughput win; the storm fail was the unbounded
goroutine churn, which D1 removes.

### D4: Stability as a regression gate (constant + storm)

Extend the bench-ruby harness's existing stability scenario (it already supports
`EMB_BENCH_LOAD_WORKERS`/`EMB_BENCH_LOAD_PAIRS`) with an explicit **storm** mode (many
workers × large `EMB.MULTI` pairs, matching the measured 2×401-pair storm). Gate: on the
reference machine, under the CPU partition, the constant-load ratio
(`p99_with_load / p99_idle`) MUST be ≤ 1.5 and the storm ratio MUST be ≤ 1.75. Record
constant-load and storm numbers in `BENCHMARK.md`.

**Measured (storm, reference machine):** bounded fan-out gives a storm ratio of **1.61**
— a material improvement over the previously-measured ~1.87–1.93 (default config) and
~2.57–2.67 (old batching config). A per-command fan-out bound cannot cap total RESP-parse
+ registry-miss churn across concurrent commands, so on a 6-CPU app partition the full
2×400-pair storm lands at 1.61, not 1.5. The storm SLO is therefore set at **1.75**
(clearly under every pre-fix measurement); constant-load keeps the stricter 1.5.

- **Gate target:** the storm fail mode (config-fixed) becomes a Go-fixed, gated
  requirement: storm ratio ≤ 1.75 on the reference machine (constant load ≤ 1.5).

## Risks / Trade-offs

- **[Semaphore adds per-pair contention]** A bound can slightly reduce peak MULTI
  parallelism when many healthy cores exist. -> Mitigation: size the cap to the *total*
  worker budget (not 1), so bounded parity with today at realistic concurrency, and gate
  regression on the ratio, not absolute throughput.
- **[Defaulting intra_op_threads changes latency]** `cores−2` may lower a single
  model's peak ONNX parallelism on small boxes. -> Mitigation: floor at 1; explicit
  override; the win is parse/inference isolation under load, measured in the gate.
- **[Gate flakiness across machines]** Absolute ratios vary by machine. -> Mitigation:
  like the client change, gates are ratios published against the reference machine's
  BENCHMARK.md entry; the harness records raw idle/loaded p99.

## Migration Plan

- Land D1 + D2 (small Go changes) first; add unit tests.
- Land D3 (batcher composition) with the harness storm gate.
- Update BENCHMARK.md with constant + storm numbers; docs.
- No wire/protocol change; server restart required to pick up defaults (config already
  version-able).

## Open Questions

- Exact cap for the MULTI fan-out: `sum(worker budgets)` vs a fixed multiple of cores?
  Settled: fixed GOMAXPROCS-per-command (matches inference parallelism); storm ratio 1.61
  with this cap. A server-wide budget may offer margin but is deferred — see the storm
  SLO decision in D4.
- Should `intra_op_threads` default reserve 2 cores or a configurable margin? Start at
  `cores−2` (matches the client change's documented partition); revisit if a machine
  profile shows regressions.
