# Tasks

## 1. Probe module

- [x] 1.1 Add a batch-determinism probe (package-private in `internal/registry` or `internal/pipeline`): fixed canned probe texts, embed each alone and co-batched on the model's own session, byte-compare (exact, no tolerance). Verify: unit test with injectable fake sessions — a batch-invariant fake (returns identical bytes) and a batch-sensitive fake (row bytes depend on batch size) — asserts the probe classifies both correctly.
- [x] 1.2 Cache the verdict on `ModelEntry` (verdict + reason + effective timeout), computed once per model lifetime, never recomputed. Verify: `go test ./internal/registry/` with a counting session fake proves the probe runs once across repeated `GetOrInit`.

## 2. Gating in ensurePool

- [x] 2.1 In `ensurePool` (no config changes): when `timeoutMS > 0`, run the probe; on failure log a greppable degradation line (`batch_determinism=failed reason=...`) and degrade `timeoutMS = 0` (existing worker-pool path); on pass, build the batcher pool unchanged. When `timeoutMS == 0` (explicit `timeout: 0`), skip the probe. Verify: registry tests assert batched/unbatched pool construction against batch-invariant/batch-sensitive fake sessions, that `timeout: 0` skips the probe, and that a degraded model's pool construction equals the worker-pool path (`effectiveEmbeddingSessions` semantics).

## 3. Observability

- [x] 3.1 Surface `batch_determinism` (`passed`/`failed`/`untested`), reason, and effective `batching_timeout` in `EMB.INFO <model>` and per-model `EMB.STATS`. Verify: server tests assert the fields for a passed, a failed (degraded), and a `timeout: 0` model.

## 4. Documentation

- [x] 4.1 Document the determinism contract and the export strategy (fp32 fallback; static-QDQ int8 via optimum calibration to keep int8 perf) in `docs/configuration.md` and the product docs; document the probe (default, no flag) and the greppable degradation line for CI gates. Verify: docs render.

## 5. Verification (real graphs)

- [x] 5.1 Re-run the real-server matrix with the probe live: vendor int8 siglip2 export loads → probe fails → model unbatched, byte-identical solo-vs-burst (matches the manual `timeout: 0` workaround bytes). Verify: boot log shows degradation; the solo/burst comparison is byte-exact.
- [x] 5.2 An fp32 (or static-QDQ) text export loads → probe passes → batching enabled and solo-vs-burst byte-identical. Verify: `batch_determinism: passed` and the burned matrix.
- [ ] 5.3 (Deployment handoff) Restore batching in the serving config once the served graphs pass the probe; confirm boot logs show `batch_determinism: passed` and threshold-crossing counts (score distributions around any configured `min_score`) stay flat. Verify: config diff + boot log + before/after stats.