## Context

The `RuntimeSession` in `internal/onnx/runtime.go` pre-allocates ORT tensors at session creation with shape `(maxBatch=32, maxSeq=256)`. On every `Run()`, the actual input data is copied into these fixed-size tensors. This causes ORT to process the full tensor shape `(32, 256)` instead of the actual input `(batchSize, seqLen)`, inflating inference time by 420x (1.3ms → 558ms). Combined with CGo marshaling overhead, each request takes ~2.9s instead of ~2ms.

## Goals / Non-Goals

**Goals:**
- Single EMB requests complete in < 10ms (currently ~2.9s)
- Batch EMB requests scale correctly with batch size
- Remove the pre-allocated tensor anti-pattern

**Non-Goals:**
- Re-introducing tensor pre-allocation correctly (future optimization)
- Changing the pipeline interface or worker pool architecture

## Decisions

### Always create ORT tensors with exact input shape

Replace the pre-allocation pattern with per-call tensor creation:

```
// Before (broken):
tensor, _ = ort.NewTensor(ort.NewShape(32, 256), make([]int64, 32*256))  // 32×256
// Reuse on every Run(), ORT processes 32×256 = 8192 tokens

// After:
tensor, _ = ort.NewTensor(ort.NewShape(batchSize, seqLen), data)  // exact shape
// Destroy after Run(), ORT processes batchSize×seqLen tokens
```

**Why not keep pre-allocated tensors with the correct shape?** The shape changes per request (different batch sizes, different sequence lengths after padding). We'd need to resize the tensor on shape change anyway, and the Go ORT wrapper requires creating a new `ort.Tensor` for new shapes (no in-place reshape API).

**Why not use a tensor pool keyed by (batch, seq)?** The allocation cost of `ort.NewTensor` is dominated by the 3 million float32s in the output tensor (12MB). For small inputs (seqLen < 32), this is negligible compared to inference time. A pool would add complexity for marginal gain.

### Remove maxBatch/maxSeq fields from RuntimeSession

Since tensors are always created with the exact shape, these pre-allocation tracking fields are dead code.

## Risks / Trade-offs

- Per-call tensor allocation adds GC pressure → The output tensor (12MB for max size) is immediately consumed and discarded. For small requests (< 100 tokens), it's < 1MB. GC on M1 handles this trivially.
- `ort.Tensor.Destroy()` must be called reliably → Use `defer tensor.Destroy()` after creation, which is already standard Go practice in the yalue ORT wrapper.
