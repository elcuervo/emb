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

## Reproduce

```bash
# Single-threaded (1 worker, 1 client, 500 requests)
just bench-redis-single

# Multi-threaded (10 workers, 16 clients, 2000 requests)
just bench-redis-multi

# Both
just bench-redis
```

## Notes

- Unlike SET/GET (~1µs), each EMB runs an ONNX inference (~5ms). High pipelining (`-P 512`) queues hundreds of inferences behind a single worker and produces misleading throughput numbers.
- Multi-worker throughput peaks at 10 workers (M1 Pro has 10 cores). Adding more clients beyond 16 increases queueing with diminishing returns.
- The model is loaded lazily on first request. The first request includes ~800ms model-loading overhead.
