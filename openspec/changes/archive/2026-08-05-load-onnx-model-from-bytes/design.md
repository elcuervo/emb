## Context

`emb` runs as an ECS Fargate worker with ONNX model files on an NFS-mounted S3 Files filesystem (`/rails/vendor/dependencies`). The current `NewRuntimeSession(modelPath, ...)` calls `ort.NewDynamicAdvancedSession` which passes the path to the ONNX Runtime C API `CreateSession`. The C runtime uses `mmap()` for the file, so weight tensor pages are lazily fetched from NFS on access — including during inference, not just at load time. This causes unpredictable per-request latency spikes whenever NFS pages are evicted under memory pressure.

Additionally, `ensurePool` creates `numWorkers` sessions, each re-reading the model file independently. With N workers, the file is read N times from NFS.

The `onnxruntime_go` v1.31.0 binding (already in use) exposes `NewDynamicAdvancedSessionWithONNXData([]byte, ...)` which calls the C API `CreateSessionFromArray`, loading entirely from memory with no file dependency after construction.

## Goals / Non-Goals

**Goals:**
- Eliminate NFS file access during inference by loading model bytes into memory once
- Reduce session pool creation from N file reads to 1 file read + N in-memory constructions
- No change to public API, configuration format, or startup behavior

**Non-Goals:**
- Changing metadata introspection functions (`GetInputNames`, `GetOutputInfo`, `InferDim`) — they run once at setup and are acceptable as path-based
- Compressing or caching model bytes across restarts
- Changing how `autoTuneWorkers` estimates memory (still uses `os.Stat`)

## Decisions

**Read file once in `ensurePool`, before the session factory closure**

`ensurePool` already has access to `cfg.ONNX` (the file path) before creating the `sessionFactory` func. Reading there means:
- One `os.ReadFile` call regardless of `numWorkers`
- The `[]byte` slice is captured by the closure and shared across all worker sessions
- After pool creation, if the slice is the only reference, it can be GC'd once ORT has copied the bytes internally

Alternative considered: read in `NewRuntimeSessionFromBytes` per call — rejected, this reads N times.

**Add `NewRuntimeSessionFromBytes` as a sibling function, not a replacement**

`NewRuntimeSession(path)` stays for callers that don't have a mount (local dev, HuggingFace downloads). Only `registry.ensurePool` switches to the bytes variant. Keeps the surface minimal.

**No changes to `downloadModel` path**

HuggingFace-downloaded models land on local ephemeral storage, not NFS — the mmap problem doesn't apply there. `downloadModel` flow is unchanged.

## Risks / Trade-offs

[Peak memory spike at load] During `ensurePool`, memory holds both the `[]byte` slice and the ORT internal session graph simultaneously → peak ≈ 2× model size briefly.
→ Mitigation: ORT copies bytes into its allocator and returns. The `[]byte` is eligible for GC once `NewPool` returns. With typical int8 models (50–150MB), the spike is tolerable and bounded.

[`os.ReadFile` on NFS is synchronous] The single file read at startup blocks `ensurePool` until the full file is transferred.
→ Acceptable: this replaces N blocking mmap reads with 1 explicit read. Startup time is identical or better (sequential prefetch is fastest access pattern for NFS).

[`autoTuneWorkers` estimates by file size via `os.Stat`] Still correct — stat is a metadata call, not a data read.
→ No change needed.

## Migration Plan

Deploy is a standard binary rebuild. No config changes, no migration steps. Rollback: redeploy previous binary.
