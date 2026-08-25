## Why

Batching is currently opt-in: a model only gets the performance path (windowed runs, token budget, async tokenization) when its config sets `batching.timeout`. A bare `models:` entry loads the worker pool, so most deployments — including production — silently miss the 1.5–4× concurrency and latency wins measured on the siglip2/hyperclusters models. There is no reason the fast path can't be the default: measured deltas show batching is neutral-to-better even at single-client latency (async tokenization hides the window), and outputs are byte-identical (mask-aware pooling).

## What Changes

- `batching.timeout` becomes optional-with-default: **unset → enabled with a 1ms window**, `0` → disabled (worker pool), `>0` → explicit window. Backward compatible for existing configs that already set `timeout`; the only behavior change is for configs that never mention batching.
- With batching on by default, `max_batch` (32), `max_batch_tokens` (16384), and `tokenize_workers` (min(4, cores)) defaults apply automatically — so **every model loaded by `emb` gets great performance with zero config**.
- The benchmark harness forces `timeout: 0` for its pool-mode baselines so the P0 baseline methodology is preserved.
- Docs: README/config comments note the new default and the `timeout: 0` escape hatch.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `smart-batching`: batching is enabled by default (1ms window); `timeout: 0` disables it.

## Impact

Files: `internal/config/config.go` (`Timeout *int`), `internal/registry/registry.go` (defaulting/wiring), `bench/fargate/main.go` (explicit `timeout: 0`), README, `openspec/specs/smart-batching`. No protocol changes; embeddings remain byte-identical.

## Validation

- `just test` + `just lint` pass.
- A plain config (only `onnx`/`tokenizer`, no batching) loads with `batching=1ms/32/16384` in the log and `batching_timeout_ms=1` in `EMB.INFO`.
- `batching: timeout: 0` loads with the worker pool (no batcher line).
- Live production config (siglip2 + hyperclusters) still byte-identical and shows the perf wins already measured.