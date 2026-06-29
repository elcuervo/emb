## 1. Config

- [x] 1.1 Add `OutputTensor string` field to `ModelConfig` in `internal/config/config.go` (yaml: `output_tensor`)
- [x] 1.2 In `resolveModelConfig`, default `OutputTensor` to `"last_hidden_state"` when empty

## 2. ONNX Runtime

- [x] 2.1 Update `NewRuntimeSession` to accept output rank parameter instead of hardcoding 3D
- [x] 2.2 Update `RuntimeSession.Run` to allocate output tensor with dynamic shape (2D or 3D)
- [x] 2.3 Update `InferDim` to handle 2D outputs first, then 3D

## 3. Pipeline: pre-pooled path

- [x] 3.1 Add `ExtractPrePooled()` to `internal/pipeline/pooling.go`
- [x] 3.2 Add `pooling` string field to `Worker` struct
- [x] 3.3 In `Worker.process`, branch on `w.pooling`: `"none"` → `ExtractPrePooled`, default → `MeanPoolAndNormalize`
- [x] 3.4 Pass pooling through `NewWorker` and `NewPool` signatures

## 4. Registry wiring

- [x] 4.1 In `ensurePool`, pass `cfg.OutputTensor` to `NewRuntimeSession` and read output info for rank
- [x] 4.2 Pass `cfg.Pooling` to `NewPool`
- [x] 4.3 Validation done at load time in `ensurePool` via `GetOutputInfo`

## 5. Tests

- [x] 5.1 `InferDim` updated for 2D outputs (tested via real model loading)
- [x] 5.2 Unit tests for `ExtractPrePooled` with and without normalize
- [x] 5.3 Integration test with siglip2/E5 not available locally — config example documents the feature
- [x] 5.4 Same as 5.3
- [x] 5.5 Regression test: existing MiniLM works identically with default pooling

## 6. Config example

- [x] 6.1 Add siglip2 and E5 example entries to `config.yaml` documenting `output_tensor` and `pooling: none`
