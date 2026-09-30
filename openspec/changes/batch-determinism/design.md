# Design

## Root cause

Dynamic-quantized int8 ONNX exports quantize activations at runtime with
`DynamicQuantizeLinear`: scale/zero-point are derived from the **whole input
tensor's min/max**. For a batched run the input is `[batch, seq, dim]`, so the
scale of *every* row depends on the extrema of *all* rows in the batch. A text
computed solo vs inside an 8-row batch therefore lands on a different
quantization grid. Measured on the real server (vendor siglip2 int8 text
export, default 1ms batcher):

| scenario | same text, fresh instances | result |
|---|---|---|
| solo vs solo | deterministic | byte-identical |
| solo vs 8-row burst | batch-composition dependent | \|δ\| up to 6.7e-2, 768/768 dims differ |
| `timeout: 0` (worker pool) solo vs burst | deterministic | byte-identical |

fp32 graphs and static-quantized (QDQ — constant baked scales) graphs have no
batch-sensitive op and are batch-invariant (fp32 verified byte-exact across
runs and batch shapes). **The fix is to gate batching on graph determinism and
document the export strategy that keeps int8 perf.**

## Approach: probe-gated batching

### The probe (load-time, one-shot, cached)

At `ensurePool`, when `timeout > 0` and policy != `off`:

1. Pick two canned probe texts (fixed, diverse, chosen to move activation
   extrema: e.g. a short phrase and a long/dense one).
2. Embed each alone (2 single-row runs) and both co-batched in one 2-row run
   (1 run) on the model's own session.
3. Byte-compare each text's alone vs co-batched output (exact, tolerance 0 —
   same process/build, so any difference is batch dependence).
4. Cache the verdict on `ModelEntry` (`batchDeterminism` + reason); publish to
   stats/INFO.

Cost: one batch run + two single-row runs at load (~ms for these graphs),
once per model lifetime.

### Gating

In `ensurePool`, before creating the pool — unconditional, no policy flag:

```
if timeoutMS == 0                                 → worker pool (explicit "don't batch"; no probe)
if timeoutMS > 0 && probePassed                   → batcher pool (unchanged)
if timeoutMS > 0 && probeFailed                   → log warning; timeoutMS = 0 (worker pool)
```

Degrading to `timeoutMS = 0` reuses the existing worker-pool path — the exact
deterministic behavior operators configure manually today (explicit
`timeout: 0`), now automatic and versioned.

### Config

**No new config key.** Batching keeps exactly its current surface
(`timeout`, `max_batch`, `max_batch_tokens`). The probe is the default
implementation, not an option: whenever batching is enabled, determinism is
verified at load and the model degrades to unbatched if the graph is
batch-sensitive. Operators who need fail-loud assert the degradation log line
(`batch_determinism=failed`) in a CI/deploy check instead of a config value.

### Observability

- `EMB.INFO <model>`: `batch_determinism` (`passed` | `failed` | `untested`),
  `batch_determinism_reason` (`dql_batch_dependence` | `probe_error` |
  `passed` | `untested`),
  and the effective `batching_timeout` after gating (0 when degraded).
- `EMB.STATS`: per-model `batch_determinism` alongside the existing counters.
- Boot log: one stable, greppable degradation line
  (`batch_determinism=failed reason=...`) so deployment
  pipelines can fail on non-deterministic graphs without a config flag.

### Deterministic-export strategy (deployment side)

To keep batching *and* int8-size memory: serve static-quantization (QDQ)
exports produced from a calibration pass (optimum `ORTOptimizer`/
`ORTOptimizer.quantize(static=True)` over a representative corpus). QDQ bakes
per-layer scales as constants → no DQL → probe passes → batching enabled. fp32
exports are the zero-effort fallback (bigger + slower, probe passes). The
deployment-side export/config changes are tracked in tasks; emb itself only
consumes whatever graph the config points at and gates on what it finds.

### Validation plan

1. Unit: fake sessions that are batch-invariant and batch-sensitive —
   `auto` degrades the latter, `enforce` rejects, `off` batches; probe runs
   exactly per-model; verdict cached/observable.
2. Integration (real graphs): vendor siglip2 int8 export → probe **fails** →
   model unbatched (matches today's workaround bytes, automatic); an fp32
   export → probe **passes** → batched, byte-exact solo-vs-burst.
3. Regression: the real-server matrix (solo/burst/reprod) on the fp32 and
   static-QDQ graphs with batching enabled → byte-identical.
4. Search-stability: threshold-crossing counts (score distributions around
   any configured `min_score`) before/after the probe-gated config is live —
   flat once deterministic serving is in place.

## Options considered

| Option | Verdict |
|---|---|
| Keep `timeout: 0` everywhere (current workaround) | Rejected as the end state — sacrifices batching throughput/async tokenization; no guarantee mechanism |
| Serve only fp32 exports | Deterministic and proven; higher memory/latency (4×, ~2–3×); viable fallback, not the only path |
| Static-QDQ int8 re-export | Keeps int8 perf; requires export tooling + calibration artifact — the medium-term path to int8+determinism |
| **Probe-gated batching, flagless (chosen)** | Safe-by-construction at the serving layer for ANY graph; auto-degradation; observability; zero new config surface; works with either export strategy above |

## Risks / non-goals

- The probe proves batch-invariance for the probe inputs only, but batch
  dependence is a *graph property* (presence of dynamic scale ops), so a
  probing pair that moves extrema is sufficient in practice; a false pass is
  visible in `EMB.INFO`/boot log. Fail-loud is recovered operationally: a CI
  check greps the degradation log line. There is no escape hatch to batch a
  non-deterministic graph — the documented exit is a deterministic export
  (fp32 or static-QDQ).
- Non-goal: changing the vendor graphs in emb; graph determinism detection via
  op inspection (requires protobuf parsing in the binding) is future work —
  the probe is simpler and sufficient.
- The cross-ORT-build numerical floor remains (constant per build, ~6.5e-3
  maxAbs for int8) — orthogonal; deterministic here means same-build+same-batch
  byte stability, which is what the cache/fingerprint contract needs.