# Tasks

## 1. The share is counted over the cache's lookups

- [x] 1.1 `CachedShare` divides the window's answered lookups by the answered
      plus the missed, and returns the lookups it counted; `Point`'s comment
      says which unit it carries. Verify: `TestCachedShareOfTheNode` asserts a
      warm window (299 hits, 1 miss, 1 run) reads 99.7% and never above 100%.
- [x] 1.2 `ModelPoint` carries the model's misses; `ModelCachedShare` and the
      eviction share `cacheUndersized` grades (`grow the cache`) are counted over
      the model's lookups, not its runs. Verify:
      `TestCacheUndersizedCountsLookupsOnce` asserts 25 evictions over 400
      lookups and `TestModelPhaseSplitAndDominance` asserts 12/20 = 60%.
- [x] 1.3 `Advice.CachedPct`'s doc, `cachedShareOfWindow`'s note on the event
      ring fallback, and the headless comment state the unit. Verify: `just lint`.

## 2. The spec and the documentation state the unit

- [x] 2.1 `emb-top-dashboard`'s cache readout requirement defines the share as
      the answered over the answered plus the missed, forbids
      `total_requests` as its denominator, and carries a scenario pinning the
      0-100% bound. Verify: `openspec validate cache-share-counts-lookups`.
- [x] 2.2 `docs/operations.md` describes the panel's clause and the
      `cached_pct=` token as the share of the window's cache lookups, and the
      rationale paragraph states why the requests that ran cannot be the
      denominator. Verify: `grep -n cached_pct docs/operations.md`.

## 3. Verification on the served surfaces

- [x] 3.1 A live node under the load that used to read 29900% (300 identical
      texts) reports `cached_pct=99.8` (599 of 600 lookups) from `emb-top -once`.
      Verify: run the probe against `bin/emb` with a cache configured.
- [x] 3.2 `go test -count=1 ./internal/embtop/ ./cmd/emb-top/` and `just lint`
      pass.
- [x] 3.3 Re-record every take with `just website-topviz`, `just
      website-clusterviz` and `just website-stressviz`, and check each plate's
      clause reads within 0-100%. Verify: `python3 website/tools/published-tree.py`
      and the recorded `runs/` frames.
