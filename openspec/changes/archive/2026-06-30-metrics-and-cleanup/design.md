## Context

The inference pipeline (`encode → pad → run → pool/normalize → return`) is duplicated between `Worker.process()` and `Batcher.process()` — 25 lines of identical code with different field references. Adding features requires touching both. Meanwhile, operators need visibility into memory usage, error rates, and configuration at runtime.

## Goals / Non-Goals

**Goals:**
- Eliminate the `Worker.process()` / `Batcher.process()` duplication
- Expose memory RSS, token counts, and error counts via stats commands
- Expand `EMB.INFO` to show model configuration (pooling, normalize, batching)
- Remove the redundant `atomic.Int64 total` on Server (duplicated by per-model stats)
- Keep RESP response format and wire protocol unchanged

**Non-Goals:**
- Changing the `Tokenizer` or `Session` interfaces
- Adding new RESP commands (all changes are to existing commands)
- Performance optimization (cleanup should not regress)

## Decisions

### Shared process function: pipeline.processBatch

Extract the common inference chain into a package-level function in pipeline.go:

```go
func processBatch(
    session onnx.Session, tok tokenizer.Tokenizer,
    texts []string, dim, maxLen int,
    normalize bool, pooling string,
) ([]byte, error, int)  // returns embeddings, err, totalTokenCount
```

Both `Worker.process()` and `Batcher.process()` call this, removing the 25-line duplication.

```diff
- Worker.process (25 lines)
- Batcher.process (25 lines)
+ processBatch (25 lines, once)
+ Worker.process (3 line wrapper)
+ Batcher.process (3 line wrapper)
```

### Stats expansion additive, not breaking

New fields are appended to the existing RESP array format. Old clients that read by position (e.g., `emb.INFO[0]` = dim, `emb.INFO[1]` = workers) keep working since new fields are at the end.

```
Before:   dim, workers, requests, avg_latency_us  (4 fields)
After:    dim, workers, requests, avg_latency_us, max_length, pooling,
          normalize, tokens, errors, memory_mb, batching_timeout,
          batching_max_batch                       (12 fields)
```

### Memory tracking: system + per-model

`currentMemoryUsage()` reads RSS from the kernel:
- **Linux**: `/proc/self/status` → `VmRSS`
- **macOS**: `mach_task_basic_info` via `libc` / `unix.ProcTaskInfo`

Per-model memory = model file size × active sessions (workers). This is an approximation but gives operators enough signal to detect leaks or misconfiguration.

### Remove redundant total counter

The `Server.total` (`atomic.Int64` incremented in each handler) duplicates per-model stats. Replace by summing `reg.StatsPerModel()` at query time. This is negligible overhead for an admin command.

## Risks / Trade-offs

- [RSS reading fails on some platforms] → fallback returns 0, stats handler omits the field
- [New INFO fields after existing ones shift indices] → Old clients reading by index keep working (new fields are appended). Clients reading by key (tagged RESP) are unaffected.
- [Summing stats at query time is slightly slower than cached counter] → Admin commands called infrequently. Negligible.
