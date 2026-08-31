## Why

The LRU cache's `auto` sizing silently caps itself at **500MB** (`internal/server/cache.go` `autoTuneCache`: `min(budget, 500MB)`), even though the budget formula already reserves 10% safety + 25% model-estimate before taking 20% of the remainder. On a 32GB instance that's a 500MB ceiling on a ~4.2GB budget — so `-cache auto` retains far fewer distinct texts than the machine could hold, capping cache hit rate on mixed/repeated workloads. `BENCHMARK.md` even documents `-cache auto` as "~20% of available RAM", which the cap contradicts. Explicit sizes (`-cache 4GB`) already work, but the default should use the memory it safely can, and operators need a proportional dial rather than hand-computed byte values.

## What Changes

- **Auto-tune without the arbitrary cap.** `autoTuneCache` drops the hard 500MB ceiling. Budget = 20% of (`totalMem − 10% safety − 25% model estimate`), floor 64MB, new proportional ceiling of 50% of total RAM so absurd boxes don't over-allocate. On a 32GB box: ~4.2GB instead of 500MB → ~8× more distinct texts retained → higher hit rate on repeated/mixed workloads, no config change for `auto` users.
- **Percent cache sizes.** `cache:` YAML / `-cache` flag accept `"N%"` (= N% of total system RAM, the operator's explicit risk acceptance), validated (`0 < N ≤ 100`; garbage or out-of-range fails startup with a clear error). Human sizes (`"1GB"`) and `"auto"` keep working unchanged.
- **Dead code cleanup.** `autoTuneCache(defaultDim)`'s unused `defaultDim` parameter (vestige of the old entries-based formula) is removed.
- **Docs + spec alignment.** `BENCHMARK.md` cache section updated to the real auto formula; `config.yaml` comment updated; the `lru-cache` spec's stale auto-tune formula and metric names (`cache_max_entries` vs the actual `cache_max_bytes`) are corrected to match implemented reality.
- **No breaking changes.** Explicit sizes behave identically; `auto` users get a bigger, still safety-margin'd cache; cache disabled default is unchanged.

## Capabilities

### New Capabilities

### Modified Capabilities
- `lru-cache`: auto-tune formula (no 500MB cap; proportional ceiling), percent-based cache configuration syntax, and metric-name alignment (`cache_max_bytes`).

## Impact

- **Code:** `internal/server/cache.go` (`autoTuneCache`, `parseCacheConfig`); `internal/server/server_test.go` (parse/auto-tune tests); `BENCHMARK.md`; `config.yaml` comment.
- **APIs:** config surface only — additive `"N%"` syntax, `"auto"`/size strings unchanged. No RESP2 command changes (`EMB.INFO`/`EMB.STATS` already report `cache_max_bytes`, `cache_memory_bytes`, hit/miss/eviction counts).
- **Memory:** `-cache auto` now uses up to ~13% of total RAM (capped at 50%); cache holds raw float32 vectors + text keys only. OOM posture unchanged for the model/safety reserves.
- **Systems:** server only; no client, protocol, or wire changes. Cache remains per-process (scaled-out fleets don't share it — unchanged).