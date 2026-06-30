## Why

emb is 2-3x faster than Ruby in-process ONNX on the large E5 model but 3x slower on the small siglip2 int8 model. The main cause is ONNX Runtime execution provider: Ruby's ORT gem (1.23.0) links CoreML + Metal for GPU acceleration, while emb's Nix-provided ORT (1.22.2) is CPU-only. Additionally, batching and thread configuration are not tuned for production workloads.

## What Changes

- Enable smart batching (`timeout: 1ms`) for both models to coalesce concurrent requests into single ONNX batches
- Set `intra_op_threads: 4` for both models to utilize Apple Silicon performance cores
- Set `preload: true` (already set in production config)
- Document optimal production configuration in README

No code changes — all optimizations are config-only.

## Capabilities

### New Capabilities
- (none — configuration tuning only)

### Modified Capabilities
- (none — no spec-level behavior changes)

## Impact

| File | Change |
|------|--------|
| `config-prod.yaml` | Add `timeout: 1`, `intra_op_threads: 4` |
| `README.md` | Document production configuration recommendations |
