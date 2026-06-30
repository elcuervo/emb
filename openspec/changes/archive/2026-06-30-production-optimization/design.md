## Context

emb's production config at `unsplash-api/config/emb-config.yaml` runs both models with `batching: timeout: 0` (disabled batching) and default thread counts (`intra_op_threads: 0` → 1, `inter_op_threads: 0` → 2). The Ruby onnxruntime gem (v0.10.1) links CoreML+Metal for GPU acceleration; emb's Nix-provided ORT is CPU-only.

Benchmarks on M1 Pro with production models:

| Model | Ruby single | emb single | emb vs Ruby |
|-------|-------------|------------|-------------|
| Siglip2 (271M int8) | 14ms | 45ms | 0.31x |
| E5 (1.1GB) | 45ms | 22ms | 2.04x |

emb excels on the large E5 model. The siglip2 gap is due to CoreML acceleration in Ruby's ORT — emb's CPU-only ORT is inherently slower for small int8 models on this hardware.

## Goals / Non-Goals

**Goals:**
- Maximize emb throughput for production workloads
- Leverage concurrent request coalescing via smart batching
- Optimize CPU thread utilization
- Maintain perfect compatibility (cosine=1.0)

**Non-Goals:**
- Matching Ruby's siglip2 speed (requires CoreML ORT build — separate concern)
- Code changes (all optimizations are config-only)

## Decisions

### Enable smart batching (timeout: 1ms)

Currently `timeout: 0` means no batching — every request is a separate ONNX inference. Setting `timeout: 1` coalesces concurrent requests arriving within 1ms into a single batched inference.

```
timeout: 0                      timeout: 1
─────────                      ─────────
req1 → inference (1 batch)      req1 ─┐
req2 → inference (1 batch)      req2 ─┤→ inference (2 batches)
req3 → inference (1 batch)      req3 ─┘

3 inferences                     1 inference
```

Benefit grows with concurrency. 10 concurrent requests → 1 batch instead of 10 separate inferences. For both siglip2 (max_length=64) and E5 (max_length=512), batching is safe since the pipeline already handles variable-length padding.

### Thread tuning: intra_op_threads: 4

Apple Silicon M1 Pro has 8 performance cores + 2 efficiency cores. Setting `intra_op_threads: 4` allows ONNX ops to use 4 cores for parallel computation. The current default (1 thread) underutilizes the hardware.

`inter_op_threads` stays at default (2) — the model graph is linear (one op at a time), so inter-op parallelism offers minimal benefit for these models.

### Worker count

Current auto-tuning creates 10 workers per model (based on available RAM). This is high — each worker is a separate ONNX session. 10 sessions for a 1.1GB model = 11GB of model copies. With batching enabled, the batcher uses a single session, making worker count irrelevant.

For the non-batching fallback, worker count is capped by CPU cores, not RAM. The auto-detect logic could be refined, but this is not critical for the batched path.

## Risks / Trade-offs

- [Batching adds 1ms latency for the first request in batch] → Trade-off: +1ms latency per batch vs 2-10x throughput gain. Acceptable for production.
- [Batching with diverse request times] → A fast 7-token request waits for the 1ms batch window. Acceptable — 1ms is negligible vs 15-45ms inference time.
- [CoreML gap remains for siglip2] → emb is CPU-only, Ruby uses GPU. Acceptable — the production benefit is shared service + concurrency, not raw single-request speed.
