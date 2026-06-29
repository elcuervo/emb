## Why

Model configs currently require manually specifying `dim`, `max_length`, `pooling`, and `normalize` — all of which are knowable by inspecting the ONNX graph and tokenizer files. This makes adding a new model a multi-line edit with room for error (wrong dim, wrong max_length). Auto-detecting these fields eliminates the friction and removes a class of configuration bugs.

## What Changes

- `dim` is inferred from the ONNX output tensor shape
- `max_length` is inferred from the tokenizer's `max_length` or model's `max_position_embeddings` in `config.json`
- `pooling` defaults to `mean` (configurable override)
- `normalize` defaults to `true` (configurable override)
- `onnx` is the only required field per model (alongside `tokenizer`, or inferred from `model_repo`)
- Config validation updated: missing `dim`/`max_length` is no longer an error if they can be inferred
- Existing explicit overrides still work and take precedence
- Embedding validation compares Go output against Python reference for the same model and inputs, using cosine similarity

## Capabilities

### New Capabilities

- `model-autoconfig`: Auto-detect embedding dimension, max sequence length, and tokenizer files from ONNX model graphs and tokenizer configurations
- `embedding-validation`: Validate that Go embedding output matches reference Python (sentence-transformers) output within cosine similarity threshold

### Modified Capabilities

- `model-loading`: Config format changed to require only `onnx` path; `dim`, `max_length`, `pooling`, `normalize` become optional with auto-detected defaults

## Impact

Files: `internal/config/config.go`, `internal/registry/registry.go`, `internal/onnx/runtime.go`. The `ModelConfig` struct gets validation logic changes. ONNX model loading adds a metadata inspection step. New `verify-embeddings` target in `justfile`. No changes to the RESP protocol, worker pool, or embedding pipeline.
