## Why

The smart batcher caps batches by **request count** (`max_batch: 32`) and pads every batch to its longest sequence (`PadEncodings`). With mixed-length traffic — the norm for a real API — that wastes most of the compute: one 512-token text plus seven 8-token texts burns `8×512 = 4096` token-slots to deliver ~570 real tokens. On ARM64/Graviton, where MLAS GEMMs are efficiency-sensitive per element, that waste is the single biggest ranking cost. TEI solves this with a token budget (`max-batch-tokens`), bounding waste and making batch memory predictable.

## What Changes

- New per-model config `batching.max_batch_tokens` (default `16384`, TEI's default; `0` disables token-budget accounting and keeps pure count-based behavior).
- The batcher flushes when accumulated **real tokens** reach the budget **or** request count reaches `max_batch` **or** the timeout fires — budget is the cap, count is the secondary bound (TEI's `max-batch-requests` analog), timeout unchanged.
- Real token totals come from the already-tracked per-request tokenization (`Stats.Tokens`); no new tokenizer pass.
- A single request whose own token count exceeds the budget is processed as one run (never split a semantic batch) — matching TEI's separate `max-client-batch-size` concern.
- Expose `batching_max_tokens` in `EMB.INFO` and `EMB.STATS`, plus a new **padding-efficiency** stat (real tokens / processed token-slots) so the harness's mixed-length metric is server-visible.
- No RESP protocol change. No change to the worker-pool (non-batching) path.

## Capabilities

### New Capabilities

- `token-budget-batching`: limit batch size by accumulated real tokens instead of request count, bounding padding waste and batch memory.

### Modified Capabilities

- `smart-batching`: the existing batching window gains token-budget accounting (`max_batch_tokens`) as the primary flush bound.

## Impact

Files: `internal/pipeline/batcher.go` (flush logic + token accounting), `internal/config/config.go` (+`MaxBatchTokens`), `internal/registry/registry.go` (wiring), `internal/server/server.go` (`EMB.INFO`/`EMB.STATS` fields), `config-batching.yaml` example, BENCHMARK.md. No protocol or pool-path changes.

## Validation

All inside `nix develop`, against the P0 golden baseline (linux/arm64, Fargate-shaped):

```
$ nix develop
$ just build && just bench-fargate-diff <baseline> <after>
```

- **Mixed-length gate:** at 8 vCPU, mixed-length workload (80% ~8-token, 20% ~512-token texts): req/s ≥ **+30%** vs baseline; server-reported padding efficiency ≥ 0.5 (baseline ~0.2).
- **Fixed-length gate:** fixed-length workloads: no regression > **−5%** req/s.
- **Latency gate:** p50 at 2 vCPU single client ≤ baseline p50 + `max_batch_tokens` budget effect (≤ +2ms) and p99 within baseline tolerance.
- **Correctness gate:** `just verify-embeddings` passes unchanged (identical embeddings to fp32 reference).
- Default config stays backward-compatible: configs without `max_batch_tokens` behave exactly as today.