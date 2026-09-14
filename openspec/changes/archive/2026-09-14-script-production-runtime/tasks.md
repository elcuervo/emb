## 1. Script observability (prerequisite)

- [x] 1.1 Emit a `MonitorEvent` from `runScripted` on completion (model, text count, latency µs, error flag; no text payload), mirroring `handleEMB` — verify: a test sends `EMB.EVSHA` then `MONITOR` and asserts one event with the right model/count/flag and no payload.
- [x] 1.2 Add cumulative scripted counters (`script_requests`, `script_errors`, `script_avg_latency_us`) to the server and emit them from `EMB.STATS` — verify: a test runs N evaluations (one failing) and asserts the three fields.
- [x] 1.3 Add per-model scripted request counts to the `EMB.STATS`/`EMB.INFO` model breakdown, distinct from embedding counts — verify: a test that mixes `EMB` and `EMB.EVSHA` on one model asserts both counts.
- [x] 1.4 Report per-model scripted resource footprint (named-tensor sessions open, tokenizer loaded) through stats — verify: a test asserts 0 sessions after an `emb.embed`-only script and ≥1 after an `emb.run` script.
- [x] 1.5 Extend the `EMB.STATS` RESP count-parity test to cover the new fields with and without scripted traffic — verify: the existing count-parity test passes with the added fields.

## 2. Lazy and bounded script resources

- [x] 2.1 Make the `emb.run`/`emb.run_batch` host binding lazy so `ScriptResources()` is created on first raw-tensor call, not host construction — verify: a test evaluating a constant-returning script asserts no named-tensor session was opened.
- [x] 2.2 Reuse the embedding pool's tokenizer in `openScriptResources` instead of loading a second one — verify: a test asserts a single tokenizer instance for a model that serves both `EMB` and scripts.
- [x] 2.3 Bound `script_workers` auto-tune by the model's embedding pool worker count — verify: a test with `workers: 2` and unset `script_workers` asserts at most 2 script sessions.
- [x] 2.4 Document the `script_workers` default change in the changelog/README — verify: README states the bound and the reason.

## 3. `emb.embed` and the shared embedding path

- [x] 3.1 Extract `Server.embedTexts(entry, model, texts)` containing the cache-read → miss-compute → cache-write sequence currently inlined in `handleEMB` — verify: existing `EMB` cache tests pass unchanged against the refactor.
- [x] 3.2 Refactor `handleEMB` to call `embedTexts` with no behaviour change — verify: the full `internal/server` suite passes and `EMB` replies are byte-identical.
- [x] 3.3 Add `Hosts.Embed` and bind `emb.embed` conditionally for models with an embedding configuration (`dim > 0`, pooling ≠ `none`), resolving the pool lazily on first call — verify: a test asserts `emb.embed` errors as unavailable on a non-embeddable model and does not open its pool.
- [x] 3.4 Implement `emb.embed(text)` and `emb.embed({texts…})` returning vectors / arrays of vectors in input order — verify: a test compares each returned vector to the `EMB` reply for the same text.
- [x] 3.5 Implement the packed form `emb.embed(texts, {bytes = true})` — verify: a test asserts the packed string equals `emb.math.float32_bytes` of the array form and has length `dim × 4`.
- [x] 3.6 Verify scripted and native embeddings share cache entries — verify: a test warms `EMB <model> x` with caching on, then evaluates an `emb.embed` script for `x` and asserts the scripted path performed no new inference.
- [x] 3.7 Verify `emb.embed` is batched and does not open a second pool — verify: a test asserts no named-tensor sessions are opened and that a concurrent `EMB` + `emb.embed` pair is admitted to the same batcher.
- [x] 3.8 Charge `emb.embed` output against the per-evaluation tensor-element budget and the text-count cap — verify: a test over the budget fails that evaluation only and leaves other requests unaffected.

## 4. Similarity and distance methods

- [x] 4.1 Implement a shared vector-operand parser accepting a Lua number array or a packed float32 string, with length and 4-byte-alignment validation — verify: unit tests cover array, packed, mixed, mismatched-length, and misaligned inputs.
- [x] 4.2 Implement `emb.similarity(a, b [, metric])` with `cosine` (default) and `dot`, higher-is-more-similar, zero-magnitude cosine yielding 0 — verify: tests assert default-equals-cosine, identical-vectors→1, dot value, mixed operand forms, length-mismatch error, zero-vector→0.
- [x] 4.3 Implement `emb.distance(a, b [, metric])` with `l2` (default), `l2sq`, and `cosine` (= 1 − cos), lower-is-closer — verify: tests assert the default, `similarity + distance = 1` for cosine, `l2sq = l2²`, and identical-vectors→0.
- [x] 4.4 Reject unknown metric names with an error listing the accepted metrics — verify: tests assert the error text names the valid metrics.
- [x] 4.5 Implement `emb.image.embed(bytes)` / `({bytes…})` delegating to the image resources, rejecting URLs like `EMB.IMG` — verify: a test asserts vector equality with `EMB.IMG` for the same bytes and an error for an `https://` argument.
- [x] 4.6 Add a cross-modal end-to-end test (text via `emb.embed`, image via `emb.image.embed`, scored with `emb.similarity`) — verify: the test runs against a dual-encoder testbed model and asserts a sane score for a matching pair.

## 5. Packed tensor I/O

- [x] 5.1 Parse the `emb.run` options table (`{bytes, outputs}`) and thread it through `runHost`/`runBatchHost` — verify: tests cover no options, `bytes = true`, `outputs = {...}`, and both.
- [x] 5.2 Implement the packed output form `{shape, bytes, dtype}` as the inverse of the existing `bytes` input — verify: a round-trip test feeds a packed output back as an input and reproduces the tensor.
- [x] 5.3 Verify packed byte length equals shape × element width for `f32` and `i64` — verify: a test asserts the length invariant on both dtypes.
- [x] 5.4 Implement selective outputs so unnamed graph outputs are not converted — verify: a test asserts only requested keys are present and that an unknown name errors listing available outputs.
- [x] 5.5 Apply both options to `emb.run_batch` — verify: a test asserts per-item results contain only requested, packed outputs.
- [x] 5.6 Reuse ORT output tensors by shape in `NamedRuntimeSession.RunNamed` instead of per-call auto-allocation/destroy — verify: a benchmark shows reduced allocation (`-benchmem`) with identical outputs.
- [x] 5.7 Serialize packed outputs in a single allocation directly from the tensor data (no per-element Lua table, no staging buffer) — verify: the packed round-trip and exact-bytes tests pass and the serializer allocates once per output. (The ORT→Go safety copy in `namedTensorFromValue` remains; eliminating it would require changing the `RunNamed` contract and is deferred.)
- [x] 5.8 Count packed outputs against the tensor-element budget and add a per-evaluation materialized-byte bound — verify: a test over the bound fails that evaluation only, and a test asserts packed and array forms consume the same element budget.

## 6. Host-side math over packed buffers

- [x] 6.1 Implement `emb.math.dot`, `cosine`, `l2`, `norm` accepting arrays or packed bytes — verify: tests assert packed and array operands agree and misaligned buffers error.
- [x] 6.2 Implement `emb.math.mean_pool(hidden, shape, mask)` (masked mean + L2 normalize) and `cls(hidden, shape)` — verify: a test compares `mean_pool` output with the embedding pipeline's pooling of the same tensor, and a masked-position test asserts padding is excluded.
- [x] 6.3 Implement `emb.math.topk`, `gather`, `slice`, `scale`, `add` — verify: tests assert descending `topk` order with 1-based indices, gather/slice index semantics, and shape validation errors.
- [x] 6.4 Accept packed bytes in `emb.math.sigmoid`, `softmax`, `argmax` — verify: tests assert parity with the array form.
- [x] 6.5 Validate `shape` against operand element count for every reduction — verify: tests assert a shape-mismatch error naming both values.

## 7. GLiNER2 reference implementation

- [x] 7.1 Rewrite `examples/scripts/gliner2.lua` using packed outputs and host math (removing the interpreted `at()` decode loop) — verify: the script runs end to end against the testbed model and returns the expected hash.
- [x] 7.2 Assert byte-identical entities against the current script's output for the golden corpus — verify: the extraction golden test passes with the rewritten script.
- [x] 7.3 Add a benchmark for the rewritten script and record it against the previous implementation — verify: `just bench-gliner` reports the new script and the improvement is captured in `benchmark-baseline.txt`.

## 8. API version and compatibility

- [x] 8.1 Add `emb.API_VERSION` as a string, set for this release's surface — verify: a test asserts a non-empty string and that a script can read it.
- [x] 8.2 Add a compatibility gate over `examples/scripts/`: every shipped script must still compile, and the full advertised host surface must be registered — verify: `TestExampleScriptsCompile` and `TestHostSurfaceIsComplete` pass. (Reply-level golden evaluation for every script is deferred: it needs each script's model downloaded; the GLiNER2 reference and the MinLM-based paths are covered by their own tests.)
- [x] 8.3 Assert no default behaviour changed: `emb.run` without options still returns per-element `data`, and reply-cache keys are unchanged for an unchanged script — verify: a test asserts the default result form and an unchanged `script.CacheKey` for a fixed script.

## 9. Documentation

- [x] 9.1 Write the script API reference covering every host function (signatures, operand forms, returns, errors, availability gating) — verify: every `emb.*` binding registered in `registerHosts` has a documented entry; reviewed against the code.
- [x] 9.2 Document similarity/distance polarity and per-metric formulas explicitly — verify: the reference states higher-is-more-similar for `similarity` and lower-is-closer for `distance`, with `cosine`/`dot` and `l2`/`l2sq`/`cosine` respectively.
- [x] 9.3 Write the production scripting guide (determinism/cache contract, `emb.embed` vs `emb.run`, packed workflow, limits and failures, memory and observability) — verify: docs exist and each listed topic is present.
- [x] 9.4 Split `examples/scripts/` into maintained `reference/` and illustrative `snippets/`, and re-point config/test paths — verify: the reference scripts are exercised by CI and the docs say which is which.
- [x] 9.5 Update the README scripting section and the `website/docs/` command/reference surface for the new API — verify: README lists the new functions and `just website` builds; docs cross-check against the reference.

## 10. Performance benchmarks and regression gate

- [x] 10.1 Add `BenchmarkScriptEmbedParity`: `emb.embed` + `emb.similarity` vs `EMB` at 8/32/128 tokens on a real model — verify: the benchmark reports both sides and the derived ratio.
- [x] 10.2 Add `BenchmarkScriptPackedRead`: packed output + host reduction vs a no-output run at 32/128 tokens, deriving µs per output element — verify: the benchmark reports the ratio and the per-element coefficient.
- [x] 10.3 Add `BenchmarkScriptSessionFootprint`: sessions opened for an `emb.embed`-only script vs an `emb.run` script — verify: the benchmark reports 0 and ≥1 respectively.
- [x] 10.4 Capture the baseline and wire the benchmarks into `just bench` — verify: `just bench` runs them and `benchmark-baseline.txt` contains their values.
- [x] 10.5 Assert the budget ratios in a test (1.30× parity, 1.35× materialization, 0.02 µs/element, 15% memory) against the baseline with a 10% regression threshold — verify: the parity test passes on the reference machine and fails when a threshold is artificially exceeded.
- [x] 10.6 Add a gated RSS measurement for the memory budget using the existing `EMB.STATS` `mem` field — verify: the test measures RSS before and after the first `emb.embed`-only evaluation and asserts the ≤15% bound.
