## 1. Extract shared process function

- [x] 1.1 Add `processBatch(session, tok, texts, dim, maxLen, normalize, pooling) (embeddings, tokens)` function to `pipeline.go`
- [x] 1.2 Update `Worker.process()` to delegate to `processBatch`
- [x] 1.3 Update `Batcher.process()` to delegate to `processBatch`
- [x] 1.4 Verify `go build` and `go test` pass

## 2. Add token and error tracking

- [x] 2.1 Add `tokens_processed atomic.Int64` and `errors atomic.Int64` fields to `Worker` struct
- [x] 2.2 Add `Tokens() int64` and `Errors() int64` methods to `Worker`
- [x] 2.3 Add same fields to `Batcher` struct
- [x] 2.4 Increment tokens after each encoding, increment errors on inferrence failures
- [x] 2.5 Verify `go build` and `go test` pass

## 3. Add memory tracking

- [x] 3.1 Add `currentMemoryUsage() uint64` to `sysmem_darwin.go` (uses runtime.ReadMemStats)
- [x] 3.2 Add `currentMemoryUsage() uint64` to `sysmem_linux.go` (uses runtime.ReadMemStats)
- [x] 3.3 Add `currentMemoryUsage() uint64` returning 0 to `sysmem_fallback.go`

## 4. Expand Stats struct

- [x] 4.1 Add `Tokens int64`, `Errors int64`, `MemoryMB int64`, `Pooling string`, `Normalize bool`, `MaxLen int`, `BatchingTimeout int`, `BatchingMaxBatch int` to pipeline `Stats` struct
- [x] 4.2 Update `Pool.Stats()` to populate new fields from worker/batcher and config
- [x] 4.3 Add `TotalErrors()` method to `Registry` that sums across models
- [x] 4.4 Verify `go build` and `go test` pass

## 5. Update server handlers

- [x] 5.1 Expand `handleSTATS`: add `active_requests`, `total_errors`, expanded per-model format
- [x] 5.2 Expand `handleINFO`: add `max_length`, `pooling`, `normalize`, `tokens`, `errors`, `batching_timeout`, `batching_max_batch`
- [x] 5.3 Remove `Server.total` atomic counter (replace with per-model sum)
- [x] 5.4 Verify `go build` and `go test` pass

## 6. Verify

- [x] 6.1 `go vet ./...` — passes
- [x] 6.2 `golangci-lint run ./...` — zero issues
- [x] 6.3 Manual: `EMB.STATS` shows new fields (uptime, requests, tokens, errors, models, per-model)
- [x] 6.4 Manual: `EMB.INFO <model>` shows all new fields (max_length, pooling, normalize, tokens, errors, batching)
- [x] 6.5 `just verify-embeddings` — 20/20 cosine=1.0 ✓
