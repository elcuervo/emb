## Why

Every eager embed call costs a full client→server round trip plus per-command parse CPU
on the server. Under load, request handling (parsing, dispatch, goroutine churn) competes
with ONNX inference for CPU, so end-to-end throughput degrades and p50/p99 become
unstable. Lazy batching already collapses N calls into one `EMB.MULTI`, but nothing
measures the round-trip/overhead budget end-to-end, the connection pool defaults are
untuned, and there is no documented path to horizontal scale. We want: as few round trips
as possible, stable inference throughput even while the request path is CPU-busy, and a
clear scaling story — with the Go implementation treated as given (config-level tuning
only).

Validation has been unreliable so far: the server, harness threads, and `redis-benchmark`
all share the machine's 10 CPUs (4 performance + 6 efficiency on the M1 Pro), so the
stability gate flaked on a busy machine and before/after numbers mix harness-side noise
with the real improvement. Partitioning the CPUs — an app partition for the server, a
benchmark partition for the tooling — makes the validation of the round-trip reduction a
clean, repeatable measurement.

## What Changes

- Add a Ruby client benchmark harness (`gems/emb/bench/`) plus a justfile target,
  measuring end-to-end scenarios: sequential eager, lazy batched, eager-pipelined, and
  threaded concurrency. Metrics: req/s, p50/p99, and an overhead ratio against the raw
  inference budget.
- Add a stability scenario to the harness: synthetic parse-heavy load (many/expensive
  commands) while measuring inference p50/p99 — the success criterion is that inference
  throughput stays stable under request-path CPU pressure.
- Partition the reference machine's CPUs for measurement: the server runs in an app
  partition (`GOMAXPROCS` + `intra_op_threads` budget; `taskset -c` on Linux) and the
  benchmark tooling — harness, parse-load generator, `redis-benchmark` — runs in a
  disjoint benchmark partition (process budget on macOS, which has no affinity tooling).
  Every result set in `BENCHMARK.md` publishes the partition layout it was measured under.
- Validate the improvement under the partition: re-run eager vs lazy-batched vs pipelined
  and the tuning config before/after, and report partitioned numbers alongside the
  earlier unpartitioned ones, so the round-trip reduction is proven on a non-contending
  baseline.
- Tune the connection pool on evidence: measure pool sizes at concurrency, then set the
  gem's default (and document the knobs) only if numbers justify a change.
- Evaluate the `hiredis` driver vs the pure-Ruby parser (throughput/latency, deployment
  cost of the native dep); document the tradeoff, adopt a default only if clearly better.
- Evaluate an eager pipelining convenience (`client.pipelined {}` or a documented
  pattern) for bursts of eager calls; ship it only if the harness shows a real win over
  plain eager calls.
- Add server tuning guidance (config only, **no Go code**): example config enabling the
  batcher window (`batching: {timeout, max_batch}`) and `intra_op_threads` below core
  count, so inference is isolated from parse/dispatch CPU.
- Document horizontal scaling: stateless multi-instance, model sharding, client-side
  routing (multiple clients / LB), per-instance cache behavior.
- Update `BENCHMARK.md` with client-side results and the stability numbers.

## Capabilities

### New Capabilities

- `ruby-client-benchmarks`: the client benchmark harness, metric definitions (req/s,
  p50/p99, overhead ratio), CPU-partitioned measurement (app partition vs benchmark
  partition), and the stability gating contract (inference p50/p99 under synthetic
  request-path load).

### Modified Capabilities

- `emb-ruby-client`: connection pooling requirement — pool default and driver behavior
  change based on benchmark evidence; pipelining convenience if adopted.

## Impact

- **Gem**: `gems/emb` — `bench/` harness, `client.rb` pool default (evidence-based),
  optional pipelining API, README pooling/driver docs.
- **Server**: none (code); example configs + docs demonstrating batcher window and CPU
  partitioning.
- **Tooling**: `justfile` — partitioned run target (server started in the app partition,
  harness in the benchmark partition), partition sizes as env vars with a documented
  default for the reference machine.
- **Docs**: `BENCHMARK.md` (client-side + stability + partition layout per result set),
  README pooling/tuning, horizontal scaling guide.
- **Tests**: benchmark scenarios double as regression targets where possible.