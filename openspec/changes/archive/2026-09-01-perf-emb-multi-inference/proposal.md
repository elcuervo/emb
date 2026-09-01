## Why

The dominant traffic pattern for `emb` is `EMB.MULTI` with **two different models and the
same input text** (e.g. `EMB.MULTI siglip2 "<T>" e5 "<T>"`). Each MULTI pays for two
single-text ONNX inferences (one per model) plus host-side post-processing.

Fact-verified model picture (siglip2 fetched from HuggingFace; e5 is the operator's
custom local export):

- **`siglip2`** (`listlessbird/siglip2-base-patch16-naflex-text-onnx`): graph outputs
  `text_embeds` **[batch, 768] — 2D pre-pooled**; prod weights are already int8
  (`text_model_int8.onnx`, per-channel dynamic int8; **min cosine vs PyTorch 0.9975**);
  single `input_ids` input; max length 64. Go work today: L2-normalize (normalize on) +
  marshalling.
- **`e5`** — **custom local export with pooling/output layers baked into the graph**
  (`pooled_sentence_embeddings_*`-style 2D output, `pooling: none`, `normalize: false`).
  Go work today: **none — pure marshalling** of the `[N, dim]` output tensor. (Note: the
  store `intfloat/e5-small-v2` export differs — 3D `last_hidden_state`; it is only a
  README example, not the served model.)

The goal is to make this dual-model path *blazing fast while returning the right
embeddings*. Because these embeddings are consumed by an OpenSearch k-NN index for
retrieval, **"right" means retrieval-correct (ranking/recall), not byte-identical**, which
unlocks SIMD L2 normalization and zero-copy marshalling without an index format change (the
server already emits fp32 float vectors).

## What Changes

- **Fast 2D/pre-pooled path** (both primary models): zero-copy float32→little-endian
  marshalling with reused per-worker row buffers. For `siglip2` (`normalize: true`), add
  SIMD/row-parallel L2-normalize first. For `e5` (`normalize: false`, pooling baked in),
  the path becomes a pure, near-free buffer export of the `[N, dim]` output.
- **Fast 3D pooling path** (`minilm`/`bge` test models, and general 3D deployments):
  SIMD/row-parallel mean(CLS)-pooling over the attention mask + L2 normalize, replacing the
  scalar `MeanPoolAndNormalize` hot loop; optionally fold pooling into the ONNX graph.
- **Session tuning**: an idle-flush heuristic (lone request while the run loop is idle
  executes immediately) and an A/B of ORT execution mode + `intra_op_threads` under
  concurrent dual-session (`siglip2` + `e5`) load.
- **Correctness gate**: a validation harness asserting mean cosine ≥ 0.99 and top-k ranking
  retention between the fast path and the fp32 baseline.
- Quantized-artifact behavior is explicitly **out of scope**: no downloader matching
  changes; `siglip2` already runs int8 via the existing `quantize: auto` name resolution
  (validated 0.9975), and the custom `e5` stays fp32.

## Capabilities

### New Capabilities

- `inference-performance`: Covers the fast 2D/pre-pooled output path (zero-copy
  marshalling, SIMD L2 normalization, buffer reuse), the fast 3D pooling path (SIMD and
  row-parallel mean/CLS pooling, optional pooling-in-graph), an idle-flush batching rule,
  and session thread/execution-mode tuning for concurrent multi-model runs.

### Modified Capabilities

- `smart-batching`: Adds the idle-flush requirement so a single pending request does not
  wait out the batching window when the run loop is idle.

## Impact

- **Code**: `internal/pipeline/pooling.go` (2D fast path + 3D SIMD pooling),
  `internal/pipeline/batch.go` + `pool.go` (idle-flush), `internal/onnx/runtime.go`
  (session options, execution mode), `internal/registry/registry.go` (actionable
  output-tensor warnings), `cmd/` + docs.
- **Config/README**: fix the README `e5` example (it points at `intfloat/e5-small-v2`,
  whose 3D export must not be configured `pooling: none`); the served custom `e5` is
  unaffected. Document `siglip2` `output_tensor` drift (`text_embeds` vs configured
  `pooler_output` — the server auto-falls back; make the warning actionable).
- **Dependencies**: optional SIMD library (e.g. `kelindar/simd`, `vectormaths` for L2).
- **Non-goals (ruled out)**: fp16/bf16 on CPU (emulated/slower per ORT); changing the wire
  format to int8/byte vectors (would break OpenSearch `float` mappings); byte-identical
  guarantees (near-identical is accepted for retrieval); any quantized-artifact
  autodiscovery work (explicitly out of scope per operator).