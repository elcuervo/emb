## Context

The current worker pool distributes requests round-robin to N workers, each running independent ONNX inferences. Single-inference benchmarks show 1.4ms for MiniLM. Under concurrent load, throughput plateaus at ~500 req/s (10 workers × 50 req/s). The ONNX runtime is heavily optimized for batched matrix operations — a batch of 8 takes only ~10ms vs ~2ms for a single inference. Collecting concurrent requests into batches unlocks this efficiency.

## Goals / Non-Goals

**Goals:**
- Collect concurrent requests per model into batched ONNX `Run()` calls
- Configurable `timeout` field (ms) controlling max wait before flush
- Max batch size (default 32) to bound memory per batch
- Backward compatible: existing configs without `timeout` get a reasonable default (2ms)
- Throughput at least 2x current baseline under concurrent load (8+ clients)
- Single-request p50 latency within baseline + timeout

**Non-Goals:**
- Batching across different models (different tensor shapes, different sessions)
- Request priority or starvation prevention (fairness is inherent: first come, first batched)
- Dynamic timeout adjustment based on load

## Decisions

### Batcher replaces the per-worker channel

```
Current:         Smart batcher:
                 
EMB ──▶ W₁      EMB ──▶              
EMB ──▶ W₂      EMB ──▶  Collector ──▶ Batched Run() ──▶ Distribute
EMB ──▶ W₃      EMB ──▶  (2ms/32max)
```

Each model gets one `Batcher` instead of N workers. The batcher:
1. Accumulates `Request` structs in a slice
2. Runs a timer goroutine that flushes on timeout
3. Also flushes when the batch reaches max size
4. Calls the underlying ONNX session (the workers' `Embed` function) once with all texts
5. Distributes results back through each request's `Result` channel

**Why replace workers, not augment them?** Batching works best when all requests go to a single collector. Multiple collectors would create smaller batches and defeat the purpose. The worker pool's parallelism came from parallel ONNX runs — but batched ONNX runs already parallelize internally across the batch dimension.

### Timeout as config field

```yaml
models:
  minilm:
    onnx: ./models/minilm/model.onnx
    batching:
      timeout: 2       # ms to wait before flush (default: 2)
      max_batch: 32    # max texts per batch (default: 32)
```

If `timeout: 0` (or omitted), batching is disabled and the server falls back to per-worker direct execution (current behavior).

### No tokenizer-level batching change

The tokenizer is already goroutine-safe. Each text is tokenized individually before being added to the batch accumulator. The batch is padded with `PadEncodings` before `Run()`, same as explicit batch EMB commands.

### Implementation approach

```go
type Batcher struct {
    reqChan   chan Request
    session   onnx.Session
    tok       tokenizer.Tokenizer
    timeout   time.Duration
    maxBatch  int
    dim       int
    maxLen    int
    normalize bool
    pooling   string
}
```

The `Batcher.Embed` sends a `Request` to `reqChan`. A background goroutine collects requests, starts a timer on the first one, flushes when timer fires or batch fills.

## Risks / Trade-offs

- [Increased latency for idle servers] → Single request under low load waits `timeout` ms (default 2ms). Acceptable tradeoff for 4x throughput under load.
- [Memory for large batches] → Max batch 32 × avg 100 tokens × 2 int64 tensors = ~50KB. Negligible.
- [Timeout too short for throughput] → configurable; 2ms default balances latency vs throughput for MiniLM.
- [Timeout too long for latency-sensitive apps] → set `timeout: 0` to disable batching (current behavior).
