## Context

`emb` keeps embeddings in a mutex-protected, byte-bounded LRU keyed by `model:text`. `EMB` and `EMB.MULTI` perform synchronous cache lookups and inserts, while cache resizing and statistics share the same lock. The cache has no clearing API and is discarded on every process exit. `CONFIG GET/SET` already exposes `cache_file` and `cache_save`, but those values are currently stored without behavior.

The change crosses the RESP command layer, cache internals, registry identity, startup/readiness, shutdown, INFO/STATS, configuration, and the Ruby wrapper. Cache hits are orders of magnitude faster than inference and are sensitive to even small additions, so the existing lookup path must remain unchanged when persistence is disabled. Absolute zero resource contention while an enabled snapshot is actively copying data is not physically possible; the enforceable contract is structural dormancy when disabled, no filesystem work on request goroutines, bounded cache-lock ownership during capture, and no statistically significant throughput or tail-latency regression under the defined benchmark gates.

This is similar to Redis RDB at the lifecycle level: it writes a compact point-in-time binary snapshot to a temporary file and atomically replaces the prior file, then reconstructs an in-memory cache at startup. It deliberately does **not** copy Redis's internal storage encodings, RDB wire format, fork-based `BGSAVE`, or durability guarantees. Forking a multithreaded Go process that embeds CGo/ONNX Runtime is unsafe, and emb's values and eviction semantics are simpler than Redis data types. The design therefore uses a Go-native streaming codec and asynchronous coordinator while retaining an RDB-like disposable-snapshot model.

## Goals / Non-Goals

**Goals:**

- Provide authenticated whole-cache and model-scoped invalidation.
- Persist and restore warm cache entries through a disposable, versioned snapshot.
- Make manual, periodic, startup, and shutdown lifecycle behavior deterministic and observable.
- Make startup load, periodic save, shutdown save, restore memory allowance, host-memory reserve, and save I/O rate independently configurable.
- Reject incompatible or corrupt data without risking incorrect embeddings or preventing startup.
- Preserve the exact current `model:text` lookup path when persistence is disabled.
- Keep encoding and all filesystem I/O off inference/request goroutines and outside the cache mutex.
- Establish unit, race, lifecycle, integration, fault-injection, and benchmark regression gates.

**Non-Goals:**

- Distributed or shared cache coherence across `emb` instances.
- Durable write-through persistence or a guarantee that every recent insertion survives a crash.
- Encryption, compression, remote object storage, incremental snapshots, or cross-endian conversion.
- Dynamic model reload or migration of entries between different embedding functions.
- Persisting hit/miss/eviction counters; restored entries start with fresh process-local counters.
- Promise of unchanged latency while an operator is actively flushing a large cache. Control operations are rare, explicit mutations and are measured separately.
- Redis RDB compatibility, append-only persistence, replication, or recovery of every acknowledged cache insertion.

## Decisions

### 1. Keep cache administration separate from eviction

`EMB.CACHE.FLUSH` clears all entries and `EMB.CACHE.FLUSH <model>` clears entries with the model prefix. It returns the number of removed entries. A missing configured model is an error; a configured model with no cached entries returns zero; a disabled cache also returns zero. Flushes do not increment eviction counters because they are administrative invalidations, but their count and removed-entry total are reported separately.

Whole-cache flush replaces the list and entry map under the existing mutex in O(1), resets per-model live entry counts, and preserves cumulative cache counters. Model flush scans the existing map/list under the mutex and therefore costs O(total entries), deliberately putting complexity on the rare control operation instead of maintaining another hot-path index. A cache insertion that acquires the mutex after a flush is a post-flush insertion, even if its inference began earlier; dynamic model replacement is out of scope, so this does not reintroduce an incompatible vector.

Alternatives considered:

- Per-model linked lists make scoped flush O(model entries) but add pointers and mutation work to every hit/insert/eviction.
- Epochs prevent pre-flush in-flight work from repopulating the cache but add atomic reads and identity data to every request. This is unnecessary without dynamic model replacement.
- Returning `OK` matches Redis flush commands, but returning an integer makes the operation directly auditable and easy to test.

### 2. Use an asynchronous single-flight snapshot coordinator

A server-owned coordinator is created only when `cache_file` is non-empty. With no file configured, no coordinator, goroutine, timer, channel, fingerprinting, or snapshot callback exists. `EMB.SAVE` checks configuration, atomically claims the single-flight slot, schedules one save, and returns `OK` once accepted. If persistence is disabled it returns an explanatory error; if another save is active it returns `ERR snapshot already in progress`.

The coordinator owns periodic scheduling, status, and shutdown waiting. `cache_save` is empty to disable periodic saves or a positive Go duration such as `5m`; live `CONFIG SET` replaces the timer without creating multiple loops. Each periodic tick is coalesced: if a save is active the tick increments a skipped counter and does not queue another save. Manual saves receive the same single-flight treatment.

Persistence uses explicit orthogonal settings rather than one overloaded mode:

| Setting | Default | Meaning | Live-settable |
|---|---:|---|---|
| `cache_file` | empty | Snapshot path; empty disables all persistence machinery | yes |
| `cache_load` | `true` | Attempt startup restore when a file is configured | no; next restart |
| `cache_save` | empty | Periodic interval; empty disables periodic saves | yes |
| `cache_save_on_shutdown` | `true` | Save a dirty cache during graceful shutdown | yes |
| `cache_restore_limit` | `auto` | Maximum restored entry bytes: `auto`, bytes, or percent of total RAM | no; next restart |
| `cache_restore_reserve` | `10%` | Host memory that restore must leave unused: bytes or percent of total RAM | no; next restart |
| `cache_save_rate_limit` | `0` | Maximum encoded bytes per second; `0` means unlimited | yes |

All settings are accepted in YAML and by corresponding CLI flags and are visible through `CONFIG GET`. Only settings with meaningful safe runtime semantics accept `CONFIG SET`; restart-only controls return the existing read-only/restart-required error. Integrity checks, atomic replacement, strict length validation, and owner-only permissions are safety invariants rather than optional features and cannot be disabled.

Alternatives considered:

- A blocking `EMB.SAVE` gives immediate completion knowledge but ties a RESP connection and request goroutine to disk latency.
- An unbounded save queue can accumulate stale work under slow disks.
- A permanently running loop with disabled configuration violates the zero-overhead default contract.
- A single `cache_persistence` mode string is concise but makes independent deployment policies difficult and creates ambiguous live-update transitions.

### 3. Capture a shallow immutable view, then perform expensive work outside the lock

Snapshot capture takes the cache mutex only long enough to traverse LRU order and copy immutable `(key, []byte reference)` descriptors into a pre-sized slice. Cache keys are immutable strings and embedding byte slices are never mutated after publication; replacing an entry assigns a new slice, leaving a captured old slice valid. Encoding, checksum calculation, file creation, writes, sync, rename, and directory sync occur after releasing the mutex.

The capture preserves most-recent-to-least-recent order. Its critical section is O(entries) but copies no embedding payload bytes. A capture-lock-duration metric is recorded. Benchmark tests populate the maximum representative cache used by CI and enforce the performance thresholds; a future chunked/RCU snapshot can replace capture internally without changing the file or command contract if the gate fails.

This choice avoids permanent request-path overhead. Maintaining a continuously mirrored snapshot map, mutation journal, cache generations, or copy-on-write values would reduce capture pauses but would add allocation, hashing, atomic, or map work to every hit/insert. That conflicts with the higher-priority default performance requirement.

Automatic snapshot state is never a server lifecycle state: it does not change `EMB.READY`, close connections, consume the inference concurrency gate, or reject/delay admission with `ERR busy`. `EMB`, `EMB.MULTI`, cache misses, cache hits, INFO, CONFIG, AUTH, and health commands remain available. A request can wait briefly for the existing cache mutex during descriptor capture, bounded and measured by the active-save latency gates, but can never wait for encoding, rate limiting, file I/O, checksum completion, or atomic rename. A snapshot failure changes only snapshot status and leaves both inference and the live cache operational.

### 4. Use a compact, versioned binary file with whole-payload integrity

The file contains:

```text
magic | format-version | header-length | payload-length
model fingerprint table
entry count
entries in MRU→LRU order: model, text key, embedding bytes
SHA-256 checksum of header+payload
```

All lengths are fixed-width little-endian integers and are validated before allocation. The encoder streams records through a fixed-size buffered writer and checksum writer directly to the temporary file; it never builds a second full payload in memory. `cache_save_rate_limit` optionally applies a token-bucket delay between buffered writes, outside every cache/request lock. The format records endianness and float width even though version 1 accepts only little-endian fp32. No compression is used, favoring predictable CPU and memory cost and simple corruption behavior.

The temporary file is created in the destination directory with mode `0600`, fully written and synced, then atomically renamed over the target. The parent directory is synced where supported. Any failure leaves the last complete snapshot untouched; temporary files from interrupted writes may be removed on the next save/startup. Snapshot bytes include raw input text because changing request keys to hashes would tax every cache access. Documentation treats the file like application input data and recommends an encrypted volume when required.

Alternatives considered:

- JSON is inspectable but inflates binary embeddings and parse/allocation costs.
- Gob couples persistence to Go type details and has weaker long-term compatibility control.
- Compression reduces disk usage but introduces CPU contention during active saves.
- Redis `fork()`/copy-on-write gives RDB a stable child-process view, but is unsafe around Go runtime threads and CGo/ONNX state and can transiently consume substantial host memory when pages are dirtied.

### 5. Validate entries against an embedding-function fingerprint

A cache entry is reusable only when its model fingerprint matches. The fingerprint covers model weight content, tokenizer content, output tensor, dimension, max length, pooling, normalization, pad-output, and quantization resolution. Thread-count, batching, worker-count, and execution-mode settings are excluded because they do not intentionally change the embedding function.

Fingerprint work exists only when persistence is configured. Weight and tokenizer hashes are computed from bytes already read during model initialization where possible. For lazy models, restored records remain quarantined by model until its fingerprint is available; the first request loads the model normally, validates that model's records, admits compatible records up to the cache budget, and discards the rest. This avoids reading every large ONNX file solely to restore cache data at startup and prevents snapshot support from changing the disabled lazy-load path.

Snapshots may contain entries only for models whose fingerprint is known at save time. Entries for a configured but never-loaded lazy model cannot exist and therefore need no fingerprint. Unknown models, dimension mismatches, fingerprint mismatches, duplicate keys, and entries individually larger than the budget are skipped and counted.

### 6. Restore streams into a host-memory-bounded staging cache

At startup, `cache_load=true` with a configured file parses the snapshot before `SetReady`; `cache_load=false` skips all open/read/fingerprint/restore work. The decoder reads through a fixed-size buffer and checksum reader and never calls `os.ReadFile` or retains the entire encoded file. It inserts candidate records into a private staging cache that is invisible to requests until the checksum and structural validation succeed, then swaps the staging storage into the live cache in O(1). Corrupt staging state is discarded without exposing a single entry.

Before allocating entries, restore samples total host RAM and current process RSS. Its effective admitted-entry limit is:

```text
min(
  configured cache byte budget,
  resolved cache_restore_limit,
  max(0, total_host_ram - current_process_rss - resolved cache_restore_reserve)
)
```

`cache_restore_limit=auto` resolves to the configured cache budget; an explicit byte size or percentage can lower it but cannot override the cache budget or host-headroom ceiling. `cache_restore_reserve` defaults to 10% of total host RAM and accepts a fixed byte size or percentage. On platforms where total RAM or RSS cannot be sampled, restore uses the stricter configured cache/restore limit, logs that host headroom could not be verified, and exposes that fact in status. The staging cache's own entry accounting includes keys, embeddings, and per-entry overhead. Decoder buffers, descriptor tables, quarantine metadata, and checksum state have fixed or separately bounded overhead; large declared record counts cannot cause proportional preallocation.

Compatible entries are processed in MRU-to-LRU order and admitted only while the effective limit permits, so the most valuable working set wins. If fingerprints for lazy models are not yet available, their staged entries remain quarantined but still count fully against the same effective memory limit; they never form an unaccounted second cache. Restore does not count as hits, misses, evictions, or flushes. A successfully parsed file may partially restore when only some models fit or match; restored/skipped-by-reason counts make that visible.

A missing file is normal. Unsupported versions, checksum failures, truncation, invalid lengths, permission/read errors, insufficient safe headroom, and all-entry incompatibility are logged and reflected in status, but the server starts with an empty or safely partial cache. Restore stops admitting entries as soon as its effective limit is reached while continuing bounded streaming validation of the remaining file so the checksum is still authoritative.

### 7. Shutdown saves are bounded and preserve graceful-drain priority

Once the server enters draining state, it stops accepting new inference work and waits for active requests as today. If persistence is configured, `cache_save_on_shutdown=true`, and the cache is dirty, shutdown requests a final snapshot or waits for the active snapshot. The wait uses the remaining shutdown context deadline. Timeout or save failure is logged and counted but does not prevent server termination. An unchanged cache may skip the final write using a mutation generation captured at the last successful save.

No disk operation occurs while active inference is being drained from the cache lock. The final capture starts after request draining, eliminating request latency impact in the normal shutdown path.

### 8. Observability extends existing RESP surfaces without changing old fields

`EMB.STATS` and `INFO cache` append fields for administrative flush count/entries; effective persistence settings; snapshot enabled/in-progress state; successful/failed/skipped counts; last success timestamp, duration, captured entries/bytes; restore effective limit, sampled RSS/headroom, restored/skipped-by-reason counts; and capture-lock duration. Existing field names, values, and order remain unchanged where clients may depend on them. `EMB.HELP` lists both commands. The Ruby client exposes `cache_flush(model = nil)` and `save_cache`; existing generic stats parsing automatically accepts appended fields.

### 9. Performance is a release gate

Tests establish three layers:

1. Structural tests assert a server with empty `cache_file` has no coordinator/timer and does not call snapshot or fingerprint hooks during cache operations.
2. Go benchmarks compare cache `Get`, `Set`, cached `EMB`, and uncached `EMB` before/after with persistence disabled. The median of repeated runs must remain within noise: no more than 2% throughput regression and no new allocation per operation. Any failure blocks the change rather than relaxing the threshold without documented evidence.
3. An enabled-persistence benchmark runs periodic/manual saves over representative full caches at several host-memory tiers while driving hits and misses. It measures unthrottled and configured rate-limited saves plus streaming restore peak RSS. Compared with the same cache and no saves, throughput must not regress more than 3%, p50 more than 5%, or p99 more than 10%; snapshot encoding/I/O must never appear on request goroutine stacks, and restore peak RSS must stay within the computed effective limit plus documented fixed overhead. Results are captured in `BENCHMARK.md` with environment and raw benchstat/load output.

Concurrency tests run under `go test -race` and coordinate with gates/channels rather than sleeps. Filesystem fault tests use injected file operations, not flaky disk filling or permission timing.

## Risks / Trade-offs

- **[Large model-scoped flush holds the cache mutex]** → Keep the hot path unchanged, measure flush duration, use whole-cache O(1) reset where possible, and document scoped flush as an explicit maintenance operation.
- **[Snapshot capture briefly blocks cache operations]** → Shallow-copy only descriptors, move payload encoding/I/O outside the lock, record capture duration, and enforce active-save p99 gates.
- **[Snapshot exposes original texts]** → Make persistence opt-in, use `0600`, document the file as sensitive, avoid logs containing keys, and recommend encrypted storage. Encryption can be a later capability.
- **[Stale vectors are restored]** → Require complete output-affecting model fingerprints and skip any mismatch rather than guessing compatibility.
- **[Corrupt files cause memory exhaustion]** → Validate all counts/lengths against cache-derived bounds before allocation and verify the checksum.
- **[Restore exhausts host memory despite the configured cache]** → Stream into one staging cache, include entry overhead, cap admission by live RSS/headroom plus restore limit/reserve, bound non-entry structures, and publish only after checksum success.
- **[Periodic saves contend for CPU/I/O]** → Single-flight coalescing, no compression, asynchronous work, and benchmark gates; operators can leave `cache_save` empty and invoke manual/shutdown saves only.
- **[Too many tuning knobs create misconfiguration]** → Keep safe defaults, expose effective resolved values in INFO, reject contradictory combinations, and never allow a knob to disable integrity, atomic replacement, memory ceilings, or permissions.
- **[Shutdown exceeds its deadline]** → Honor the existing context deadline and treat the snapshot as best-effort; the previous atomic snapshot remains usable.
- **[Snapshot format becomes a compatibility burden]** → Declare it versioned and disposable, reject unknown versions, and make no promise of indefinite cross-major-version migration.

## Migration Plan

1. Ship command and snapshot support with empty `cache_file`/`cache_save` defaults, leaving existing deployments structurally unchanged.
2. Deploy with `cache_file` set, `cache_load=true`, periodic saving disabled, the default host reserve, and shutdown saving disabled; invoke `EMB.SAVE`, verify status and file permissions, then restart and confirm restore limit/headroom metrics and hits.
3. Enable `cache_save_on_shutdown`, then a conservative `cache_save` interval and `cache_save_rate_limit` only after observing snapshot duration, peak RSS, and workload benchmarks on production-sized caches.
4. Roll back by clearing `cache_file` and `cache_save`; the server ignores the disposable snapshot and resumes the previous in-memory-only behavior. Older binaries are unaffected by the extra file.

## Open Questions

- The concrete CI cache size and repetition count for the 3%/5%/10% active-save gate should be calibrated once on both macOS ARM64 and the gold Linux ARM64 environment, without weakening the stated ceilings.
- Whether `EMB.SAVE` should return a generated snapshot sequence identifier in a later version; version 1 uses `OK` plus INFO/STATS status to keep RESP2 simple.
- Whether future encryption should be application-managed at the filesystem/volume layer or introduced as an explicit key-bearing snapshot format version.
