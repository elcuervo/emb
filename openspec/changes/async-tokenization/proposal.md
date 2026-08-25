## Why

`processBatch` (pipeline.go:34) tokenizes the entire batch synchronously in the worker goroutine, then calls the ONNX run — the session idles during tokenization. TEI decouples tokenization into dedicated `--tokenization-workers`, overlapping the tokenize of batch N+1 with the inference of batch N. On Graviton CPU serving, where inference is latency-critical and cores are the scarce resource, hiding tokenization off the critical path is a direct throughput win.

## What Changes

- Extract tokenization out of `processBatch`/`Worker.process` into a **producer stage**: a set of tokenizer goroutines that consume texts from a bounded queue and emit `Encoding`s (or pre-padded batches) to the run loop.
- A single **run loop** consumes encodings, packs them (respecting the P1 token budget and `max_batch`), runs the session, and distributes results to each request's `Result` channel — order preserved.
- New config `tokenize_workers` (default `min(4, cores)`, `0` disables and keeps current serial behavior). Applies to both batcher and worker-pool paths through a shared pipeline.
- Tokenization errors surface on that request's channel exactly as today; lifecycle (`Close`) drains the queue.
- No RESP protocol change, no embedding-output change.

## Capabilities

### New Capabilities

- `async-tokenization`: dedicated tokenizer workers overlapping tokenization with inference behind a bounded queue.

### Modified Capabilities

- `smart-batching`: the batching window now consumes pre-tokenized encodings from the async producer stage.

## Impact

Files: `internal/pipeline/tokenizer.go` (new producer stage), `internal/pipeline/batcher.go` + `pool.go` (consume encodings, feed run loop), `internal/config/config.go` (+`TokenizeWorkers`), `internal/registry/registry.go`. No protocol, onnx, or embedding changes. Tokenizer is already goroutine-safe (used concurrently today).

## Validation

All inside `nix develop`, diffed against the P0 golden baseline (linux/arm64):

```
$ nix develop
$ just build && just bench-fargate-diff <baseline> <after>
```

- **Overlap gate:** instrument wall-time vs sum of `Run()` times; tokenization wall share drops to ≤ 5% of request wall time (today ~15-25%).
- **Throughput gate:** fixed-length workload req/s ≥ **+15%** at 8 vCPU; mixed-length ≥ **+15%**.
- **Tail gate:** p99 under the existing stability gate (loaded p99 / idle p99 ≤ 1.5) — no queueing regressions.
- **Correctness gate:** `just verify-embeddings` passes unchanged.