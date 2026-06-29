## Context

`emb` currently hardcodes `last_hidden_state` as the ONNX output tensor name and always applies mean pooling over the sequence dimension. This works for standard sentence-transformer models (e.g., all-MiniLM-L6-v2) that expose token-level hidden states.

However, many production models expose pre-pooled outputs:
- **siglip2** (`text_model_int8.onnx`): outputs `pooler_output` — a 2D tensor (batch × 768) from a learned pooling projection on the [CLS] token
- **E5 hyperclusters** (`model.onnx`): outputs `pooled_sentence_embeddings_debiased_normalized` — a 2D tensor (batch × 768) already L2-normalized and debiased

Requesting `last_hidden_state` from these models either fails at session creation or produces wrong embeddings (mean-pooling raw hidden states ≠ model's intended output).

The `Pooling` config field already exists but is silently ignored — `resolveModelConfig` defaults it to `"mean"` but nothing downstream reads it.

## Goals / Non-Goals

**Goals:**
- Make output tensor name configurable per model (`output_tensor` in YAML)
- Support `pooling: none` for 2D pre-pooled outputs (skip mean pool, optional L2 normalize)
- Keep backward compatibility — models without these fields behave identically
- `InferDim` works for both 2D and 3D output tensors

**Non-Goals:**
- CLS pooling strategy (pooler_output is already CLS-derived inside the ONNX graph — no need to implement it externally)
- Configurable input tensor names (token_type_ids detection already handles optional inputs)
- Multi-output models

## Decisions

### Output tensor: config field, no auto-detection

**Decision**: Add `OutputTensor string` to `ModelConfig`. Default empty → use `"last_hidden_state"`. Explicit value overrides.

**Why**: Auto-detecting the right output from ONNX metadata is ambiguous — models often expose multiple outputs. Explicit config is clearer and safer.

**Alternative considered**: Inspect ONNX graph for 2D outputs and prefer them. Rejected — too magic, breaks for models that expose both.

### Pooling dispatch in Worker, not Pool

**Decision**: Pass `pooling` string to `Worker`. In `process()`, branch on pooling strategy after ONNX run.

```
pooling: "mean" (default) → existing MeanPoolAndNormalize path (3D output expected)
pooling: "none"           → new ExtractPrePooled path (2D output, optional normalize)
```

**Why**: Worker already owns the inference → pooling logic. Pool is just a round-robin router — no reason to put strategy there.

### RuntimeSession output shape: dynamic

**Decision**: `RuntimeSession` allocates output tensor dynamically based on actual output shape from `GetInputOutputInfo`. No hardcoded shape assumption.

**Current problem**: `outputTensor` is pre-allocated as `[batch, seqLen, dim]` (3D). For 2D outputs this is wrong size.

**Fix**: Read output shape from ONNX metadata at session creation; allocate accordingly. `Run()` returns flat `[]float32` — caller knows shape from config (pooling strategy).

### InferDim: check 2D outputs first

**Decision**: `InferDim` looks for outputs with 2 or 3 dimensions. 2D (batch × dim) → `dim = Dimensions[1]`. 3D (batch × seq × dim) → `dim = Dimensions[2]`. First match wins.

## Risks / Trade-offs

- **Wrong tensor name in config** → ONNX session creation fails at startup with a clear error. Acceptable — fail fast.
- **pooling: none + 3D output** → first token slice used, silent wrong result. Mitigation: validate output rank matches pooling strategy on load.
- **Token type IDs**: siglip2 may not have `token_type_ids` input. Already handled by `hasTokenType` detection in `NewRuntimeSession` — no change needed.

## Open Questions

- Should `output_tensor` default be `"last_hidden_state"` or auto-detect from ONNX graph? Current decision: hardcoded default for predictability.
- Should we validate output tensor rank vs pooling strategy at model load time? Recommend yes — add to `resolveModelConfig`.
