# Proposal

## Why

Batching produces a slight, bounded precision variance for dynamic-quantized int8 ONNX exports. These graphs compute activation quantization scales per *tensor* (`DynamicQuantizeLinear` over the whole `[batch, seq, dim]` input), so each row's quantization grid depends on the other rows in the same inference batch. The same text embedded solo vs. inside an 8-row batch therefore lands on a slightly different grid, and its output vector shifts accordingly.

**Measured on the real int8 export (solo vs. co-batched, byte-identical solo-vs-solo as the control):**

| quantity | value |
|---|---|
| same-text cosine (solo vs. batched) | 0.9988–0.9998, mean 0.9991 |
| worst deviation from 1.0 | 1.24e-3 |
| angular equivalent | ≤ 2.9° (typical ~2.4°) |
| per-dimension max \|δ\| | 6.7e-2 (all dims move; model-dependent) |

**This is a precision change, not output corruption.** Vectors remain ~99.9% cosine-similar across batch patterns; the effect is confined to decision margins: threshold-based retrieval can flip membership for candidates within the ~±1.2e-3 wobble band, and near-tie rankings can reorder. It is also *composition-dependent* — which grid a text lands on is decided by traffic coincidences inside the batching window (model-dependent magnitude; the measured class above is a 1ms window, 2–32 row runs; batch size barely matters once a text is not alone).

The real problem is the **determinism contract**, at any magnitude: emb's text cache keys on `(model, text)`, and downstream fingerprinting/dedup/incremental-refresh pipelines assume bytes are a function of the text. A slight variance violates that assumption in a way that is hard to reproduce (depends on arrival patterns) and hard to bound without the probe.

fp32 graphs and static-quantized (QDQ, baked-scale) int8 graphs have no batch-sensitive op and are byte-exact across batch shapes (verified). The bug class is precisely *dynamic* activation quantization.

The current workaround disables batching (`batching: timeout: 0`), trading batcher throughput for stability. This change fixes the underlying issue so batching stays the default for deterministic graphs and determinism is guaranteed by construction — without disabling a feature.

## What Changes

- **Load-time determinism probe (new, default implementation — no config flag).** Whenever a model loads with batching enabled, emb embeds two canned probe texts — once each alone, once co-batched — and byte-compares (exact, tolerance 0; same process/build). Batch-invariant graphs (fp32, static-quantized int8) pass; dynamic-quantized int8 graphs fail. Verdict is cached for the model's lifetime (probe cost: ~3 single-row-equivalent runs once, at load).
- **Unconditional gating.** A failing probe automatically downgrades the model to unbatched (worker pool, single-row — the deterministic behavior `timeout: 0` configures manually today) with a logged warning. There is no policy to re-enable batching on a non-deterministic graph; the documented exit is a deterministic export. Operators who need fail-loud assert the degradation log line in a CI/deploy check.
- **Observability.** `EMB.INFO <model>` and `EMB.STATS` report `batch_determinism: passed | failed | untested` plus a reason (e.g. `dql_batch_dependence`), and the boot log carries a stable, greppable degradation line for pipeline gates.
- **Deterministic-export guidance + tooling.** A documented, verified path for keeping both batching and int8-size memory: serve static-quantized (QDQ) exports — activation scales baked as constants, no DQL — produced from a calibration pass (optimum/onnxruntime quantizer over a representative corpus). fp32 exports are the zero-effort alternative (verified byte-exact and batch-insensitive).
- **Deployment.** Restore batching in serving configs once the served graphs pass the probe; boot logs confirm `batch_determinism: passed`.

No change to embedding values for deterministic graphs: the probe only *gates* batching — the worker-pool path and the batcher path produce identical bytes when the graph is batch-invariant (verified).

## Capabilities

### New Capabilities

- `batch-determinism`: embeddings served by a batched model SHALL be a deterministic function of the input text and the model, independent of batch composition — enforced by a load-time probe (default behavior, no configuration).

### Modified Capabilities

- `smart-batching`: batching for a model MAY be downgraded to per-request (unbatched) when the model's batch-determinism probe fails.

## Impact

- Code: `internal/registry/registry.go` (`ensurePool` gating, probe cache on `ModelEntry`), new probe module (pipeline or registry), observability (`EMB.INFO`/`EMB.STATS`), docs. No config changes.
- Behavior: dynamic-quantized int8 models stop batching automatically (same output as today's manual `timeout: 0`, no config); static/fp32 graphs keep batching and gain a guaranteed determinism contract.
- Perf: batching returns for deterministic graphs; memory unchanged (probe runs once at load); for failing int8 models the existing unbatched cost is already accepted.
- Dependency: none new at runtime; an export-side calibration script (optimum/onnxruntime quantizer) for deployers who want int8 + static. Weights/artifacts live in the deployment pipeline, not emb.
- Severity scope: this addresses a bounded precision variance (sub-1.3e-3 cosine on the measured class) whose practical impact is confined to threshold margins, near-tie rankings, and reproducibility of caches/fingerprints — not a broad correctness break. The fix eliminates the variance entirely for deterministic graphs and makes it explicit and observable for non-deterministic ones.