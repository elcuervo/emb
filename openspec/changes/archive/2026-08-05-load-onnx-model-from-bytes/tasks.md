## 1. Add bytes-based session constructor

- [x] 1.1 Add `NewRuntimeSessionFromBytes(data []byte, inputNames, outputNames []string, dim, outputRank, intraOpThreads, interOpThreads int) (*RuntimeSession, error)` to `internal/onnx/runtime.go` using `ort.NewDynamicAdvancedSessionWithONNXData` with identical session options to `NewRuntimeSession`

## 2. Switch registry to read-once pattern

- [x] 2.1 In `registry.ensurePool()`, after resolving `cfg.ONNX` path, read the file: `modelData, err := os.ReadFile(cfg.ONNX)` and return error on failure
- [x] 2.2 Update the `sessionFactory` closure to call `onnx.NewRuntimeSessionFromBytes(modelData, inputNames, []string{cfg.OutputTensor}, cfg.Dim, out.Rank, cfg.IntraOpThreads, cfg.InterOpThreads)` instead of `onnx.NewRuntimeSession(cfg.ONNX, ...)`

## 3. Verify

- [x] 3.1 Run existing benchmarks (`just bench` or equivalent) against a model on NFS/slow storage to confirm inference latency improves — DEFERRED: no NFS mount in dev environment; run in NFS deployment before/after deploy
- [x] 3.2 Confirm `go build ./...` passes with no errors
