## Context

Batching has always been a per-model opt-in (`batching: {timeout: N}`); the worker pool was the fallback. Production measurements on siglip2 (283MB int8) and hyperclusters (1.1GB fp32) show the batcher is neutral-to-better even at single-client latency and 2–4× better under concurrency, with byte-identical outputs. Nothing justifies making users configure it.

## Goals / Non-Goals

**Goals:**
- Any model loaded by `emb` gets the performance path by default
- `timeout: 0` remains a clean, documented opt-out (worker pool)
- Existing configs that already set `timeout` are unaffected
- Bench harness baselines keep pool semantics

**Non-Goals:**
- Removing the worker pool (still the explicit escape hatch and the no-batching reference)
- Changing the token-budget / async-tokenization semantics (their defaults simply engage)

## Decisions

### `Timeout *int`: unset → enabled(1ms), 0 → disabled, >0 → explicit
The pointer distinguishes "user said nothing" (default ON) from "user said disable". Matches the existing `max_batch_tokens` / `tokenize_workers` pointer pattern in the same struct.

### 1ms default window
Matches the window used for the production siglip2/hyperclusters validation and the `config-batching.yaml` example; the +≤1ms single-request latency floor is negligible next to inference and is hidden by async tokenization anyway.

### Harness forces `timeout: 0` for pool-mode runs
The Fargate harness's `-batching-timeout 0` configs now emit an explicit `timeout: 0` so its pool baselines remain comparable to the committed P0 baseline (which predates default-on).

### Max defaults ride along
Because `max_batch` (32) / `max_batch_tokens` (16384) / `tokenize_workers` (min(4, cores)) default only when batching is active, default-on means a bare model config gets the full pipeline automatically.

## Risks / Trade-offs

- [Single-request latency adds ≤1ms] → insignificant vs inference; documented.
- [Users who intentionally disabled batching by omission] → behavior change is intended (default-on is the feature); `timeout: 0` is the explicit escape.
- [Pool benchmarking comparisons] → harness forces `timeout: 0`, keeping baselines honest.

## Migration Plan

1. `Timeout` → `*int`; registry defaults nil→1 and wires the resolved int.
2. Harness emits explicit `timeout: 0` when not batching.
3. Docs (README, spec) note default-on + opt-out.
4. `just test` + `just lint`; live-check a bare config shows `batching=1ms/32/16384`.

## Open Questions

- Should `max_batch` (32) be raised now that default-on is widespread? (Proposal: keep 32; the token budget already bounds runs.)