## Why

The `RuntimeSession.Run()` method reuses pre-allocated tensors with shape `(maxBatch, maxSeq)` = `(32, 256)` instead of creating tensors with the actual input shape `(batchSize, seqLen)`. This causes ONNX Runtime to process the full 32×256 = 8192 token buffer instead of the actual input (e.g., 1×6 tokens), resulting in ~2.9s per embedding request instead of ~1.3ms.

## What Changes

- Remove pre-allocated tensor reuse in `RuntimeSession.Run()`
- Always create ORT tensors with the correct `(batchSize, seqLen)` shape for each request
- Remove the obsolete `maxBatch`/`maxSeq` pre-allocation fields from `RuntimeSession`
- Verified performance: single EMB drops from 2.9s → ~2ms

## Capabilities

### New Capabilities

- None (bug fix within existing capability)

### Modified Capabilities

- `embedding-pipeline`: The ONNX inference step MUST create tensors with the exact input shape, not a fixed maximum. This changes the implementation detail but not the external contract.

## Impact

Only `internal/onnx/runtime.go` needs changes. No API or config changes. Tests that mock the session are unaffected.
