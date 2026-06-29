## 1. Core Fix

- [x] 1.1 Rewrite `RuntimeSession.Run()` to create ORT tensors with exact `(batchSize, seqLen)` shape instead of reusing pre-allocated `(maxBatch, maxSeq)` tensors
- [x] 1.2 Remove pre-allocated tensor creation from `NewRuntimeSession()` (inputIDs, attnMask, tokenTypeID, output fields)
- [x] 1.3 Remove `maxBatch`, `maxSeq`, `hasTokenType` fields from `RuntimeSession` struct
- [x] 1.4 Add `defer tensor.Destroy()` for all per-call tensors to prevent memory leaks

## 2. Verification

- [x] 2.1 Build and verify `go vet ./...` passes
- [x] 2.2 Run `go test ./...` and confirm all existing tests pass
- [x] 2.3 Run server and confirm `EMB minilm test` completes in ~45ms (was ~2.9s)
