## 1. Session borrow semantics

- [x] 1.1 Reuse the output tensor across runs in `RuntimeSession.Run` (replaces per-run `NewEmptyTensor` + `Destroy`); input tensors stay per-run (their cost is small vs the output copy)
- [x] 1.2 Return `outTensor.GetData()` directly — no `make`+`copy` of the output. Aliasing confirmed empirically: batcher/worker sessions serialize their runs, and pooling consumes the slice synchronously before the next `Run`
- [x] 1.3 Legacy copy path removed outright (no flag needed; verified byte-identical below)

## 2. Pipeline & server handoff

- [x] 2.1 Pooling/normalization reads the borrowed slice synchronously inside `runEncodings`; no release plumbing needed since it never outlives the call
- [x] 2.2 No RESP-side borrowing: embeddings are `[][]byte` copies produced by pooling before the reply is built (batcher + pool + cache paths all covered by existing tests)
- [x] 2.3 Buffer liveness guaranteed structurally (single session owner; next `Run` overwrites after pooling reads)

## 3. Benchmarks & tests

- [x] 3.1 Added `BenchmarkRuntimeSessionRun` (real ONNX session, benchmem) covering 1×512, 32×512, 1×16
- [x] 3.2 Existing correctness tests green (byte-identical results)

## 4. Validation stage (nix develop)

- [x] 4.1 In `nix develop`: `go test ./...` + `just lint` pass
- [x] 4.2 Alloc gate (measured via `go test -bench`): batch=1×512 4.5KB B/op vs ~1.5MB before (−99.7%); 32×512 removes the ~50MB per-run output alloc; 1×16 592B
- [ ] 4.3 Throughput gate: req/s ≥ +10% at 16 clients / 8 vCPU vs P0 baseline (pending gold-host A/B, as P1/P2)
- [ ] 4.4 Tail gate: p99 at 16 clients / 2 vCPU ≤ baseline p99 (pending gold-host)
- [x] 4.5 `just verify-embeddings` passes (20/20, cosine 1.0)
- [x] 4.6 Legacy copy path removed (4.6 covered by 1.3/4.5)