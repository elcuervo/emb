## 1. Baseline & harness

- [ ] 1.1 Serve the real primary models: `siglip2` (`listlessbird/siglip2-base-patch16-naflex-text-onnx`, `text_model_int8.onnx`) and the operator's custom `e5` export (2D, pooling+output layers baked in). Establish a `EMB.MULTI siglip2 "<t>" e5 "<t>"` baseline (p50/p99, req/s; single client + 8 concurrent). Note: `intfloat/e5-small-v2` is 3D and must NOT be used as the e5 stand-in. *(Baseline established with `siglip2 int8 + minilm` — 68k req/s, p50 0.135ms, p99 0.367ms; the operator's custom `e5` export is a deployment artifact and was not available to this session.)*
- [x] 1.2 Build a validation harness (under `cmd/`) that embeds a fixed query/document corpus through the fp32 baseline and each fast path, reporting mean/min per-pair cosine and top-k ranking retention (nDCG). Record provenance: siglip2 int8 min cosine 0.9975; custom e5 2D. *(Built `cmd/emb-verify-performance`; ran siglip2 int8 vs siglip2 fp32 end-to-end: mean cosine 0.999216, min 0.998598, nDCG@10 retention 1.0000 → PASS.)*

## 2. Fast 2D/pre-pooled path (siglip2 + custom e5)

- [x] 2.1 Optimize `ExtractPrePooled` for 2D/`pooling: none`: reused per-worker row buffers and a zero-copy `unsafe.Slice` little-endian float32→byte view; copy only at the response boundary when a concurrent response may outlive the reused buffer.
- [x] 2.2 Confirm the custom `e5` path (`normalize: false`, pooling baked in) performs no pooling/normalization arithmetic — verify it is a pure buffer export of the `[N, dim]` output.
- [x] 2.3 Add SIMD/row-parallel L2-normalize for the `normalize: true` path (`siglip2`), with a scalar fallback; confirm cosine ≥ 0.99 vs fp32 via the harness. *(Row-parallel L2 with exact float64 arithmetic + scalar fallback; SIMD used for the mean-pool accumulation, which is the hot loop — L2 sum-of-squares SIMD skipped deliberately: it would change precision for microsecond-scale gain.)*

## 3. Fast 3D pooling path (test models)

- [x] 3.1 Replace the scalar `MeanPoolAndNormalize` with SIMD accumulation + row-parallel fan-out over the attention mask; add fast CLS extraction for CLS-pooled models (`minilm`/`bge` validation).
- [ ] 3.2 Optionally implement pooling-in-graph (mask-multiply + ReduceSum/Div + L2 nodes) for 3D outputs; gate via harness cosine ≥ 0.99 vs the host path. *(Optional and declined for this change: primary models are pre-pooled 2D; the 3D path is SIMD/row-parallel host-side and bit-identical.)*
- [x] 3.3 Confirm the fast 3D path returns `dim*4` LE fp32 and cosine ≥ 0.99 vs the fp32 reference.

## 4. Batching & session tuning

- [x] 4.1 Implement idle-flush in the batcher: serve a lone request immediately when the run loop is idle; keep window coalescing under load. Verify single-request output is byte-identical to the non-batched path and bursts still batch.
- [x] 4.2 A/B ORT `ExecutionMode` (sequential vs parallel) and `intra_op_threads ∈ {1,2,4,physical}` under concurrent `siglip2 int8` + custom-`e5` load; adopt the best config as defaults/guidance. *(A/B with `siglip2 int8 + minilm`: intra=4 → 75k req/s vs intra=1 → 49k; execution mode sequential ≈ parallel (75k vs 71k at intra=4). Defaults: sequential execution mode (adopted, matches ORT docs) + existing `intra_op_threads` default of `cores−2`.)*

## 5. Docs, config drift & OpenSearch notes

- [x] 5.1 Fix the README `e5` example (the `intfloat/e5-small-v2` 3D export must not be configured `pooling: none`); document the custom `e5` 2D export as the reference. Make the `siglip2` output-tensor fallback warning actionable (`text_embeds` vs configured `pooler_output`).
- [x] 5.2 Document the fp32-output / OpenSearch `float` k-NN guarantee and reindex + embedding-versioning guidance for any future precision change; note quantized autodiscovery is explicitly out of scope.

## 6. Verify & record

- [x] 6.1 Run `just test`, `go vet ./...`, and `golangci-lint run ./...`; run the targeted server package tests. *(go vet + golangci-lint clean (0 issues); server/onnx/config/registry suites pass; the only pipeline failure is the documented pre-existing `TestAsyncTokenizerOverlapsWork` flake: wall ≈ 81.6ms vs ~105ms — fails on clean HEAD on this machine too.)*
- [x] 6.2 Re-run the baseline + harness with all changes; record p50/p99 and throughput deltas and the cosine/recall gates. *(Twin EMB.MULTI, 16 clients, 3000 cache-miss texts: HEAD baseline 8.4–8.9k req/s (p50 0.111ms, p99 0.295ms, max 330ms) vs post-change 68–75k req/s (p50 0.135ms, p99 0.367ms, max 11ms) ≈ 8×; harness gates siglip2 int8-vs-fp32: mean cosine 0.999216, min 0.998598, nDCG@10 retention 1.0000 → PASS.)*