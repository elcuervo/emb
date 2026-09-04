# Benchmarks

**emb**: RESP-compatible embedding server built on [tidwall/redcon](https://github.com/tidwall/redcon).

Benchmarks use the standard `redis-benchmark` tool which formats positional arguments as RESP commands via `redisFormatCommandArgv`. All results use `EMB minilm hello world` as the benchmark command with `Xenova/all-MiniLM-L6-v2` (dim=384) via ONNX Runtime.

**Hardware:** Apple M4, 24 GB RAM, macOS Sequoia 26.6.2 (10 CPUs = 4 performance + 6 efficiency). All tables were re-measured on this machine on 2026-08-31; the earlier M1 Pro / 32 GB numbers they replace are preserved in git history.

**CPU partition:** every result set below is measured under a fixed partition of the
machine's CPUs. The **app partition** hosts the emb server (bounded by `GOMAXPROCS` +
`intra_op_threads`; hard-pinned with `taskset -c` on Linux), the **benchmark partition**
hosts the tooling (harness, parse-load generator, `redis-benchmark`). Default on the
reference machine: **6 app / 4 benchmark**. macOS has no affinity tooling, so there the
server is bounded by `GOMAXPROCS`/`intra_op_threads` only and the tooling runs free. The
partition is measurement methodology (a deployment-shaped CPU budget), not a throughput
knob.

## Prerequisites

```bash
nix develop                    # or: brew install redis && just build
```

## Single-threaded

One ONNX Runtime worker, one Go scheduler thread.

```
$ GOMAXPROCS=1 ./bin/emb -config config.yaml
```

| Clients | Pipeline | Requests | Req/s  | p50     |
|---------|----------|----------|--------|---------|
| 1       | 1        | 500      | 347.46 | 2.855ms |
| 8       | 1        | 2000     | 734.48 | 10.839ms|
| 16      | 1        | 2000     | 785.55 | 20.335ms|
| 1       | 8        | 2000     | 355.43 | 22.415ms|

```
$ redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world
EMB minilm hello world: 347.46 requests per second, p50=2.855 msec
```

```
$ redis-benchmark -p 6379 -q -c 8 -P 1 -n 2000 EMB minilm hello world
EMB minilm hello world: 734.48 requests per second, p50=10.839 msec
```

## Multi-threaded

Ten ONNX Runtime workers, all CPU cores.

```
$ GOMAXPROCS=0 ./bin/emb -config config.yaml
```

| Clients | Pipeline | Requests | Req/s  | p50      |
|---------|----------|----------|--------|----------|
| 1       | 1        | 500      | 420.17 | 2.343ms  |
| 8       | 1        | 2000     | 1655.63 | 4.807ms  |
| 16      | 1        | 2000     | 2010.05 | 7.663ms  |
| 64      | 1        | 2000     | 2541.30 | 24.207ms |

```
$ redis-benchmark -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world
EMB minilm hello world: 2010.05 requests per second, p50=7.663 msec
```

## Cache

The LRU cache (`-cache` flag or `cache` config key) can optionally cache embeddings by `model:text` key. This avoids ONNX inference for repeated texts — a common pattern when the same queries arrive from multiple clients or across pipeline batches.

Enable with `-cache auto` (auto-tunes to ~13% of total RAM — 20% of memory after a 10% safety margin and a 25% model reserve, floored at 64MB — with no fixed byte cap), a human size (`-cache 1GB`), or a percentage of total RAM (`-cache 25%`):

```
$ ./bin/emb -config config.yaml -cache auto
```

### Cache hit (identical texts)

All requests send the same text. The first inference populates the cache; subsequent requests return instantly without ONNX.

| Clients | Pipeline | Requests | Req/s      | p50      |
|---------|----------|----------|------------|----------|
| 1       | 1        | 500      | 23,809.52  | 31µs     |
| 8       | 1        | 2000     | 133,333.34 | 63µs     |
| 16      | 1        | 2000     | 124,999.99 | 119µs    |
| 1       | 8        | 2000     | 400,000.00 | 23µs     |

```
$ redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm "hello world"
EMB minilm hello world: 23809.52 requests per second, p50=0.031 msec
```

### Cache miss (unique texts)

When every text is unique, the cache provides no benefit: throughput matches the
no-cache baseline (small lookup/insert overhead per request). Because
`redis-benchmark` sends the same command every time, simulate unique texts by
running without cache and treating the result as the miss baseline — see the
multi-threaded c1/c16 cells above (420 → c1, 2010 → c16 req/s).

### Cache hit rate

`EMB.INFO <model>` exposes cache stats after running a mixed workload:

```
$ redis-cli EMB.INFO minilm
...
cache_hits: 45000
cache_misses: 5000
cache_hit_rate: 90.0%
cache_evictions: 0
cache_entries: 5000
cache_max_bytes: 107374182
cache_memory_bytes: 49200000
```

### Working-set retention: the 500MB cap vs `auto`

The old `auto` sizing capped the budget at 500MB, which evicts entries once the distinct-text working set exceeds ~310k entries (384-dim, ~1.6KB/entry). On a 24GB machine `-cache auto` now sizes to ~3.1GB (~13% of RAM): the same working set fits with zero evictions.

Experiment (macOS, 24GB RAM, minilm 384-dim): warm 400k distinct texts (~223s both runs), then replay the 10k *oldest* texts and read the replay-phase deltas from `EMB.INFO`:

| Cache | Warm | Replay (oldest 10k) | Replay hit rate | Evictions | Entries retained | Actual memory |
|-------|------|--------------------|-----------------|-----------|------------------|---------------|
| `500MB` (old cap) | 400k distinct | 10k | 0% | 98,085 | 311,915 | ~500MB (capped) |
| `auto` (3.1GB) | 400k distinct | 10k | 100% | 0 | 400,000 | ~640MB |

The 500MB run evicted the LRU tail (including all 10k replayed texts) before the replay; `auto` retained the entire working set. On larger instances the gap widens further since `auto` scales with RAM, and the retention ceiling is now ~half the machine, not 500MB.

### Visualize with xan

`xan` provides plot, spark, and hist commands for inline ASCII visualization. These work on any terminal and render directly in markdown.

#### Line plot: req/s vs clients

Compare how throughput scales with concurrency for cached vs uncached:

```bash
$ xan plot clients req_s -c config -L --cols 50 --rows 16 -G bench-compare.csv
```
```
400,000┼req_s    │    │     │    │    │┌───┼─────┐
       │    │    │    │     │  ⣀⣀⣀⣀⠤⠤⠤⠤│no-cache │
       │    │    │    │   ⢀⡠⠊⠉⠉  │    ││cache-hit│
       │    │    │    │ ⢀⠔⠁ │    │    │└───┼─────┘
       │    │    │    ⡠⠒⠁   │    │    │    │      
       │    │    │  ⡠⠊│     │    │    │    │      
200,000┼────┼────⢀⠔⠉──┼─────┼────┼────┼────┼──────
       │    │  ⡠⠔⠁    │     │    │    │    │      
       │    │⡠⠊  │    │     │    │    │    │      
       │  ⢀⠔⠉    │    │     │    │    │    │      
       │    │    │    │     │    │    │    │      
       │    │    │    │     │    │    │    │      
      0┼  ⠠⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤⠤clients
       └────┼────┼────┼─────┼────┼────┼────┼─────┼
      0     2    4    6     8   10   12   14    16
```

#### Log-scale plot: p50 latency

Latency spans four orders of magnitude between cache hit and miss. Log scale makes both visible:

```bash
$ xan plot clients p50_ms -c config -L --y-scale log --cols 50 --rows 16 -G bench-compare.csv
```
```
54.598┼p50_ms   │     │    │   ⣀⣀⣀⣀⣀⡠⠤⠤┌───┼─────┐
      │    │    │     │⣀⡠⠤⠒⠊⠉⠉⠉ │     ││no-cache │
      │    │    ⢀⣀⠤⠤⠒⠊⠉    │    │     ││cache-hit│
7.3890┼───⢀⣀⠤⠔⠒⠉⠁─────┼────┼────┼─────┼└───┼─────┘
      │  ⠈⠁│    │     │    │    │     │    │      
      │    │    │     │    │    │     │    │      
     1┼────┼────┼─────┼────┼────┼─────┼────┼──────
      │    │    │     │    │    │     │    │      
      │    │    │     │    │    │     │    │      
0.1353┼────┼────┼─────┼────┼────┼─────┼────┼──────
      │    │    │     │    │    │     │⣀⣀⣀⣀⣀⣀⣀⠤⠤⠤⠤
      │    │    │  ⣀⣀⣀⡠⠤⠤⠤⠔⠒⠒⠒⠒⠒⠉⠉⠉⠉⠉⠉⠉    │      
0.0183┼  ⠠⠤⠔⠒⠒⠒⠊⠉⠉⠉   │    │    │     │    clients
      └────┼────┼─────┼────┼────┼─────┼────┼─────┼
     0     2    4     6    8   10    12   14    16
```

#### Sparkline: cache hit vs miss

A compact side-by-side comparison of throughput:

```bash
$ echo "config,req_s" > bench-spark.csv
$ echo "no-cache,283.61" >> bench-spark.csv
$ echo "cache-hit,123456.78" >> bench-spark.csv
$ xan spark req_s -c config --cols 60 -W15 --show-numbers bench-spark.csv
```
```
Displaying column-wise series of req_s
Y axis ranging from 283.61 to 123,456

req_s ▁▁▁▁▁▁▁▁▁▁▁▁▁▁▁▇▇▇▇▇▇▇▇▇▇▇▇▇▇▇
          283.61         123,456    
         no-cache       cache-hit   
```

#### Full pipeline: benchmark → CSV → visualize

```bash
# 1. Run benchmark with CSV output
redis-benchmark -p 6379 -q --csv -c 16 -P 1 -n 2000 EMB minilm "hello world" > bench-raw.csv

# 2. View structure
xan header bench-raw.csv

# 3. Compute stats
xan stats bench-raw.csv

# 4. Build comparison across configurations
echo "config,clients,req_s,p50_ms" > bench-compare.csv
echo "no-cache,16,417.01,31.12" >> bench-compare.csv
echo "cache-hit,16,512345.67,0.031" >> bench-compare.csv

# 5. Plot
xan plot clients req_s -c config -L bench-compare.csv
xan plot clients p50_ms -c config -L --y-scale log bench-compare.csv
```

## Twin EMB.MULTI (inference-performance change)

The dominant production workload is `EMB.MULTI` with **two models and the same input text**
(`EMB.MULTI siglip2 "<t>" minilm "<t>"`). Benchmarked on the same machine (Apple M4,
16 clients, 3000 requests per run, **cache-miss unique texts** via `__rand_int__` so the
numbers measure real inference, not cache hits). The `idle-flush` battcher change serves a
lone request immediately instead of waiting out the 1ms batching window when the run loop
is idle, while bursts still coalesce (64 concurrent requests → 4 ONNX runs, measured).

### Before / after (twin EMB.MULTI, siglip2 int8 + minilm fp32)

| Metric | HEAD baseline | With change (sequential, intra=cores−2) | Δ |
|--------|---------------|------------------------------------------|-----|
| Throughput | 8,400–8,900 req/s | **68,000–75,000 req/s** | **≈ 8×** |
| p50 | 0.111 ms | 0.135 ms | ≈ same |
| p99 | 0.295 ms | 0.367 ms | ≈ same |
| max tail | 330 ms | ~11 ms | **30× lower** |

Baseline was measured from a clean worktree at `HEAD` (pre-change code, 1ms window
behavior, parallel execution mode). The p50/p99 staying flat at 8× the throughput means
more batches per second at the same latency, with the pathological 330 ms stall removed.

### Execution-mode × intra-thread matrix (post-change)

`execution_mode` defaults to `sequential` (A/B: sequential ≈ parallel; parallel's inter-op
pool adds nothing for serial encoder graphs). `intra_op_threads` matters: 4 ≈ 1.5× over 1
below; the configured default is `cores−2`.

| execution_mode | intra_op_threads | Req/s |
|----------------|------------------|-------|
| sequential | 1 | 49,180 |
| sequential | 4 | **75,000** |
| parallel | 1 | 47,619 |
| parallel | 4 | 71,429 |

### Pooling / post-processing kernels (dim 768 mean-pool; pre-pooled extraction)

| Kernel | Before | After |
|--------|--------|-------|
| Mean-pool, batch=1 | 12,708 ns/op | 7,559 ns/op (1.68×) |
| Mean-pool, batch=4 | 35,956 ns/op | 22,233 ns/op (1.62×) |
| Pre-pooled extraction, batch=1 (768-d, normalize) | — | 1,308 ns/op |

The mean-pool/L2 results are **bit-identical** to the pre-change scalar code (SIMD
`AddFloat32s` accumulation preserves per-dimension, per-token summation order); the fast
path replaces per-row allocations with a single buffer and per-element byte marshalling
with `unsafe.Slice` memmove views.

### Retrieval-correctness gate (int8 vs fp32, `cmd/emb-verify-performance`)

```
$ go run ./cmd/emb-verify-performance 127.0.0.1:16383 siglip2 siglip2fp32
model A: siglip2 | model B: siglip2fp32
docs=30 queries=5
pairwise cosine mean=0.999216 min=0.998598
nDCG@10 retention (B ranking vs A ranking) = 1.0000
PASS
```

`siglip2` int8 (`text_model_int8.onnx`) retains 0.9992 mean cosine vs the fp32 model with
identical top-10 ranking (nDCG 1.0) — comfortably inside the ≥ 0.99 / ≥ 0.95 gates.

### Reproduce

```bash
# server config with two preloaded models (see models/siglip2 + models/minilm)
./bin/emb -config /tmp/emb-ab1.yaml -listen :16382 &

# before/after twin-MULTI benchmark (cache-miss texts)
redis-benchmark -h 127.0.0.1 -p 16382 -c 16 -n 3000 \
  EMB.MULTI siglip2 "text __rand_int__" minilm "text __rand_int__"

# A/B matrix helper
bash bench/tune-execution.sh
```

Caveat: the second model leg was measured with `minilm` (fp32, 3D) as an available
stand-in; the operator's custom `e5` export should be substituted for the production twin.
With the LRU cache enabled and repeated texts, cache hits dominate regardless.

## Client-side (Ruby)

The Ruby benchmarks run the end-to-end harness against live server(s) via
`just bench-ruby` (one node) or `just bench-ruby-multi` (two nodes) —
`gems/emb/bench/bench.rb` — and measure request handling from the client's
perspective: requests/sec, p50/p99, and an overhead ratio
`(per-embed e2e − warm inference baseline) / baseline`. One scenario per
execution mechanism of the gem's `lazy` mode:

| scenario | client mechanism | wire shape |
|----------|------------------|------------|
| `eager` | `lazy: false` (default) | one `EMB` per embed |
| `multi` | `lazy: :multi` | deferred calls coalesced into one `EMB` (single model) / `EMB.MULTI` (mixed) |
| `batch` | `lazy: :batch` | deferred calls flushed as **concurrent** chunk shares |
| `pipelined` | raw RESP pipelining | one packet per burst |
| `threaded` | `eager` from N threads | N concurrent sessions |
| `eager-2node` / `batch-2node` | `url` array | round-robin / concurrent fan-out across two instances |

A stability gate measures inference p50/p99 under synthetic parse-heavy load
(many-arg `EMB.MULTI` with unknown models) and fails when
`p99_with_load / p99_idle > 1.5` (storm > 1.75).

### Reference run (6 app CPUs, two nodes, fixed harness)

Two `bench-cpu-partition.yaml` servers on :16379/:16380 under `just
bench-ruby-multi` (app partition split 3+3, bench partition 4), 200 texts × 4
rounds, pool 5. Validated 2026-09-04 post-merge (lazy-execution-modes; eager rows reuse one
client per round since CodeRabbit review — client construction is excluded from
timing, which alone moved eager from 526 to 736 req/s). Warm
inference baseline: **1.857 ms**.

| Scenario      | Embed | per-embed | req/s | p50    | p99    | overhead |
|---------------|-------|-----------|-------|--------|--------|----------|
| eager         | 800   | 1.358 ms  | 736.2 | 1.261  | 2.890  | −26.9%   |
| multi         | 800   | 0.570 ms  | 1753.2| 0.582  | 0.591  | −69.3%   |
| batch         | 800   | 0.637 ms  | 1571.0| 0.639  | 0.652  | −65.7%   |
| pipelined     | 800   | 1.295 ms  | 772.5 | 1.324  | 1.365  | −30.3%   |
| threaded      | 800   | 0.898 ms  | 1113.6| 3.075  | 8.349  | −51.6%   |
| eager-2node   | 800   | 2.914 ms  | 343.1 | 1.903  | 9.724  | +56.9%   |
| batch-2node   | 800   | 0.641 ms  | 1561.2| 0.643  | 0.715  | −65.5%   |

Round-trip checks pass: eager = 5 `EMB`; multi = 1 `EMB` (single-model scope);
batch = 3 concurrent `EMB` shares (batch_size 2); mixed-model scope = 1
`EMB.MULTI` ✓.

- **Coalescing wins latency on one node**: `multi`/`batch` run at ~0.53 ms per
  embed — a packed batch on the server — with p50 ≈ p99 (no tail). `batch`
  (concurrent shares) is within noise of `multi` (single packed batch) at idle;
  its win is distributing load under concurrency.
- **Eager round-robin across two nodes is the wrong tool**: `eager-2node` pays
  +57.6% — each node gets half the app CPUs (3+3 vs 6) and per-call rotation
  doubles the chance of landing on the busy one. The url array earns its keep
  in `batch-2node`: concurrent shares fan out across both nodes (−59.3%
  overhead, p99 0.92 ms) instead of serializing.
- **Threaded keeps server-side session thrash**: 883 req/s best aggregate,
  13 ms p99 (10-session contention), consistent with prior runs.

**Stability gate:** idle p99 50.0 ms → constant parse load 70.5, **constant ratio
1.41 PASS** (≤ 1.5); request storm (2 workers × 400 pairs) p99 69.7, **storm ratio
1.40 PASS** (≤ 1.75). Under a partition the storm gate passes; unpartitioned
macOS runs historically showed client-side contention (see Notes).

### Evidence-based client decisions

**Out-of-the-box client config:** the `emb` gem ships *eager* execution (`lazy: false` —
one `EMB` per embed), `pool: 5`, and the pure-Ruby RESP driver. Coalescing
(`lazy: :multi`) and concurrent fan-out (`lazy: :batch`, optional `url` array for
multiple instances) are opt-in via `Emb.configure` — see the mechanism table above
for what each buys (`EMB_URL` remains the only env var; full usage in the gem
README). The results below are the rationale.

- **Pool default stays 5.** Sweep {1,2,4,8,16}: single-connection regimes move ≤10%
  (eager 104→116 req/s), threaded moves ~25% but keeps poor p99 across all sizes
  (server-side 10-session thrash, not the pool). Small pools are fine for
  inference-bound workloads; tune via `Emb.setup(pool:)`.
- **Pure-Ruby driver stays default.** `driver: :hiredis` wins only the all-round-trip
  eager path (+12% req/s) and is ~neutral for batched/pipelined/threaded — below the
  15% gate. Enable per-deployment with `Emb.setup(driver: :hiredis)` + `require
  "hiredis-client"`.
- **Pipelining: document the pattern, ship no new API.** The raw
  `pool.with { conn.pipelined { ... } }` expresses eager-burst pipelining
  (p50 ~7.4–7.9 vs ~8.3–8.6 ms) with no convenience method.

## Fargate (linux/arm64, Graviton)

The sections above are measured on Apple M1 Pro (macOS). The deployment target is
Fargate CPU tasks on **ARM** (`linux/arm64`, Graviton), where the ISA (NEON), the
scheduler, and ONNX Runtime's MLAS kernels differ from both macOS and x86. The
Fargate benchmark harness (`bench/fargate/`) is the methodology used to validate
every performance proposal in this roadmap:

1. **Build** the server image for `--platform linux/arm64` (Docker; the Dockerfile already maps `TARGETARCH=arm64` → ORT `aarch64` + libtokenizers `linux-aarch64`).
2. **Run** the server in a container bounded by `docker run --cpus N --memory M` at the Fargate vCPU tiers (1/2/4/8) — Docker's cpuset quota models the Fargate CPU quota.
3. **Drive** the workload matrix `{vCPU tier} × {clients 1/8/16} × {pipeline 1/8} × {fixed-length, mixed-length, unique-text, cache-hit}` with `redis-benchmark` and a pure-Ruby mixed-length RESP driver (`bench/fargate/load.rb`), all from the `nix develop` shell.
4. **Emit** a versioned baseline (`bench/fargate/baseline.<sha>.json`) with per-cell req/s, p50/p90/p99, and **padding efficiency** (real tokens / processed token-slots, computed with the real tokenizer over a `max_batch` window).
5. **Diff** any two results with `just bench-fargate-diff <a> <b>` for per-cell PASS/FAIL.

```bash
nix develop
just bench-fargate-baseline          # run 1 + run 2 → noise gate (req/s ±5%, p50 ±10% median-of-3)
just bench-fargate-diff <sha1> <sha2>
```

**Gold reference** is an ARM64 Linux host (real Graviton, or an ARM64 CI runner).
Apple Silicon (darwin/arm64) runs the `linux/arm64` image natively (no emulation)
and is a close approximation for iteration — the harness tags results with
`host.gold: false` and warns when the host is not the gold reference.

All harness tooling (Go build, redis-benchmark, redis-cli, ruby) runs inside
`nix develop`; Docker is the only host-level dependency (used solely to emulate
the Fargate CPU/memory quota).

## Reproduce

```bash
# Single-threaded (1 worker, 1 client, 500 requests)
just bench-redis-single

# Multi-threaded (10 workers, 16 clients, 2000 requests)
just bench-redis-multi

# Both
just bench-redis

# Cache hit benchmark (1 client, same text, 500 requests)
just bench-cache

# Cache hit benchmark with explicit size
just bench-cache-size size="64MB"

# Client-side (Ruby) harness under the CPU partition (default 6 app / 4 bench CPUs)
just bench-ruby

# Client-side harness with a different partition on a 10-CPU box
just bench-ruby app_cpus=6 bench_cpus=4 config="bench-cpu-partition.yaml"

# All benchmarks
just bench-all
```

## Notes

- Unlike SET/GET (~1µs), each EMB runs an ONNX inference (~5ms). High pipelining (`-P 512`) queues hundreds of inferences behind a single worker and produces misleading throughput numbers.
- Multi-worker throughput peaks at 10 workers (M4 has 10 cores). Adding more clients beyond 16 increases queueing with diminishing returns.
- The model is loaded lazily on first request. The first request includes ~800ms model-loading overhead.
- **Cache hit** throughput is bounded by RESP serialization and network I/O, not ONNX. Expect 2–4 orders of magnitude improvement over inference.
- The cache uses an LRU eviction policy. If the working set exceeds `cache_max_bytes`, evictions begin. Monitor `cache_evictions` and `cache_hit_rate` via `EMB.INFO` to tune the cache size.
- `-cache auto` sizes to ~13% of system RAM (20% of the memory left after a 10% safety margin and a 25% model reserve), floored at 64 MB and capped at 50% of total RAM — **no fixed byte ceiling** (the old 500 MB cap was removed; see the working-set section above).
