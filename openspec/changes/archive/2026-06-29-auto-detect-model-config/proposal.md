## Why

Adding a new model requires manually specifying `output_tensor`, `pooling`, and `normalize` — even though the ONNX graph structure tells us everything we need. A rank-2 output (e.g., `pooler_output`, `sentence_embedding`) is pre-pooled; a rank-3 output (`last_hidden_state`) needs mean pooling. Normalize is typically `true` for sentence-transformers models and `false` for raw LLM embeddings. Auto-detecting these eliminates trial-and-error config and lets users add models with just a path or repo name.

## What Changes

- `output_tensor` is auto-detected from available ONNX outputs: prefer rank-2 → fall back to rank-3
- `pooling` is inferred from output rank: rank-2 → `none`, rank-3 → `mean`
- `normalize` is inferred from model type heuristics (sentence-transformers → true, raw LLM → false)
- Existing explicit overrides for `output_tensor`, `pooling`, `normalize` still take precedence
- Config validation relaxed: `output_tensor` and `pooling` no longer required
- Logging reports the auto-detected choices at model registration time

## Capabilities

### New Capabilities
- `model-autoconfig`: Auto-detect output tensor name, pooling strategy, and normalization from ONNX graph structure and HuggingFace model metadata

### Modified Capabilities
- `model-loading`: Config validation relaxed — `output_tensor`, `pooling`, `normalize` become fully optional

## Impact

Files: `internal/registry/registry.go` (resolveModelConfig extended), `internal/onnx/runtime.go` (could add model-type heuristics from config.json). No changes to the RESP protocol, worker pool, or tokenizer. Backward compatible — any existing explicit config overrides continue to work.
