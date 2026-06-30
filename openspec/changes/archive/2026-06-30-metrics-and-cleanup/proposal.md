## Why

The current `EMB.STATS` and `EMB.INFO` commands expose minimal data: request counts, average latency, and uptime. Operators debugging production issues have no visibility into memory pressure, tokenization volume, error rates, or model configuration. Additionally, `Worker.process()` and `Batcher.process()` are identical — a duplicated inference pipeline that will diverge as features are added.

## What Changes

### Metrics expansion

**EMB.STATS** gains:
- `active_requests` — in-flight requests at the time of the call (uses the `WaitGroup` from graceful-shutdown)
- `total_errors` — sum of all model errors
- `memory_rss_mb` — process RSS memory on Linux/macOS
- `per_model` format expands: `"name: req=N avg=Xms tok=N err=N mem=Nmb pool=S norm=B batch=T/N"`

**EMB.INFO <model>** gains:
- `max_length`, `pooling`, `normalize`, `tokens_processed`, `errors`, `memory_mb`, `batching_timeout_ms`, `batching_max_batch`

### Code cleanup

- Extract shared inference pipeline from `Worker.process()` / `Batcher.process()` into a standalone `processBatch()` function
- Remove redundant server-level `total` counter (per-model stats already provide this)

## Capabilities

### New Capabilities
- (none — existing commands gain fields)

### Modified Capabilities
- (none — no spec-level behavior changes, only output enrichment)

## Impact

| File | Change |
|------|--------|
| `internal/pipeline/pipeline.go` | Add `processBatch()` shared function; expand `Stats` struct with new fields |
| `internal/pipeline/pool.go` | Delegate `process()` to shared function; expand stats output |
| `internal/pipeline/batcher.go` | Delegate `process()` to shared function; track tokens + errors |
| `internal/pipeline/batch.go` | (unchanged) |
| `internal/pipeline/pooling.go` | (unchanged) |
| `internal/server/server.go` | Expand `EMB.STATS` and `EMB.INFO` handlers with new fields; remove redundant `total` counter |
| `internal/registry/sysmem_darwin.go` | Add `currentMemoryUsage()` function |
| `internal/registry/sysmem_linux.go` | Add `currentMemoryUsage()` function |
| `internal/registry/sysmem_fallback.go` | Add `currentMemoryUsage()` returning 0 |
