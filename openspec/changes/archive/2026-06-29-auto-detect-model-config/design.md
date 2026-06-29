## Context

The existing `model-autoconfig` change detects `dim` and `max_length` from the ONNX graph, but `output_tensor`, `pooling`, and `normalize` still need manual config. Every model registration requires the user to know the right output tensor name and whether it's pre-pooled — information the ONNX graph already provides. Siglip2 uses `pooler_output` (rank 2), MiniLM uses `last_hidden_state` (rank 3), and the pooling strategy is determined by that rank.

The ONNX `GetOutputInfo` function already returns the rank and dimension of every output tensor. We can use that to auto-select the right tensor and pooling strategy.

## Goals / Non-Goals

**Goals:**
- Auto-detect `output_tensor` from available ONNX outputs: prefer rank-2 → fall back to rank-3
- Auto-detect `pooling`: rank-2 output → `none` (pre-pooled), rank-3 output → `mean`
- Auto-detect `normalize` based on model family heuristics (sentence-transformers → true, others → false)
- All explicit config values (`output_tensor`, `pooling`, `normalize`) still take precedence
- Log the auto-detected values at model registration time for visibility
- Backward compatible — existing configs with explicit values continue to work unchanged

**Non-Goals:**
- Changing the ONNX tensor selection at inference time (selected once at pool creation)
- Dynamic switching of pooling strategy per-request
- Detecting custom pooling strategies beyond mean (e.g., max, weighted, CLS)
- Inference-time normalization override per-EMB call

## Decisions

### Output tensor selection: rank-first with name fallback

```
GetOutputInfo(modelPath) → {tensor_name: {Rank, Dim}, ...}

If multiple outputs:
  1. Prefer rank-2 outputs over rank-3 (already pooled)
  2. Among same-rank: prefer by name order: sentence_embedding > pooler_output > last_hidden_state
  3. If user set explicit output_tensor, use it (no auto-detect)

If single output:
  Use it directly regardless of rank
```

This handles all common model patterns:
- **Sentence-transformers (3D)**: single `last_hidden_state` output → rank 3 → mean pool
- **SigLIP/E5 (2D)**: `pooler_output` or `sentence_embedding` → rank 2 → no pooling
- **Multi-output models**: some have both rank-2 and rank-3 outputs; rank-2 is preferred

### Pooling inference from rank

```
if output.Rank == 2:
    pooling = "none"
elif output.Rank == 3:
    pooling = "mean"
```

This is a direct consequence of the tensor shape:
- Rank 2: `(batch, dim)` — already pooled, no mean pooling needed
- Rank 3: `(batch, seq_len, dim)` — sequence level, needs mean pooling across seq_len

The existing `MeanPoolAndNormalize` vs `ExtractPrePooled` switch in `pool.go` already implements this distinction — we just need to make the selection automatic.

### Normalization heuristics

```
if cfg.Normalize is explicitly set:
    use it
else:
    // Heuristic based on model type
    if models with "sentence-transformers" in model metadata or repo:
        normalize = true
    else:
        normalize = true  // safe default for embedding models
```

The safe default for embedding models is `normalize: true` — cosine similarity is the most common use case, and normalized vectors don't break dot-product similarity. Users who need raw unnormalized outputs can still set `normalize: false` explicitly.

### Implementation in resolveModelConfig

The detection runs in `resolveModelConfig` which is already the place where dim and max_length are auto-detected. The new block:

```go
if cfg.OutputTensor == "" || cfg.Pooling == "" || cfg.Normalize was not set {
    outInfo, err := onnx.GetOutputInfo(cfg.ONNX)
    if err == nil {
        selected := selectOutput(outInfo, cfg.OutputTensor)
        if cfg.OutputTensor == "" {
            cfg.OutputTensor = selected.Name
        }
        if cfg.Pooling == "" {
            cfg.Pooling = poolingForRank(selected.Rank)
        }
    }
}
```

`cfg.Normalize` being a `bool` has no "unset" sentinel. Options:
- **A)** Change to `*bool` pointer (breaking JSON/YAML)
- **B)** Use a sentinel wrapper type
- **C)** Always default to `true` (current behavior, document it)

C is simplest and already matches current behavior. If a model truly needs `normalize: false`, the user sets it explicitly.

### Logging

```
log.Printf("  %s: auto-detected output=%q pooling=%s normalize=%v",
    name, cfg.OutputTensor, cfg.Pooling, cfg.Normalize)
```

This makes the auto-detection visible in server logs, so users can verify the choices without digging into code.

## Risks / Trade-offs

- [Wrong output tensor selected] — If a model has both a rank-2 and rank-3 output and rank-2 is not the correct one (e.g., it's an auxiliary loss output), the auto-detect picks the wrong tensor. Mitigation: users can always set `output_tensor` explicitly.
- [Pooling=none when mean pooling is actually needed] — If a rank-2 output is present but is a logit or classification head (not pooled embeddings), the embeddings will be meaningless. Mitigation: explicit override.
- [Normalize=true for raw LLM embeddings] — LLMs produce unnormalized embeddings by default. Setting normalize=true is harmless (always valid math) but changes similarity semantics. Mitigation: explicit override.
