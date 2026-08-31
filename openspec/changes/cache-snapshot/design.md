## Context

See proposal.md — Why. Current cache surface (`internal/server/cache.go`): `Get`/`Set`/`Stats` only, keyed `"<model>:<text>"` → raw float32 bytes; byte-budget LRU (now auto-sized per the landed `bigger-memory-cache`). Shutdown path exists (`server.Shutdown`, signaled in `cmd/emb/main.go`). Config struct (`internal/config/config.go`) has `Cache string` — `CacheFile` slots in beside it.

## Goals / Non-Goals

**Goals**
- Restart-warm: dump on demand + on graceful shutdown, restore at boot, per-model, budget-aware, corruption-tolerant.
- Format is versioned and stable; vectors stay raw float32 (no widening — consistent with the float-bloat posture).

**Non-Goals**
- No periodic/auto-save intervals (Redis `save 900 1`) in v1 — on-demand + shutdown cover the "restart" case that motivated this.
- No compression (float32 data is incompressible; text keys are short — gzip would cost CPU for ~nothing; a future `snapshot: gzip` flag is a possible extension, not v1).
- No cross-process/shared cache, no replication — snapshot is local restart-warmth only.
- No restore of *which* LRU recency exactly — order is restored approximately (see Decision 3); exact reuse-order is a non-goal.

## Decisions

### 1. Snapshot needs a cache iteration API; dump in LRU order

`Cache` gains `Visit(fn func(model, text string, value []byte) bool)` walking the LRU list **front → back** (most→least recently used) under the existing mutex. The snapshot writer streams entries in that order.

**Why front→back:** restore rebuilds LRU by inserting in dump order — front entries re-inserted first become the recency-appropriate head, tail entries the warmth-appropriate tail. Exactness isn't required (see non-goals), but this gets "still-hot at shutdown → still-hot at boot" right for free.

**Why a Visit callback over returning a slice:** zero extra allocation for a cache that may hold 100k+ entries; the writer streams straight to the temp file.

### 2. Atomic write: temp + fsync + rename

`os.CreateTemp(dir, base+".tmp*")` → write all entries → `f.Sync()` → `f.Close()` → `os.Rename(tmp, path)`. On any error: remove the temp file, reply error / log warn.

**Why fsync:** a snapshot whose rename lands before its data flushes is a corrupt snapshot on power loss — the whole point is surviving restarts; one syscall per dump is negligible against 400k-entry writes.

**Why rename-not-truncate:** readers/tools and a concurrent boot must never observe a partial file; rename is the atomicity primitive.

### 3. Restore policy: loaded models only, budget-aware, garbage-tolerant

- **Ordering:** restore runs in `Server` bootstrap *after* model registration and *after* preloaded models are up (`preload: true` models are the only ones with known dims at boot). Entries for unloaded models are skipped and counted (`cache_restore_skipped`).
- **Budget:** the loader tracks `curBytes` against `maxBytes` exactly like `Set`'s eviction path — stop-and-log once the budget is exhausted rather than evicting a freshly-loaded snapshot (evicting right after restoring is wasted work).
- **Corruption:** streamed reader with strict length checks; on inconsistency (truncation, length beyond EOF), log `restored N entries, stopped at byte X: corrupt/mismatched format`, keep the valid prefix. Never fail boot — same posture as Redis's "unable to load DB... start with empty".

**Why preload-gated:** lazy models (`preload: false`, the default) don't have `dim` known at boot without forcing a load — forcing a load would auto-download big models just to fill a cache. Skipping is honest and observable via counters. (Note: this is the one real limitation — snapshot warmth is for preloaded models. Documented in tasks + README.)

**Alternative considered:** store `dim` per *model* in the snapshot header (like a mini model manifest) so lazy models could validate later. Rejected for v1: doubles the format surface, and a lazy model's first request would load anyway; preloaded-model scoping keeps v1 honest.

### 4. Format: explicit LE uint32 length prefixes, versioned envelope

```
magic   "EMBCACHE" (8B)
version uint32 LE (1)
count   uint32 LE
[model  uint32 len + bytes
 text   uint32 len + bytes
 dim    uint32 LE
 vec    dim*4 bytes    (float32 LE, server payload verbatim)
] * count
```

**Why per-entry `dim`:** validation at restore without consulting model state (which may be lazy), and self-contained portability if the same file is read back on a differently-configured boot.

**Why uint32 LE:** matches the wire format's LE posture; 4GB entry lengths are absurdly beyond reality; simple `binary.Write`/`binary.Read`.

### 5. Snapshot capture is a reference copy under a brief lock (BGSAVE-equivalent)

`Cache.Snapshot() []SnapshotEntry` collects the (model, text, value []byte) references while holding `c.mu`, then releases it; the writer streams to file **outside** the lock. Values are never mutated in place — `Set` replaces the reference — so captured references stay valid after unlock even if an entry is evicted or replaced while writing (the GC owns them).

**Why reference-copy rather than dump-under-lock or deep-copy:** holding the mutex across a 640MB file write would stall every request for seconds; deep-copying doubles peak memory for nothing. The lock hold is microseconds (pointer walks) regardless of cache size, so periodic saves can never stall the request path.

### 6. Periodic save loop mirrors Redis `save seconds changes` pairs

`cache_save: "900 1 300 10"` → parse into `(seconds, changes)` pairs (even count; both > 0; malformed → startup error like `parseCacheConfig` today). A 1-second ticker goroutine starts when persistence is enabled and `cache_save` is non-empty, and stops on graceful shutdown:

- each tick: for any pair, `elapsedSinceLastSave ≥ seconds && dirtyChangesSinceLastSave ≥ changes` → trigger a background save;
- `Cache.Set` bumps an atomic dirty counter; a successful save resets it;
- single-flight: if a save is already running, the tick is skipped (coalesced — Redis's "last save failed"/merge behavior);
- failures are logged, never fatal;
- graceful shutdown stops the loop then always writes a final snapshot (covers an interval not yet elapsed).

**Why a 1s ticker instead of sleep-until-next-pair:** pairs can be small (`save 1 1` in tests, `900 1` in prod); a 1s tick is the simplest correct driver and costs one goroutine wake/s. `EMB.SAVE` remains a synchronous manual path sharing the identical writer — one write function, two callers.

### 7. The save loop reads runtime config each tick

The loop reads `cache_file`/`cache_save` from a runtime-config accessor every tick, not captured-at-start values. Zero coupling with the `redis-style-info` change's `CONFIG SET`: when that lands, `CONFIG SET cache_file ...` just changes what the next periodic/shutdown save writes, with no notification or restart machinery.

### 8. Config: `cache_file` / `cache_save` beside `cache`

`Config.CacheFile string yaml:"cache_file"` + `-cache-file` arg; `Config.CacheSave string yaml:"cache_save"` + `-cache-save` arg. Persistence is inert unless the cache itself is enabled (`cache` non-empty) — mirroring how a Redis `dbfilename` without RDB saves does nothing harmful until save triggers.

## Risks / Trade-offs

- **Snapshot staleness between intervals (crash loss)** → bounded by the tightest configured pair; `EMB.SAVE` remains available for checkpoints; graceful deploys always final-save.
- **Periodic save racing heavy write traffic** → reference capture means the lock is held microseconds; the file write happens off the request path; single-flight prevents overlapping snapshots.
- **Save loop churn on a hot cache** → coalescing + atomic rename + single output file keeps disk bounded; a failing disk only logs (non-fatal per spec).
- **Restore cost at boot for huge caches** → streaming + budget cutoff; a 640MB file loads in ~seconds (sequential reads, no per-entry allocation beyond the string copies).
- **Disk bloat / double-write on multi-model setups** → single file, atomic replace; size reported by `EMB.SAVE`; path is operator-chosen.
- **LE float32 cross-platform** → vector bytes are the RESP2 payload verbatim (already LE and portable); format explicitly LE so an LE/LE host round-trips byte-identically.
- **Restore-before-model-load ordering bugs** → single bootstrap hook after registration; tasks include a startup-order test (entries for lazy models skipped, preloaded restored).

## Migration Plan

- Additive: `cache_file` default empty = feature off; existing deployments unchanged.
- Enablement: set `cache_file`, boot once with a warm cache doing `EMB.SAVE` (or SIGTERM), then restarts become warm.
- Rollback: unset `cache_file`; delete the file; nothing else changes.

## Open Questions

None for v1. Deferrable follow-ups (each its own change): gzip option, snapshot of *per-model dim manifests* to extend restore to lazy models, and fork/Copy-On-Write saves for extreme cache sizes.