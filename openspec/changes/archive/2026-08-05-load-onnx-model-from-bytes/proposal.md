## Why

When ONNX model files are loaded from an NFS-backed filesystem (AWS S3 Files), ONNX Runtime's `CreateSession` C API uses `mmap()` internally, meaning weight tensor pages are fetched lazily from NFS on each inference call. This causes persistent inference latency penalties beyond the initial load, not just at startup.

## What Changes

- Add `NewRuntimeSessionFromBytes(data []byte, ...)` to `internal/onnx/runtime.go` using `ort.NewDynamicAdvancedSessionWithONNXData`
- In `registry.ensurePool()`, read the model file into memory once (`os.ReadFile`) before the session factory loop, then pass bytes to all worker sessions instead of a path
- Each worker session in the pool is created from the same in-memory bytes — no repeated file reads

## Capabilities

### New Capabilities
- `onnx-bytes-session`: Load ONNX Runtime sessions from pre-read byte slices instead of file paths, eliminating NFS file access during session creation and inference

### Modified Capabilities

## Impact

- `internal/onnx/runtime.go`: new function alongside existing `NewRuntimeSession`
- `internal/registry/registry.go`: `ensurePool` reads file once, sessionFactory uses bytes variant
- Memory: peak load +1× model size (temporary, freed after sessions created); steady-state unchanged since weights were already resident for active inference
- `autoTuneWorkers` uses `os.Stat` for size estimation — unchanged, path still available
- Metadata introspection (`GetInputNames`, `GetOutputInfo`, `InferDim`) still uses path-based APIs — called once at setup, not during inference, acceptable
