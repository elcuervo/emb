## Context

The server currently loads all model pools eagerly at startup, creates `GOMAXPROCS` workers per model regardless of model size or available RAM, and tracks request/latency stats internally without exposing them. This works for small models on developer machines but doesn't scale to large models or memory-constrained environments.

## Goals / Non-Goals

**Goals:**
- Lazy model loading by default (first request triggers pool creation)
- `preload: true` config for eager loading at startup
- Worker pool size auto-tuned to `min(GOMAXPROCS, available_memory / 2 / (model_size * 1.2))`
- Configurable `workers: N` override for manual tuning
- Real request count and average latency returned by `EMB.INFO`
- Per-model stats in `EMB.STATS`
- Backward compatible (existing configs work unchanged)

**Non-Goals:**
- Sliding window / P99 latency histograms (cumulative average only)
- Downscaling workers after load drops
- Model unloading after idle periods
- GPUs or remote execution providers

## Decisions

### Lazy loading via sync.Once

`ModelEntry` wraps pool creation in a `sync.Once`:

```
type ModelEntry struct {
    Pool   *pipeline.Pool      // nil until loaded
    once   sync.Once           // ensures single initialization
    cfg    config.ModelConfig  // stored for lazy init
    name   string
    initMu sync.Mutex          // serialize first load errors
}
```

`Registry.GetOrInit(name)` calls `entry.once.Do(...)` which creates the pool. If initialization fails, the error is stored and the `sync.Once` is reset (via a flag) so the next attempt retries.

### Memory detection

Platform-specific:
- **Darwin**: `unix.SysctlUint64("hw.memsize")` via `golang.org/x/sys/unix`
- **Linux**: `/proc/meminfo` or `unix.Sysinfo(&info)` returning `TotalRam`
- (Adds `golang.org/x/sys` dependency for portable syscall access)

Fallback: if memory detection fails, use `GOMAXPROCS` workers (current behavior).

### Auto-tune formula

```
workers = min(GOMAXPROCS, int(total_ram * 0.5 / (model_file_size * 1.2)))
workers = max(workers, 1) // always at least 1
```

Where:
- `total_ram` is total physical RAM (not free — lazy loading means other models haven't allocated yet)
- `0.5` means use at most half of physical RAM for all workers
- `1.2` is a 20% overhead factor for activation memory per session

### Cumulative stats wiring

`Worker` already tracks:
- `requests atomic.Int64` (increments per request)
- `totalLat atomic.Int64` (accumulates µs per request)

Two new methods on `Pool`:
- `Requests() int64` — sums across all workers
- `AvgLatency() float64` — totalLat / requests (returns 0 if no requests)

`EMB.INFO` reads these through `ModelEntry.Pool`.

## Risks / Trade-offs

- [First request latency with lazy loading] → Pool creation takes ~1-2 seconds (ONNX session + tokenizer). Add a log line at start of loading so the user knows why the first request is slow.
- [Memory detection depends on OS support] → Fallback to GOMAXPROCS for unknown platforms. gopkg.in/x/sys covers Darwin and Linux.
- [sync.Once with retry on error] → If pool creation fails, the once must be reset. Use an internal `sync.Mutex` pattern: first caller tries, stores error, subsequent callers see the error and retry.
- [Cumulative average is noisy] → A single slow request (e.g., GC pause) skews the average permanently. Acceptable for an MVP — the absolute request count provides context for the latency number.
