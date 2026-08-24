## Why

The client side is done: lazy batching collapses N calls into one `EMB.MULTI` and the
stability gate is reproducible under a CPU partition (committed). The remaining
bottleneck is **server-side** — and it shows under constant load from a client.

Measured (roundtrip-reduction design, D6): with the default config the server runs
`workers = cores` sessions against `intra_op_threads = all cores`, so request parsing and
ONNX thrash the same cores; the batching config (`batching: {timeout, max_batch}` +
`intra_op_threads < cores`) wins throughput (baseline 4.1–4.5ms vs 8.6–12.9ms, threaded
p99 48–54 vs 66–84ms) but is **worse** under a request storm (storm 2.57–2.67 vs default
1.87–1.93) because `EMB.MULTI` spawns one goroutine per pair — `N` pairs → `N`
unbounded goroutines racing for inference cores. Config alone cannot fix that fan-out;
it is a Go property. These wins are also all opt-in, so deployments get none of them by
default.

We want the server to stay stable and throughput-bound under constant load and request
storms: bound the `EMB.MULTI` fan-out, reserve parse/dispatch CPU by default, and turn
the measured stability scenarios into regression gates — so the client improvements
actually pay off in production.

## What Changes

- **Bound `EMB.MULTI` fan-out (Go, `internal/server`):** replace the unbounded
  goroutine-per-pair with a bounded concurrency pool / semaphore sized to the model's
  worker capacity, so a request storm can't spawn unbounded goroutines competing for
  inference cores. Per-pair `nil` (MGET) semantics and result ordering are preserved.
- **Default thread isolation (Go, model config):** set `intra_op_threads` to `cores−2`
  when unset (reserving cores for request parsing/dispatch) instead of defaulting to all
  cores — a busy request path can't starve ONNX, and vice versa. Explicit config still
  overrides.
- **Compose `EMB.MULTI` with the batcher window when enabled:** when a model sets
  `batching: {timeout, max_batch}`, MULTI pairs for that model flow through the existing
  window so the throughput win and MULTI combine (measured win preserved, storm now
  bounded).
- **Ship the stability scenarios as regression gates (Go test + harness):** the
  constant-load and storm stability ratios must pass a published threshold on the
  reference machine — this was the measured fail mode config could not fix. Numbers
  published in `BENCHMARK.md`.

## Capabilities

### New Capabilities

- `inference-cpu-isolation`: default `intra_op_threads` reservation (`cores−2`) so
  inference and request parsing/dispatch each keep a CPU budget; documented override.

### Modified Capabilities

- `emb-multi`: `EMB.MULTI` fan-out is bounded — the server MUST cap concurrent
  pair-processing to the model's worker capacity so request storms cannot spawn
  unbounded goroutines, while preserving per-pair `nil` (MGET) semantics, result
  ordering, and correct embeddings.

## Impact

- **Server (Go):** `internal/server/server.go` (`handleEMBMULTI` bounded fan-out),
  `internal/onnx` session options (default `intra_op_threads`), model registry defaults,
  Go tests.
- **Client/tooling:** re-run `bench-ruby` stability scenarios; add a storm mode to the
  gate (already present in the harness as `EMB_BENCH_LOAD_*`).
- **Docs:** `BENCHMARK.md` stability (constant + storm) numbers; server README
  thread-isolation and batcher defaults.
- **Tests:** server unit tests for bounded fan-out and `intra_op_threads` default; a Go
  or harness regression gate for storm stability.
