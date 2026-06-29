## Why

The server handles each `EMB` command as an independent inference. Our benchmarks show that a batch of 4 texts takes the same time as a single text (~18ms). Batching concurrent requests together can multiply throughput without multiplying hardware. For small embedding models like MiniLM, the ONNX runtime processes batch dimensions far more efficiently than individual requests.

```
Current:  each EMB → one Run() → 20ms → 50 req/s per worker × 10 = 500 req/s
Smart:    collect requests → batch Run() → 4× throughput without more workers
```

## What Changes

- Replace the per-worker request channel with a batch collector per model
- The collector accumulates requests over a configurable window (default 2ms) or up to a max batch size (default 32)
- When the window expires or the batch fills, all accumulated texts are sent as a single ONNX `Run()` call
- Results are distributed back to each requester based on their position in the batch
- The `timeout` field configures the max wait time for batching
- Existing `pooling: none` and `output_tensor` features work transparently through the batcher

## Capabilities

### New Capabilities

- `smart-batching`: Batch concurrent embedding requests into single ONNX inference calls, controlled by a configurable `timeout` (max wait ms) and max batch size

## Impact

Files: `internal/pipeline/pool.go` (new `Batcher`), `internal/config/config.go` (new `Timeout` field), `internal/registry/registry.go` (pass timeout to pool). No changes to the RESP protocol, ONNX runtime, or pooling logic. Throughput improved for concurrent workloads; single-request latency may increase by up to `timeout` ms.

## Benchmarks

Current at concurrency=8, seq_len=16:
```
Before: 509 req/s  P50=11ms  P90=27ms
After:  ~1200 req/s P50=8ms   P90=12ms  (projected)
```

Baseline captured in `benchmark-baseline.txt`. Pre/post comparison required.
