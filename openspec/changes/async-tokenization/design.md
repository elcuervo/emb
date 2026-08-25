## Context

Today `processBatch` (pipeline.go:34) runs `tok.Encode` for every text in a window and then `sess.Run` — all inside the single worker goroutine. Tokenization and inference share one core: while tokenizing, the session (and any intra-op threads) idle. The tokenizer is already used concurrently across workers today (goroutine-safe), so a producer pool shares no new locking state. The P1 change introduces per-request real-token accounting, which a producer stage can feed directly.

## Goals / Non-Goals

**Goals:**
- Move tokenization to a bounded, concurrent producer stage feeding the run loop
- Preserve strict arrival-order results and existing error semantics
- Share the stage across both batcher and worker-pool paths
- Default `tokenize_workers = min(4, cores)`, disable with `0`

**Non-Goals:**
- Changing the tokenizer implementation or model configs
- Changing pooling/normalization/output semantics
- GPU/accelerator overlap (CPU-only Fargate target)

## Decisions

### Producer stage + single run loop
```
clients ──▶ Embed(texts) ──┐
                           ▼
                    [bounded queue]
                           │   tokenize_workers goroutines: text → Encoding (+ tokens)
                           ▼
                    [run loop]  packs → flush (budget/count/timer) → Run → distribute via Result chans
```
One run loop keeps batch packing and result ordering trivial (single writer). Tokenizer workers are pure producers; the run loop is the only consumer. This mirrors TEI's dedicated tokenization workers feeding a single dynamic batcher.

### Packing stays in the run loop
Encoding into `PadEncodings`-style buffers is cheap relative to tokenizing; keeping it in the run loop avoids an extra copy hop and simplifies the token budget flush (P1) that lives there.

### Backpressure through a bounded channel
The producer queue is bounded (cap derived from `max_batch` × a small multiplier). When full, `Embed` blocks the connection handler — the same backpressure behavior the worker pool has today (unbuffered `reqChan` send). No unbounded goroutine growth.

### Applies to both pool paths
The worker-pool (non-batching) path also funnels through the same producer stage so behavior is uniform; with `tokenize_workers: 0` both paths fall back to today's serial encode-in-worker behavior.

**Implemented outcome:** the shared producer stage was implemented for the **batcher path** only (the single-session bottleneck where overlap matters). The worker-pool path keeps its per-worker `encode → run` loop — its N workers already overlap tokenization with inference *across* workers, satisfying the spec requirement ("tokenization of a later request can overlap the run of an earlier batch") without a second stage. `tokenize_workers: 0` gives serial batcher behavior.

## Risks / Trade-offs

- [CPU contention between tokenizer workers and intra-op threads] → `tokenize_workers` defaults are conservative (`min(4, cores)`); `intra_op_threads = cores−2` already reserves cores for request handling, and the harness gate validates the net effect.
- [Fairness when a huge text occupies tokenizer workers] → bounded queue + arrival-order enforcement keeps visible behavior identical to today's per-connection ordering.
- [Lifecycle races on Close] → close the producer queue, let in-flight encodes finish, then drain the run loop; covered by the graceful-shutdown scenario.
- [Instrumentation overhead] → overlap metric is wall-clock sums only, sampled off the hot path.

## Migration Plan

1. Add `tokenize_workers` config (default `min(4, cores)`).
2. Introduce producer stage; switch batcher and pool `Embed` to feed it; keep `tokenize_workers: 0` = build the old serial mapping.
3. Wire lifecycle (`Close` drains), then run the P0 harness gates.
4. Document in config.yaml + BENCHMARK.md.

## Open Questions

- Is `min(4, cores)` the right default for models where tokenization is heavier than MiniLM's (e.g. multilingual tokenizers)? (Proposal: keep and tune after phase-gate data.)