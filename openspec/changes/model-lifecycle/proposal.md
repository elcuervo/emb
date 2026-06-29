## Why

The server loads all model pools eagerly at startup, consuming memory for models that may never be queried. Worker count is fixed to CPU count regardless of model size or available RAM, risking OOM with large models. Request counters and latency are tracked internally but never exposed, making it impossible to monitor server health.

## What Changes

- Models loaded lazily by default: pool created on first `EMB <model>` request, not at startup
- Models with `preload: true` in config retain eager startup loading
- Worker pool size auto-tuned based on available RAM and model file size, capped at GOMAXPROCS
- Optional `workers: N` config field to override auto-tune
- Cumulative request count and average latency exposed per model via `EMB.INFO`
- `EMB.INFO` reports real stats (was hardcoded to 0)
- `EMB.STATS` reports per-model request totals (was per-server only)

## Capabilities

### New Capabilities

- `model-lifecycle`: Lazy model loading, auto-tuned worker pools based on available RAM, configurable `preload` and `workers` fields

### Modified Capabilities

- `model-loading`: Config format extended with optional `preload` (bool, default false) and `workers` (int, default 0 = auto-tune). Models with `preload: false` are loaded on first request instead of startup.
- `emb-cmds`: `EMB.INFO <model>` MUST return real request count and average latency from internal counters. `EMB.STATS` MUST return per-model breakdown.

## Impact

Files: `internal/config/config.go`, `internal/registry/registry.go`, `internal/pipeline/pool.go`, `internal/pipeline/pipeline.go`, `internal/server/server.go`, `cmd/emb/main.go`. No changes to the RESP embedding format or inference pipeline. Backward compatible — existing configs without `preload`/`workers` default to lazy loading and auto-tune.
