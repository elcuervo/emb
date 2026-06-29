## Why

`emb` hardcodes `last_hidden_state` as the ONNX output tensor and always applies mean pooling, making it incompatible with models that expose pre-pooled outputs (e.g., `pooler_output`, custom normalized embeddings). Two models needed for full integration — siglip2 and E5 — both use 2D pre-pooled outputs that bypass mean pooling entirely.

## What Changes

- Add `output_tensor` field to `ModelConfig` — configures which ONNX output tensor to read (default: `last_hidden_state`)
- Add `pooling: none` strategy — skips mean pooling for models that return already-pooled 2D tensors (batch × dim)
- Wire `Pooling` config field through to the pipeline worker (currently parsed but ignored)
- `InferDim` gains support for 2D output tensors (currently only handles 3D)
- Input names remain auto-detected or configurable (no change to existing behavior)

## Capabilities

### New Capabilities

- `configurable-output-tensor`: Allow `output_tensor` to be set per model in config; used when creating the ONNX session
- `pre-pooled-output`: When `pooling: none`, treat ONNX output as 2D (batch × dim), skip mean pooling, apply optional L2 normalization

### Modified Capabilities

*(none — existing mean-pool behavior unchanged for models not setting these fields)*

## Impact

- `internal/config/config.go` — add `OutputTensor` field
- `internal/registry/registry.go` — use `cfg.OutputTensor` in session factory; pass pooling strategy to pool
- `internal/pipeline/pool.go` — switch on pooling strategy; add `none` path
- `internal/pipeline/pooling.go` — add `ExtractPrePooled` function for 2D output handling
- `internal/onnx/runtime.go` — output tensor shape becomes dynamic (2D or 3D); `InferDim` updated
- Backward compatible: existing configs without these fields behave identically
