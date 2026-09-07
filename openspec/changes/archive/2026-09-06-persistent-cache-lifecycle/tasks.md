## 1. Baseline and performance harness

- [x] 1.1 Add focused Go benchmarks for cache `Get`, `Set`, cached `EMB`, and uncached `EMB`, recording allocations and stable fixture sizes without changing production behavior.
- [x] 1.2 Capture repeated pre-change benchmark samples inside `nix develop`, analyze them with `benchstat`, and record raw environment/results as the comparison baseline in `BENCHMARK.md` or a versioned benchmark artifact.
- [x] 1.3 Add an integration benchmark harness that drives concurrent cache hits and misses while a gated/rate-limited snapshot source/sink runs, reporting throughput, p50/p99 latency, snapshot throughput, restore peak RSS, and resolved host-memory limits.

## 2. Cache administration primitives

- [x] 2.1 Add cache-level whole-flush support using O(1) map/list replacement while preserving the configured byte budget and cumulative hit/miss/eviction counters.
- [x] 2.2 Add model-scoped flush support with correct entry counts, byte accounting, per-model live counts, preserved remaining LRU order, and separate flush metrics.
- [x] 2.3 Add table-driven unit tests for whole, scoped, empty, disabled, unknown-model, accounting, LRU-order, and post-flush insertion semantics.
- [x] 2.4 Add deterministic gated concurrency tests covering `Get`, `Set`, eviction, resize, snapshot-view capture, whole flush, and scoped flush; verify them with `go test -race` and no sleeps.

## 3. Snapshot-safe cache view

- [x] 3.1 Define immutable snapshot entry/view types and capture MRU-to-LRU key/value references under the existing cache mutex, with all payload copying and encoding deferred until after unlock.
- [x] 3.2 Ensure cache replacement never mutates published embedding slices in place and add gated tests proving captured values remain stable through replacement, eviction, resizing, and flushing.
- [x] 3.3 Add the cache mutation generation and successful-save generation tracking inside existing mutation critical sections, with tests for dirty/clean transitions and no hit-path synchronization additions.
- [x] 3.4 Instrument snapshot capture duration without scanning or allocating when status is read.

## 4. Versioned snapshot codec and file safety

- [x] 4.1 Implement the version-1 streaming binary encoder/decoder with magic, explicit version, bounded fixed-width lengths, fp32/endianness metadata, model fingerprint table, MRU-to-LRU entries, fixed buffers, and SHA-256 whole-payload checksum using only the standard library.
- [x] 4.2 Enforce cache/model/host-headroom-derived allocation limits, dimension/value-length consistency, duplicate-key handling, and exact EOF/checksum validation before any staged entry can be served; prohibit whole-file reads and record-count-sized preallocation.
- [x] 4.3 Add round-trip and golden-file tests covering multiple models, Unicode/binary-safe text keys, empty caches, maximum legal lengths, LRU order, and deterministic bytes.
- [x] 4.4 Add table-driven corruption tests for every field boundary, bad magic/version/checksum, truncated data, integer overflow, excessive counts/lengths, duplicate keys, wrong dimensions, and oversized entries; add a decoder fuzz target that asserts no panic or unbounded allocation.
- [x] 4.5 Implement injected filesystem operations and atomic `0600` temp-write, file sync, rename, and supported directory sync, preserving the previous snapshot on write/sync/rename failure; add clock-injected token-bucket output throttling for `cache_save_rate_limit` outside all cache locks.
- [x] 4.6 Add deterministic filesystem fault tests for create, write, short write, sync, chmod/permission, rename, and directory-sync failures plus interrupted temporary-file cleanup.

## 5. Model compatibility fingerprints

- [x] 5.1 Define the canonical fingerprint input covering model/tokenizer content and all output-affecting settings while excluding worker, thread, batching, and execution scheduling settings.
- [x] 5.2 Compute fingerprints only when persistence is configured, reuse model/tokenizer bytes already read during lazy/preloaded initialization where possible, and expose registry hooks without loading lazy models solely for restore.
- [x] 5.3 Add unit tests proving every included setting/artifact change invalidates the fingerprint and every deliberately excluded scheduling setting preserves it.
- [x] 5.4 Implement per-model quarantine/admission for lazy-model restored entries and tests proving entries cannot serve before exact fingerprint validation.

## 6. Snapshot coordinator and complete configuration

- [x] 6.1 Add YAML and CLI fields/defaults/validation for `cache_file`, `cache_load`, `cache_save`, `cache_save_on_shutdown`, `cache_restore_limit`, `cache_restore_reserve`, and `cache_save_rate_limit`, including sizes, percentages, durations, rates, booleans, and contradictory combinations.
- [x] 6.2 Implement an optional server-owned snapshot coordinator that is nil when `cache_file` is empty and uses a single-flight state machine for manual, periodic, and shutdown saves.
- [x] 6.3 Implement `CONFIG GET` for every persistence setting; implement live `CONFIG SET` for file, interval, shutdown-save, and save-rate controls; reject load/restore memory controls as read-only/restart-required.
- [x] 6.4 Implement live scheduler/rate replacement using injectable clocks and timers, coalescing ticks during active or clean saves and tracking skipped ticks without an unbounded queue.
- [x] 6.5 Add deterministic coordinator/config tests for defaults, parsing, acceptance, disabled triggers, overlap rejection, timer/rate replacement, dirty-generation coalescing, enable/disable races, read-only settings, status transitions, and goroutine cleanup without wall-clock sleeps.
- [x] 6.6 Add structural tests proving empty `cache_file` creates no coordinator, timer, goroutine, fingerprint work, filesystem call, serialization call, callback, or additional cache operation.
- [x] 6.7 Add gated availability tests proving automatic capture/encode/write/sync/rename and injected failures never change readiness, close connections, consume inference admission slots, reject commands, or make requests wait on post-capture work.

## 7. Restore and server lifecycle integration

- [x] 7.1 Resolve restore admission as the minimum of cache budget, `cache_restore_limit`, and sampled total-RAM/current-RSS headroom after `cache_restore_reserve`, with a conservative observable fallback when host metrics are unavailable.
- [x] 7.2 Stream restore into one unpublished staging cache whose entries, overhead, and lazy-model quarantine share the effective memory ceiling; verify checksum before O(1) publication and release all staging memory on failure.
- [x] 7.3 Admit compatible entries MRU-first within the effective ceiling, skipping unknown/incompatible/oversized/out-of-headroom entries by reason without changing hit/miss/eviction/flush counters.
- [x] 7.4 Integrate configurable dirty-cache final saves after inference drain and before resource close, reusing an active save and honoring `cache_save_on_shutdown` plus the remaining shutdown context deadline.
- [x] 7.5 Add lifecycle integration tests for load disabled, missing/valid/corrupt/partially compatible restores, constrained live host memory, unavailable host metrics, smaller budgets, staging checksum failure, lazy validation, clean/disabled shutdown skip, dirty shutdown save, active-save reuse, save failure, and forced timeout.
- [x] 7.6 Run lifecycle and coordinator tests repeatedly under `go test -race -count=20` inside `nix develop` to catch leaks, deadlocks, and order-dependent failures.

## 8. RESP commands and observability

- [x] 8.1 Register authenticated `EMB.CACHE.FLUSH [model]` with strict arity, unknown-model validation, integer removed-count replies, and disabled-cache idempotence.
- [x] 8.2 Register authenticated asynchronous `EMB.SAVE` with strict arity, prompt `OK` acceptance, disabled-persistence error, and deterministic overlap error.
- [x] 8.3 Extend `EMB.HELP`, `EMB.STATS`, and `INFO cache` with lifecycle fields, effective trigger/rate settings, sampled host memory, effective restore limit, and skip reasons using atomically published status and exact RESP array count parity.
- [x] 8.4 Add wire-level RESP2 tests for command arity, replies, authentication, pipelining, disabled modes, overlap, metrics transitions, HELP output, and STATS count parity in every cache/persistence state.
- [x] 8.5 Add Ruby client `cache_flush(model = nil)` and `save_cache` wrappers with unit/integration tests that preserve raw server errors and work through pooled/round-robin connections.

## 9. Performance acceptance and documentation

- [x] 9.1 Run the disabled-persistence benchmarks against the captured baseline with identical Nix environment and repetitions; fail the change for any added allocation or median regression above 2 percent instead of relaxing the gate without documented evidence.
- [x] 9.2 Run the enabled active-save/streaming-restore harness at representative full-cache and host-memory tiers, both unlimited and rate-limited; require throughput regression at or below 3 percent, p50 at or below 5 percent, p99 at or below 10 percent, and peak restore RSS within the effective ceiling plus documented fixed overhead; capture profiles proving encoding/filesystem work stays off request goroutines.
- [x] 9.3 If either performance gate fails, optimize or redesign snapshot capture/coordinator internals and repeat baseline comparison before proceeding.
- [x] 9.4 Document the RDB-inspired but non-RDB-compatible storage design, every persistence setting/default/live-update rule, command syntax, asynchronous completion/status polling, startup/periodic/shutdown behavior, host-memory ceiling, I/O throttling, format compatibility, and operational recovery in the project and Ruby READMEs.
- [x] 9.5 Document that snapshot files contain original input text and embedding bytes, use owner-only permissions, require protected/encrypted storage as appropriate, and are disposable rather than a durable database.
- [x] 9.6 Run `just format`, `just lint`, `just test`, focused race tests, Ruby unit/integration checks with the server on port 16379, and `just all` inside `nix develop`; record the final verification results with the benchmark evidence.
