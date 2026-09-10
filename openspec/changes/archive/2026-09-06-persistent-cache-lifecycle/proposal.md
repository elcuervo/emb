## Why

The in-process embedding cache is fast but operationally incomplete: stale entries cannot be invalidated without restarting, and every restart or rollout discards a potentially expensive warm working set. The server already exposes `cache_file` and `cache_save` as forward-compatible runtime settings, so completing the cache lifecycle now turns that reserved surface into reliable behavior while preserving the existing request-path performance contract.

## What Changes

- Add `EMB.CACHE.FLUSH [model]` to invalidate either the complete cache or only entries belonging to one model, without disabling or resizing the cache.
- Add `EMB.SAVE` to request an asynchronous cache snapshot using the configured `cache_file`; reject overlapping saves deterministically instead of queueing them.
- Restore a valid snapshot through a streaming staged cache before the server becomes ready, preserving LRU order and discarding entries that are incompatible with the current model configuration, configured cache budget, restore limit, or live host-memory headroom.
- Implement independently configurable startup loading, periodic snapshots, save I/O throttling, and bounded best-effort final snapshots during graceful shutdown.
- Preserve the existing in-memory cache-key and lookup path unchanged. Snapshot files contain the current `model:text` keys and embeddings, are opt-in, owner-readable only, and are explicitly documented as sensitive application data.
- Write versioned, checksummed snapshots through a temporary file followed by an atomic rename; corrupt, truncated, or unsupported snapshots never prevent the server from starting.
- Expose persistence controls through YAML, CLI flags, and `CONFIG GET`; expose settings that are safe to change live through `CONFIG SET`. Controls include `cache_file`, `cache_load`, `cache_save`, `cache_save_on_shutdown`, `cache_restore_limit`, `cache_restore_reserve`, and `cache_save_rate_limit`.
- Add snapshot and flush counters/status to the existing observability surfaces and document cache-file sensitivity, permissions, compatibility, and failure behavior.
- Preserve performance invariants: when persistence is unconfigured there is no snapshot goroutine, timer, disk access, serialization, or added cache-lock work; when configured, filesystem I/O and encoding happen outside the cache lock and never execute inline in `EMB` or `EMB.MULTI` handlers.
- Add unit, concurrency, integration, corruption/recovery, lifecycle, and benchmark regression coverage, including an explicit no-regression gate for cache hits and misses.

## Capabilities

### New Capabilities

- `cache-administration`: Defines whole-cache and model-scoped invalidation through `EMB.CACHE.FLUSH`, including atomic visibility, concurrent-request semantics, metrics, authentication, and disabled-cache behavior.
- `cache-snapshots`: Defines Redis-RDB-inspired snapshot creation, host-memory-bounded streaming restore, independently configurable startup/periodic/shutdown behavior, compatibility validation, atomic files, corruption handling, observability, and request-path performance isolation.

### Modified Capabilities

- `lru-cache`: Adds invalidation and snapshot-copy integration while preserving the existing key identity, hit/miss, byte-budget, eviction, and LRU request-path behavior.
- `redis-config-command`: Makes the existing persistence settings operational and adds configuration/validation for load, shutdown-save, host-memory restore limits/reserves, and save I/O throttling.
- `server-lifecycle`: Adds startup restore before readiness and bounded best-effort snapshot completion during graceful shutdown.
- `server-stats-observability`: Adds cache flush and snapshot lifecycle counters/status without changing existing fields.

## Impact

- **Protocol:** New authenticated control commands `EMB.CACHE.FLUSH [model]` and `EMB.SAVE`; `EMB.HELP` documents both. Existing inference replies remain byte-for-byte compatible.
- **Server:** Changes affect `internal/server` cache storage, command registration, INFO/STATS rendering, configuration, server startup, and graceful shutdown coordination.
- **Registry/config:** Snapshot compatibility needs a stable per-model embedding-function fingerprint derived from model identity and output-affecting settings; it must not require loading a lazy ONNX session.
- **Ruby client:** Add thin wrappers for flush/save and expose the new status fields without introducing client-side persistence logic.
- **Storage/security:** Snapshot files contain embedding bytes and original input text, are sensitive application artifacts, and are created with owner-only permissions. Keeping the hot cache-key path unchanged is an explicit performance trade-off. No new third-party runtime dependency is required.
- **Performance:** The default, unconfigured path must remain structurally dormant and benchmark-equivalent. Restore streams from disk into a bounded staging cache without reading the whole file or duplicating embedding payloads. Enabled saves may consume background CPU and I/O, but cannot block inference on filesystem work or hold the cache mutex while encoding/writing; configurable rate limiting and performance gates cover active snapshots.
- **Compatibility:** The snapshot format is explicitly versioned and disposable. Unsupported or incompatible files are ignored with observable errors rather than migrated implicitly or treated as fatal server state.
