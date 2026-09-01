# fargate benchmark harness

Reproducible Fargate-shaped benchmark harness for `emb` on ARM64/Graviton CPU.

## Purpose

All project benchmarks in `BENCHMARK.md` were measured on Apple M1 Pro (macOS).
Production targets `linux/arm64` Fargate tasks. This harness replays the server
inside a `linux/arm64` Docker container bounded by `--cpus`/`--memory` matching the
Fargate vCPU tiers (1/2/4/8), drives it with `redis-benchmark` and a pure-Ruby
mixed-length RESP driver, and emits a versioned golden baseline so every
performance proposal can be validated pre/post.

## Hosts and "gold reference"

- **Gold reference**: an ARM64 Linux host (real Graviton instance or an ARM64 CI runner).
- **Approximation**: Apple Silicon (darwin/arm64) runs the `linux/arm64` image natively
  (no emulation) — good for iteration, not for absolute numbers.
- The harness prints a warning when the host is not the gold reference and tags
  emitted results with `"host": {"gold": false}`.

## CPU quota emulation

Every tier container runs with `--cpus <tier>` **and** `GOMAXPROCS=<tier>`
(container env). The env pin matters on hosts where the container CPU quota is
invisible to Go (Docker Desktop / OrbStack on macOS): there `runtime.GOMAXPROCS`
otherwise reports the host core count and the measured tiers would not match the
gold host, where Go honors the task cgroup quota. The env pin makes tier behavior
deterministic on every host.

Note that pinning GOMAXPROCS to the tier does **not** make throughput monotonic
in the tier: `intra_op_threads` defaults to `cores−2` (6 threads at tier 8), and
for small models (dim ≤ 768) more than ~2 intra-op threads loses to thread sync —
measured: tier8 c1 ≈ 140 req/s (6 threads) vs ≈ 550 at tier 2 (1 thread), and an
isolated 1-thread tier-8 run hits ≈ 680 req/s. On the gold host the same default
applies (cgroup quota → tier cores), so the non-monotonic curve is real deployment
behavior, not a harness artifact; operators should set `intra_op_threads`
explicitly (≤ 2 for small models) in production configs.

## Workloads

| Workload       | Driver             | Purpose                                                       |
|----------------|--------------------|---------------------------------------------------------------|
| `fixed-length` | `redis-benchmark`  | uniform short texts (~8 tokens), batching efficiency baseline  |
| `unique-text`  | `redis-benchmark`  | unique texts via `__rand_int__` (cache-cold, high token churn)|
| `mixed-length` | `bench/fargate/load.rb` | 80% short / 20% long texts, interleaved — padding-waste probe |
| `cache-hit`    | `redis-benchmark`  | server launched with `-cache auto`, warmed once               |

## Metrics per cell

`{vCPU tier} × {clients} × {pipeline} × {workload}` → req/s, p50/p90/p99 ms, plus
**padding efficiency** for mixed-length (real tokens / processed token-slots,
computed with the real tokenizer over a `max_batch` window). Each cell is the
median of 3 runs.

**Diff gates:** inference-bound cells (<5000 req/s) require req/s ≥ −5% and
p50 ≤ +10%. Saturated cells (≥5000 req/s, e.g. cache-hit) finish in
sub-millisecond windows where timer quantization dominates, so they gate on
p50 ≤ +10% only.

## Usage

```
nix develop
just bench-fargate-baseline          # writes bench/fargate/baseline.<sha>.json
just bench-fargate                   # writes bench/fargate/out/run.<sha>.<ts>.json
just bench-fargate-diff <a.json> <b.json>   # per-cell PASS/FAIL vs tolerances
```

`docker` (and `git`) are not part of the `nix develop` profile, so the harness
resolves them with absolute-path fallbacks (`/usr/local/bin`, `/opt/homebrew/bin`,
`/usr/bin`, `~/.local/bin`). Set `PATH` or add `/usr/local/bin/docker` (etc.)
yourself if Docker lives somewhere else.

Flags for `go run ./bench/fargate`: `-platform -cpus -clients -pipeline -count
-model-dir -mode (run|baseline|diff) -before -after -req-gate -p50-gate
-short-len -long-len -short-ratio -skip-build -keep`.

## Layout

```
bench/fargate/
  main.go          # orchestrator: build image, run tiers, workloads, emit/diff JSON
  load.rb          # pure-stdlib Ruby mixed-length RESP driver (latencies)
  README.md        # this file
```