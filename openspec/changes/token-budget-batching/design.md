## Context

`Batcher.run` (batcher.go) accumulates `Request`s and flushes when the count reaches `max_batch` or the `timeout` fires. `processBatch` (pipeline.go) tokenizes the whole window, pads to the longest sequence (`PadEncodings`), then runs ONNX. On mixed-length traffic this pads short texts up to the longest text in each window, wasting a large fraction of GEMM work. The target is ARM64/Graviton CPU serving, where per-element GEMM efficiency matters, so bounding padding waste is the primary lever. `Stats.Tokens` already accumulates real tokens per window, so token accounting needs no extra tokenizer passes.

## Goals / Non-Goals

**Goals:**
- Flush batches on an accumulated real-token budget (`max_batch_tokens`, default 16384)
- Keep `max_batch` count and `timeout` semantics as secondary bounds
- Never split a single `EMB` command's texts across runs
- Backward compatible: no `max_batch_tokens` config → today's behavior
- Expose budget + padding efficiency via `EMB.INFO`/`EMB.STATS`

**Non-Goals:**
- Splitting or reordering texts within one command
- Adaptive, load-based budget sizing
- Changing the worker-pool (non-batching) path
- Cross-model batching

## Decisions

### Tokens are counted at enqueue time, not at flush time
Each request records its real token count when it is added to the window (available from tokenization). The batcher accumulates `sum(tokens)` and flushes on budget. This avoids re-measuring at flush and keeps the timeout window intact.

### Budget is the primary bound; count/timer are secondary
Flush triggers evaluated in order: budget ≥ threshold **or** count ≥ max_batch **or** timer fired. Matches TEI, whose `max-batch-tokens` is the dominant control with `max-batch-requests` secondary.

### Default 16384, matching TEI
For `all-MiniLM-L6-v2` (512 max tokens), that's ~32 full-length texts or many more short ones — a safe bound that keeps a single window within a small memory footprint while capturing batching efficiency. Endpoints that want pure count behavior set `0` (or rely on the default only where sensible).

### Oversized single commands run whole
TEI separates `max-client-batch-size` (per-request) from the global token budget. The analog here: a single `EMB` argument list is a semantic unit and is never split; the token budget applies to the aggregation of requests. Documented in config comments.

### Padding efficiency as a server stat
`padding_efficiency = Σ real tokens / (Σ batch_size × padded_seq_len)` is computed per run in the batcher and surfaced in stats. This makes the harness metric server-authoritative and debuggable per model.

## Risks / Trade-offs

- [Default budget too large for tiny models] → budgets are per-model configurable; defaults documented per typical model size.
- [Token counting overhead per enqueue] → O(1) per request (sum already tracked); no re-encode.
- [Behavioral surprise for existing mixed configs] → `max_batch_tokens` defaults to 16384 when `batching` is enabled in the new schema; documented in config.yaml and BENCHMARK.md; users pin `0` for old exact behavior.
- [Long-text flood can still produce a big single run] → bounded by `maxLen` × request size; single-command splitting explicitly out of scope (semantic unit).

## Migration Plan

1. Add `MaxBatchTokens` to config with default; keep `Timeout`/`MaxBatch` semantics.
2. Extend `Batcher` to accumulate tokens and flush on budget; leave pool path untouched.
3. Wire config → registry → pool; add stats fields.
4. Land behind the P0 harness gate (mixed-length ≥ +30%, fixed-length ≥ −5%, p50 ≤ +2ms) on linux/arm64.
5. Update `config-batching.yaml` example + README + BENCHMARK.md.

## Open Questions

- Should the default budget be per-model (max_len-dependent) instead of a flat 16384? (Proposal: keep flat default; allow override.)

## Validation finding (during apply, tasks 4.2-4.4)

A/B on the harness (2 vCPU, n=300, count-only vs budget=16384/1024) showed the
numeric gates in tasks 4.2-4.4 are **not measurable at that scale**:

1. Run-to-run noise on a shared 2-vCPU container reaches ±10-37%, swamping a
   single-run A/B diff; gates need a paired/interleaved A/B at n≥2000.
2. The harness-derived padding efficiency (0.24) is workload-defined (80/20
   short/long in max_batch=32 windows) and is NOT changed by the budget. The
   meaningful signal is the **server-reported** `padding_efficiency` (added by
   this change to EMB.INFO) on workloads where the budget actually binds.
3. On 2 vCPU the single-session batcher (intra-op floor=1) is ~30% slower than
   the worker pool at 8 clients — batching is recommended on ≥4 vCPU tiers.

Tasks 4.2-4.4 therefore remain pending until measured with a paired A/B on the
4/8 vCPU tiers (and preferably on the gold ARM64 Linux reference).

**Correction (later measurement on the same host):** a clean sequential A/B of
the four configs showed the single-session batcher strongly winning under
concurrency (8-client req/s 116→1048 fp32, →1397 with int8) and short-txt p50
11.0ms→2.7ms full-config. The earlier "batcher ~30% slower than the pool on
2 vCPU" reading was an artifact of n=300 run-to-run noise, not a real regression.

## Related work (captured during apply)

Learnings from michaelfeil/infinity that inform this change and the roadmap:

- Infinity's `--engine torch|optimum|ctranslate2` shows the engine is a first-class,
  per-deployment knob; `ctranslate2` supports only BERT models and its CPU docker
  image defaults to `optimum` (ONNX). This validates the `backend: auto|onnx|ctranslate2`
  design in ctranslate2-backend and its BERT-only fallback.
- Infinity reports experimental int8 on cpu/cuda (and fp8 on H100/MI300), matching the
  int8-weight-quantization direction. Its CLI exposes precision as a top-level
  `--dtype` knob — emb's per-model `quantize` config covers the same concern.
- "Dynamic batching and tokenization dedicated in worker threads" mirrors the
  async-tokenization change.
- `model_warmup=True` runs a dummy embed at startup to warm ORT arenas/allocators —
  emb's `preload` loads the session but does not run a warm inference; a boot-time
  warmup would remove the first-request allocation skew.
- Infinity's sibling project (mixedbread-ai/batched) implements slot-based batch
  packing: not just a token budget but length-aware slot assignment. A future
  refinement of this change could pack same-length texts into slots instead of
  arrival-order windows.