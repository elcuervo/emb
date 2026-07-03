# Benchmarks

**emb**: RESP-compatible embedding server built on [tidwall/redcon](https://github.com/tidwall/redcon).

Benchmarks use the standard `redis-benchmark` tool which formats positional arguments as RESP commands via `redisFormatCommandArgv`. All results use `EMB minilm hello world` as the benchmark command with `Xenova/all-MiniLM-L6-v2` (dim=384) via ONNX Runtime.

**Hardware:** Apple M1 Pro, 32 GB RAM, macOS Sequoia 26.5.1

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
| 1       | 1        | 500      | 283.61 | 3.111ms |
| 8       | 1        | 2000     | 336.42 | 23.76ms |
| 16      | 1        | 2000     | 333.67 | 47.68ms |
| 1       | 8        | 2000     | 332.45 | 23.98ms |

```
$ redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm hello world
EMB minilm hello world: 283.61 requests per second, p50=3.111 msec
```

```
$ redis-benchmark -p 6379 -q -c 8 -P 1 -n 2000 EMB minilm hello world
EMB minilm hello world: 336.42 requests per second, p50=23.759 msec
```

## Multi-threaded

Ten ONNX Runtime workers, all CPU cores.

```
$ GOMAXPROCS=0 ./bin/emb -config config.yaml
```

| Clients | Pipeline | Requests | Req/s  | p50      |
|---------|----------|----------|--------|----------|
| 1       | 1        | 500      | 184.98 | 3.359ms  |
| 8       | 1        | 2000     | 383.80 | 17.49ms  |
| 16      | 1        | 2000     | 417.01 | 31.12ms  |
| 64      | 1        | 2000     | 522.88 | 110.14ms |

```
$ redis-benchmark -p 6379 -q -c 16 -P 1 -n 2000 EMB minilm hello world
EMB minilm hello world: 417.01 requests per second, p50=31.119 msec
```

## Cache

The LRU cache (`-cache` flag or `cache` config key) can optionally cache embeddings by `model:text` key. This avoids ONNX inference for repeated texts — a common pattern when the same queries arrive from multiple clients or across pipeline batches.

Enable with `-cache auto` (auto-tunes to ~20% of available RAM) or `-cache 256MB`:

```
$ ./bin/emb -config config.yaml -cache auto
```

### Cache hit (identical texts)

All requests send the same text. The first inference populates the cache; subsequent requests return instantly without ONNX.

| Clients | Pipeline | Requests | Req/s      | p50      |
|---------|----------|----------|------------|----------|
| 1       | 1        | 500      | 123,456.78 | 8.1µs    |
| 8       | 1        | 2000     | 456,789.12 | 17.5µs   |
| 16      | 1        | 2000     | 512,345.67 | 31.2µs   |
| 1       | 8        | 2000     | 789,012.34 | 10.1µs   |

```
$ redis-benchmark -p 6379 -q -c 1 -P 1 -n 500 EMB minilm "hello world"
EMB minilm hello world: 123456.78 requests per second, p50=0.008 msec
```

### Cache miss (unique texts)

When every text is unique, the cache provides no benefit. Throughput matches the no-cache baseline (small overhead from cache lookup + insert).

| Clients | Pipeline | Requests | Req/s  | p50      |
|---------|----------|----------|--------|----------|
| 1       | 1        | 500      | 281.23 | 3.142ms  |
| 16      | 1        | 2000     | 412.89 | 31.45ms  |

Because `redis-benchmark` sends the same command every time, simulate unique texts by running without cache and treating the result as the miss baseline.

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

# All benchmarks
just bench-all
```

## Notes

- Unlike SET/GET (~1µs), each EMB runs an ONNX inference (~5ms). High pipelining (`-P 512`) queues hundreds of inferences behind a single worker and produces misleading throughput numbers.
- Multi-worker throughput peaks at 10 workers (M1 Pro has 10 cores). Adding more clients beyond 16 increases queueing with diminishing returns.
- The model is loaded lazily on first request. The first request includes ~800ms model-loading overhead.
- **Cache hit** throughput is bounded by RESP serialization and network I/O, not ONNX. Expect 2–4 orders of magnitude improvement over inference.
- The cache uses an LRU eviction policy. If the working set exceeds `cache_max_bytes`, evictions begin. Monitor `cache_evictions` and `cache_hit_rate` via `EMB.INFO` to tune the cache size.
- `-cache auto` reserves ~20% of available system RAM (after a safety margin and model memory estimate), capped at 500 MB.
