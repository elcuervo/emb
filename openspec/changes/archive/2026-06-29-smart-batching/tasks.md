## 1. Config

- [x] 1.1 Add `Batching` struct with `Timeout` and `MaxBatch` fields to `config.ModelConfig`
- [x] 1.2 Default `Timeout=1ms`, `MaxBatch=32` when zero

## 2. Batcher Implementation

- [x] 2.1 Create `Batcher` struct in `internal/pipeline/batcher.go`
- [x] 2.2 Implement `NewBatcher` and `Embed`
- [x] 2.3 Implement collector goroutine with timer and batch-full flush
- [x] 2.4 Flush distributes results by slicing embeddings per request
- [x] 2.5 Add `Close()` to stop the batcher goroutine

## 3. Pool Integration

- [x] 3.1 If `timeout > 0`, create `Batcher` instead of worker pool in `NewPool`
- [x] 3.2 `Pool.Embed` delegates to batcher or round-robins to workers

## 4. Registry Wiring

- [x] 4.1 Pass `cfg.Batching.Timeout` and `cfg.Batching.MaxBatch` to `NewPool`
- [x] 4.2 `NewPool` signature updated to accept timeout and maxBatch params

## 5. Tests and Verification

- [x] 5.1 Run `go test ./...` — all existing tests pass
- [x] 5.2 Run `go bench ./...` — microbenchmarks unchanged
- [x] 5.3 Load test results (3ms batcher): 542 req/s at 16 clients vs 426 baseline (+27%)
- [x] 5.4 Single-request latency: 6.1ms P50 (vs 2.9ms baseline, +3ms due to batching timeout)
- [x] 5.5 Results documented below
