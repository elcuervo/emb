## Context

`emb` serves a `EMB.MULTI`-heavy workload: two models (`siglip2`, `e5`) embedding the same
input text in one command. Each MULTI is concretely:

```
EMB.MULTI siglip2 "<T>" e5 "<T>"
  → fanOut goroutines → 2 × Pool.Embed([]string{T})
      → siglip2: tokenize → 1 int8 ORT run → L2 + marshal
      → e5:     tokenize → 1 fp32 ORT run → marshal (pooling+norm baked in graph)
```

Verified model facts:

- **siglip2** (`listlessbird/siglip2-base-patch16-naflex-text-onnx`):
  `input_ids` → `text_embeds` [batch, **768**], **2D pre-pooled**, max length 64,
  prod int8 (`text_model_int8.onnx`, dynamic per-channel int8, min cosine vs torch
  **0.9975**). Go today: `ExtractPrePooled` + L2 (normalize on).
- **e5**: **custom local export with pooling/output layers baked into the graph**
  (`pooled_sentence_embeddings_*`-style 2D output, `pooling: none`, `normalize: false`).
  Go today: **pure marshalling, no math**. (The store `intfloat/e5-small-v2` export is 3D
  `last_hidden_state` and appears only in the README example — not the served model.)
- **minilm**/bge (repo test models): 3D `last_hidden_state`, mean-pooled in Go — the
  secondary target for the 3D pooling path.

Constraint that shapes everything: embeddings go into an **OpenSearch k-NN index** for
retrieval. The server already emits **fp32 float vectors**, so "right embeddings" =
retrieval-correct (recall/ranking); near-identical values are acceptable; no OpenSearch
`float` mapping change is needed.

## Goals / Non-Goals

**Goals:**
- Cut single-MULTI latency and raise dual-model throughput for `siglip2` + `e5`.
- Make the 2D path near-free: SIMD L2 (siglip2) + zero-copy marshalling; for `e5`
  (normalize off, pooling baked in) a pure buffer export.
- Replace the scalar 3D pooling loop (minilm/bge) with SIMD/row-parallel pooling, with
  pooling-in-graph as an option.
- Provably "right": cosine ≥ 0.99 and top-k recall retained vs fp32.
- Keep OpenSearch `float` k-NN mappings and stored-vector format identical.

**Non-Goals:**
- Quantized-artifact autodiscovery / downloader matching — explicitly out of scope.
- fp16/bf16 on CPU (emulated, slower per ORT) — excluded.
- Emitting int8/byte vectors on the wire (would change OpenSearch `float` mapping).
- Byte-identical guarantee — near-identical is accepted for retrieval.
- Changing `EMB.MULTI` command semantics, ordering, or MGET-style partial failures.

## Decisions

**D1 — Fast 2D/pre-pooled path (siglip2, e5).** For 2D/`pooling: none` models, `Run()`
already returns `[N, dim]` fp32. Replace per-row allocation + per-element byte writes with
reused per-worker row buffers and an `unsafe.Slice` little-endian view of the result,
copying only at the response boundary when a concurrent reply may outlive the reused
buffer. siglip2 normalizes first (D2); the custom `e5` (normalize false, pooling baked in)
is a pure export with no arithmetic. This is the highest-frequency path per MULTI (two
models × one text).

**D2 — SIMD/row-parallel L2-normalize (siglip2).** For `normalize: true` 2D models,
normalize rows with SIMD (`kelindar/simd`, `vectormaths` for L2) and row-parallel
fan-out. Row-parallel keeps each row near-identical; SIMD reorders within a row, acceptable
because near-identical is accepted. Rationale: siglip2 sequences are ≤64 tokens; the L2
pass dominates its small hidden tensor, so a fast kernel matters.

**D3 — Fast 3D pooling path (minilm/bge; optional pooling-in-graph).** Replace the scalar
`MeanPoolAndNormalize` (and add fast CLS extraction) with SIMD accumulation + row-parallel
fan-out over the attention mask for 3D models. Optionally wrap a 3D output with
mask-multiply + ReduceSum/Div + L2 nodes at load time so one `Run()` returns the final
vector. Secondary: primary models are 2D; this serves the repo test models and general
deployments, and must pass the D6 cosine gate.

**D4 — Session tuning for concurrent dual-session load.** A/B `ExecutionMode` (the code
forces `ORT_PARALLEL`; docs favor `SEQUENTIAL` for serial encoder graphs) and sweep
`intra_op_threads ∈ {1, 2, 4, physical cores}` with `siglip2` (768 int8) and `e5` (384)
running concurrently. Rationale: two ORT sessions share CPU; thread balance matters as much
as thread count.

**D5 — Batching idle-flush.** When the batching run loop is idle (no batch in flight), serve
a newly arrived request immediately rather than waiting the window. Rationale: drops
single-MULTI latency to "no artificial wait" while preserving burst batching. Autotuning
was considered and rejected (Risks).

**D6 — Correctness gate in the harness.** Embed a corpus with the fp32 baseline and each
fast path; report mean/min cosine and top-k ranking retention (nDCG); `just` target fails
if cosine < 0.99 or recall drops beyond a threshold. `EMB.MULTI` must stay byte-identical
to sequential `EMB` per the `emb-multi` contract (consistency check separate from the
cosine gate).

## Risks / Trade-offs

- **Custom `e5` not independently verifiable here**: its pooling/normalization layers live
  in the operator's export. The harness must validate the fast path against the served
  custom model; the store `intfloat/e5-small-v2` (3D) must NOT be used as the `e5`
  validation stand-in — README example needs correction so nobody serves it with
  `pooling: none` (which would slice a 3D buffer and corrupt batch ≥ 2 results).
- **Near-identical values**: SIMD shifts vectors ~0.99 cosine; no mapping change needed,
  but if the operator ever switches precision (e.g. an int8 `e5`), that requires a one-time
  OpenSearch reindex and embedding versioning (model_id + precision + normalizer). Do not
  mix precisions in one index.
- **Zero-copy safety**: a reused `unsafe.Slice` buffer must not be overwritten while a
  concurrent RESP response references it; copy at the response boundary when needed.
- **SIMD dependency**: third-party lib with AVX2 + Go fallback; D6 gate guards correctness.
- **Autotuning rejected**: window autotuning targets throughput/tail-latency under load and
  risks feedback loops; it does not reduce single-MULTI latency (inference-dominated).
- **2-pair shape limits batching**: group-by-model batching inside a MULTI collapses to the
  current per-pair calls (one text per model); batching wins come from cross-command
  coalescing + D5.