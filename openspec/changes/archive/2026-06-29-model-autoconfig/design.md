## Context

Model configs require 5-6 fields per model (`onnx`, `tokenizer`, `pooling`, `normalize`, `max_length`, `dim`). Most are knowable from the ONNX graph and tokenizer files. The `dim` is literally in the output tensor shape. The `max_length` is in `config.json`'s `max_position_embeddings` or the tokenizer's `model_max_length`.

## Goals / Non-Goals

**Goals:**
- Minimal config: `models: { name: { onnx: ./path/model.onnx } }` works
- `dim` auto-detected from ONNX output shape
- `max_length` auto-detected from tokenizer or model config
- `tokenizer` path inferred from ONNX directory if not specified
- `pooling` defaults to `mean`, `normalize` defaults to `true`
- All explicit config values still work and take precedence
- Backward compatible

**Non-Goals:**
- Detecting pooling strategy from model type (models don't encode this)
- Auto-detecting ONNX input/output tensor names (vary by model family)
- Continuous integration validation (manual `just verify-embeddings` target)

## Decisions

### dim detection: ONNX metadata inspection

Use the `DynamicAdvancedSession.GetModelMetadata()` API or an ONNX reader to get the output tensor shape. The output `last_hidden_state` has shape `(batch, seq_len, dim)`. The third axis is `dim`.

For models with multiple outputs, use the first output's last dimension. If the output has the wrong number of dimensions (e.g., pooled output already), fall back to config.

### max_length detection: tokenizer config.json

The ONNX model directory typically contains a `config.json` with `max_position_embeddings`. The tokenizer file may have `model_max_length`. Priority:
1. Explicit `max_length` in config → use it
2. `config.json` `max_position_embeddings` → use it
3. Default to 512

### tokenizer path inference

If `tokenizer` is not specified but `onnx` is, look for `tokenizer.json` in the same directory as the ONNX file.

### Implementation approach

Detection happens in `registry.LoadModel()` after config parsing:
1. Parse YAML config (unchanged)
2. For each model, resolve defaults:
   - Infer tokenizer path from ONNX dir if not set
   - Read ONNX metadata for dim if not set
   - Read config.json for max_length if not set
   - Apply pooling/normalize defaults (mean, true)
3. Validate (tokenizer must exist, ONNX must exist)
4. Create workers (unchanged)

### Embedding validation flow

Validation ensures the auto-configured model produces embeddings that match Python's sentence-transformers:

1. **Generate reference**: Python script embeds 20 test sentences using `sentence-transformers` with `all-MiniLM-L6-v2`, saves raw float32 embeddings to JSON.
2. **Start server**: emb server starts with minimal config (auto-detected dim/max_length).
3. **Compare**: Go benchmark (`cmd/emb-verify`) reads the reference JSON, sends each sentence via RESP, reads the response, reshapes to float32, and computes cosine similarity.
4. **Report**: Pass/fail per sentence with threshold > 0.999.

### Implementation

The `verify-embeddings` just target:
- Runs the Python reference script if `reference-embeddings.json` doesn't exist
- Starts the emb server
- Runs `go run ./cmd/emb-verify` which compares against reference
- Stops the server

The `cmd/emb-verify` tool is a small Go program (like `cmd/emb-bench`) that connects to the server, sends EMB commands, and compares results.

## Risks / Trade-offs

- [ONNX metadata API requires loading the model file] → Only reads the model header, not the full weights. Takes ~10ms.
- [config.json might not exist] → Graceful fallback to default max_length=512.
- [Model has non-standard output names] → Only `last_hidden_state` is checked. If the model uses a different output name, explicit `dim` is required. Document this.
- [Token distance: auto-detected dim vs explicit dim mismatch] → Explicit always wins. If auto-detected dim doesn't match expected, log a warning.
- [Validation threshold too strict] → Float32 accumulation order between Python and Go produces tiny differences. Threshold of 0.999 is safe given identical tokenization and model weights.
