# Tasks: Roundtrip Reduction

## 1. Benchmark harness

- [x] 1.1 Create `gems/emb/bench/` with a plain-Ruby harness (no test-framework dep): scenarios for sequential eager, lazy batched (`Emb.batch`), eager-pipelined bursts, and N-thread concurrency; report req/s, p50, p99 per scenario
- [x] 1.2 Wire `just bench-ruby` (start server, run harness, stop server) and a `rake bench` task; assert non-zero exit + measurable numbers per scenario
- [x] 1.3 Add an overhead-ratio metric: `(e2e − inference) / inference` using a warm single-embed baseline as the inference time
- [x] 1.4 Emit round-trip evidence: assert the lazy scenario sends exactly one `EMB.MULTI` for N pairs vs N eager `EMB` commands

## 2. Baseline & stability

- [x] 2.1 Run the harness on the reference machine (same server config family as `BENCHMARK.md`); record eager/lazy/pipelined numbers and overhead ratios
- [x] 2.2 Add the stability scenario: synthetic parse-heavy load (many/large-arg commands) while measuring inference p50/p99; compute `p99_with_load / p99_idle`
- [x] 2.3 Decide and publish the stability threshold in `BENCHMARK.md` from the first run; the gate fails when the ratio exceeds it

  Threshold set: 1.5. First runs on the default server config FAIL (~1.85–2.06×) —
  recorded as the motivation for the server tuning guidance (section 4).
- [x] 2.4 Implement CPU-partitioned runs in the harness/justfile: start the server in
  its app partition (`GOMAXPROCS` + `intra_op_threads` budget; `taskset -c` on Linux)
  and run the harness / parse-load generator / `redis-benchmark` in the disjoint
  benchmark partition; expose both partition sizes as env vars with a documented
  default for the reference machine (10 CPUs → 6 app / 4 benchmark)

  Implemented: `app_cpus`/`bench_cpus` justfile vars (default 6/4), `bench-ruby`
  (default config `bench-cpu-partition.yaml`, GOMAXPROCS=app_cpus + taskset -c on
  Linux for server and tooling), `bench-redis-single`/`multi` partition wiring, and
  `EMB_BENCH_APP_CPUS`/`EMB_BENCH_BENCH_CPUS` env vars the harness reports in its
  header line.
- [x] 2.5 Re-run baseline, scenario, and stability measurements under the partition and
  validate the improvement: report eager vs lazy-batched vs pipelined req/s, p50/p99, and
  overhead ratios with the server alone on its app partition; confirm the lazy scenario's
  one-`EMB.MULTI` claim and the tuning config's throughput win are reproducible on a
  non-contending baseline, and record partitioned-vs-unpartitioned differences

  Partitioned (6 app / 4 bench, `bench-cpu-partition.yaml`, GOMAXPROCS=6,
  intra_op_threads=4): eager 156 req/s (p50 5.8, p99 16.7ms), lazy 153 req/s
  (p50 6.6, p99 **6.6ms**), pipelined 157 req/s, threaded 167 req/s (p50 12.4,
  p99 116.5). Stability ratio **1.14 PASS** (idle p99 359 → loaded 411). Lazy sends
  exactly one `EMB.MULTI`; its p99 ≈ p50 (no tail), vs eager p99 16.7ms.
  Unpartitioned (10 app / 0 bench, default config): eager 255, lazy 251, pipelined 283,
  threaded 400 req/s; stability 1.21 PASS. Server is capped to its 6-CPU app budget
  under the partition, so raw req/s drops vs 10-CPU — the partition is measurement
  methodology (a deployment-shaped budget), not a throughput knob; its win is
  reproducible tails + a stable gate.

## 3. Evidence-based client decisions

- [x] 3.1 Pool-size sweep {1, 2, 4, 8, 16} at the benchmarked concurrency; pick the knee; update the gem default in `client.rb` only if it differs from 5

  M4 sweep: single-connection regimes move ≤10% (104→116 req/s eager); threaded ~25%
  but p99 poor across all sizes (server-side thrash). Default stays 5 — documented as a
  per-workload knob with the sweep numbers.
- [x] 3.2 Driver comparison (pure-Ruby vs `driver: :hiredis`) on the same harness; adopt hiredis as default only if it meets the improvement threshold, otherwise document it as a knob

  M4: hiredis +12% only on the eager all-round-trip path; neutral on batched/threaded.
  Below the 15% gate — pure-Ruby stays default; document the knob + `require
  "hiredis-client"`.
- [x] 3.3 Eager-pipelining experiment: measure plain eager bursts vs `RedisClient#pipelined`; ship `client.pipelined {}` only if the harness shows a meaningful p50 win, otherwise document the pattern

  M4: pipelined is the best client path (p50 7.4–7.9 vs eager 8.3–8.6ms) but the raw
  `pool.with { pipelined }` pattern already expresses it — document, no new API.

## 4. Server tuning guidance (config only)

- [x] 4.1 Add an example server config (or doc snippet) enabling `batching: {timeout, max_batch}` and `intra_op_threads` below core count for a model
- [x] 4.2 Validate the config improves the stability ratio in the harness (inference stable under parse load); record both before/after numbers

  Interleaved runs: batching config wins throughput (baseline 4.4 vs 8.9ms, threaded p99
  51 vs 84ms) but is WORSE under parse load (realistic 1.32–2.03 vs 1.16–1.58; storm
  2.57–2.67 vs 1.87–1.93). Stability limit is the server goroutine-per-pair fan-out —
  config cannot fix it (no Go changes); recorded as a follow-up.

## 5. Docs & validation

- [x] 5.1 Update `BENCHMARK.md`: client-side results (scenarios, overhead ratios,
  stability ratio, pool sweep, driver comparison) and the CPU partition layout
  (app vs benchmark) under which every number was measured, following the existing format

  Added the partition layout to the Hardware line, a Client-side (Ruby) section with
  partitioned (6/4) and unpartitioned (10/0) runs, pool/driver/pipelining evidence, and
  `just bench-ruby` in Reproduce.
- [x] 5.2 Update `gems/emb/README.md`: pooling knobs, driver selection, pipelining guidance, and the horizontal-scaling pattern (stateless multi-instance, model sharding, per-instance cache)

  Added default-pool note (5), a Performance section (eager-burst pipelining pattern via
  `Client#pool`, `driver: :hiredis` on demand, horizontal scaling).
- [x] 5.3 Run the gem's rubocop/rspec equivalents and confirm `openspec validate roundtrip-reduction --type change` passes

  rubocop: 14 files, no offenses. rspec: 52 examples, 0 failures (server on
  bench-cpu-partition.yaml). `ruby -c` clean. `openspec validate --type change` passes.