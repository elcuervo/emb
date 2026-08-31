## Context

See proposal.md — Why. Current `autoTuneCache` (`internal/server/cache.go:108-128`) computes a safety-margin'd budget but clamps it with `min(budget, 500MB)`, contradicting both the documented "~20% of RAM" claim (`BENCHMARK.md`) and the machine's available headroom. Explicit byte sizes already work; `"N%"` and the bigger auto default are the gap. The `lru-cache` spec is also stale (formula + `cache_max_entries` metric name vs the implemented `cache_max_bytes`).

## Goals / Non-Goals

**Goals**
- `-cache auto` uses the memory it safely can: no 500MB cap, proportional guardrails.
- Operators can size by percentage: `cache: "N%"` of total RAM.
- Docs and spec describe the real behavior; metric names match implementation.

**Non-Goals**
- No command/protocol changes (`EMB.INFO`/`EMB.STATS` already expose `cache_max_bytes`, `cache_memory_bytes`, hit/miss/eviction counts).
- No cache *architecture* change: still a global in-process LRU keyed `model:text`; no sharing across scaled-out processes; no concurrency redesign.
- No semantic key improvements (case folding/normalization of text keys) — memory sizing only.

## Decisions

### 1. Auto budget: keep the formula, drop the arbitrary cap, add a proportional ceiling

```
remaining = mem − mem/10 (safety) − mem/4 (model reserve)        # 65% of RAM
budget    = max(64MB, min(0.20 × remaining, mem/2))              # ~13% of RAM
```

- **Why remove the cap:** it is the entire reason `auto` under-delivers; the formula already self-reserves 35% of RAM for safety + models.
- **Why a 50% ceiling at all:** it never binds at current constants (13% ≪ 50%) — it is defense-in-depth so a future formula change (e.g., a smaller model reserve) cannot silently push the cache past half the machine. One `min` call, documented intent.
- **Alternative considered:** dropping the ceiling entirely (formula is self-bounding). Rejected: an unbounded budget is one constant edit away from OOM; the ceiling documents the invariant "cache never exceeds 50% of RAM for `auto`".

### 2. Percent syntax is a separate, explicit path

`parseCacheConfig` gains: suffix `"%"` → strip, parse float, validate `0 < pct ≤ 100`, budget = `pct/100 × TotalSystemMemory()`. Invalid (garbage string, `0%`, `150%`) → startup fails with a clear message (same posture as today's invalid sizes).

- **Why percent of *total* RAM, not of the auto-remaining:** the percent is the operator's explicit risk acceptance — they said how much they want; applying hidden safety margins to an explicit number would make the dial lie. `auto` remains the margin'd default.
- **Why not a float syntax or a new `cache_margin` key:** single-string config that flows through the existing `cache:` YAML / `-cache` flag unchanged; no new config fields, no docs surface growth.
- **`TotalSystemMemory() == 0` (unsupported platform fallback):** `auto` already falls back to 100MB; percent sizes error out with "cannot determine system memory" instead of silently allocating 0 bytes.

### 3. Dead parameter removed

`autoTuneCache(defaultDim int)` keeps an unused `defaultDim` (vestige of the old entries-based formula, per `git log` the cache shipped once). It becomes `autoTuneCache()`; the `auto` branch in `parseCacheConfig` drops the `384` literal. Pure cleanup, zero behavior change.

### 4. Validation measures the actual win, not a tautology

The existing `just bench-cache-size` (`redis-benchmark -n 500` with the *same* text) cannot distinguish 500MB from 4GB — both hold one entry. Validation instead:

- **Unit (`TestParseCacheConfig` + new `TestAutoTuneCache`):** percent parsing/validation matrix; `auto` budget always ≥ 64MB, ≥ the old non-capped formula value, ≤ `mem/2`, and on machines with ≥ 4GB RAM it SHALL exceed 500MB (skipped otherwise) — the regression guard for the cap.
- **Working-set experiment (documented, manual):** embed ~200k distinct texts to warm, then replay a mixed subset; compare `EMB.INFO … cache_hit_rate` at `-cache 500MB` vs `-cache auto` on a large-RAM box, and write the observed delta into `BENCHMARK.md`.

**Why a documented experiment rather than a harness:** the harnesses (`bench/bench.rb`, `just bench-cache-*`) are built around latency/throughput, not working-set retention; a permanent hit-rate harness is a bigger change than the user asked for.

## Risks / Trade-offs

- **OOM from a bigger default cache** → the 13%-of-RAM budget sits inside the 35% already reserved away from models/OS; 50% ceiling; percent is explicit consent; explicit byte sizes still override everything.
- **Static 25% model reserve may be wrong for a given deployment** (e.g., multiple big models) → reserve is conservative; `int8-weight-quantization` shrinks models further; any operator can pin `cache: "1GB"`.
- **Single-mutex LRU contention at larger sizes** under heavy mixed GET/SET churn → unchanged architecture; the mutex cost is per-request regardless of entry count; noted as an open thread (sharded concurrent LRU) if it ever shows up in the throughput benchmark.
- **Metric-name spec drift (cache_max_entries)** → aligned to the implemented `cache_max_bytes` in this change's spec delta; operators reading the spec get what the binary emits.

## Migration Plan

- Config-compatible: `"auto"`, human sizes, and empty all behave as before, except `auto` now yields more cache. No flag deprecation, no rollout ordering.
- Rollback: revert the change; `auto` returns to 500MB. There is nothing on the wire to unwind.

## Open Questions

None — the sizing formula, percent semantics, ceiling, and validation approach are settled; anything further (sharded LRU, semantic key normalization, cross-process cache) is deliberately out of scope and would be its own change.