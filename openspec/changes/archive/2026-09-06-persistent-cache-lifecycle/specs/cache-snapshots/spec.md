## ADDED Requirements

### Requirement: Persistence is opt-in and dormant by default
Cache persistence SHALL be enabled only when `cache_file` is non-empty. With persistence disabled, the server SHALL create no snapshot coordinator, goroutine, timer, channel, model-fingerprint work, filesystem access, serialization work, cache callback, or additional cache-lock operation.

#### Scenario: Default server has no persistence machinery
- **WHEN** the server starts with empty `cache_file` and `cache_save`
- **THEN** no snapshot coordinator or scheduler SHALL exist
- **AND** cache hits, misses, inserts, and inference SHALL execute through the same request-path operations as before this change

#### Scenario: Disabled-path performance gate
- **WHEN** repeated cache `Get`, cache `Set`, cached `EMB`, and uncached `EMB` benchmarks compare the implementation against the pre-change baseline
- **THEN** no benchmark SHALL add an allocation per operation
- **AND** median throughput regression for each benchmark SHALL NOT exceed 2 percent

### Requirement: Persistence lifecycle controls are independently configurable
The server SHALL expose `cache_file`, `cache_load`, `cache_save`, `cache_save_on_shutdown`, `cache_restore_limit`, `cache_restore_reserve`, and `cache_save_rate_limit` through YAML, CLI flags, and `CONFIG GET`. `cache_file`, `cache_save`, `cache_save_on_shutdown`, and `cache_save_rate_limit` SHALL be live-settable; startup-only settings SHALL be reported as read-only/restart-required by `CONFIG SET`. Integrity checking, atomic replacement, allocation bounds, and owner-only file permissions SHALL remain mandatory safety invariants.

#### Scenario: Disable each automatic lifecycle trigger
- **GIVEN** `cache_file` is configured
- **WHEN** `cache_load=false`, `cache_save` is empty, and `cache_save_on_shutdown=false`
- **THEN** startup SHALL not read the file
- **AND** no periodic or shutdown save SHALL occur
- **AND** authenticated manual `EMB.SAVE` SHALL remain available

#### Scenario: Effective configuration is inspectable
- **WHEN** a client sends `CONFIG GET cache*` and `INFO cache`
- **THEN** all configured raw persistence values SHALL appear in CONFIG
- **AND** all resolved byte limits, host-memory samples, enabled triggers, and rate limits SHALL appear in INFO

### Requirement: Manual asynchronous snapshot
`EMB.SAVE` SHALL be an authenticated control command that asynchronously saves the current cache to `cache_file`. It SHALL return `OK` after one save is accepted, return an error when `cache_file` is empty, and return `ERR snapshot already in progress` when another manual, periodic, or shutdown save owns the single-flight slot.

#### Scenario: Manual save accepted
- **GIVEN** cache persistence is configured and no save is active
- **WHEN** an authenticated client sends `EMB.SAVE`
- **THEN** it SHALL promptly receive `OK` without waiting for filesystem completion
- **AND** exactly one background save SHALL begin

#### Scenario: Disabled and overlapping saves rejected
- **WHEN** `EMB.SAVE` is sent with empty `cache_file`
- **THEN** it SHALL return an explanatory error and create no file or goroutine
- **WHEN** `EMB.SAVE` is sent while another save is active
- **THEN** it SHALL return `ERR snapshot already in progress` and SHALL NOT queue another save

### Requirement: Atomic and integrity-checked snapshot file
Snapshots SHALL use a documented versioned binary format containing magic, format version, bounded lengths, model fingerprints, MRU-to-LRU entries, fp32/endianness metadata, and a SHA-256 integrity checksum. A save SHALL create an owner-only temporary file in the destination directory, write and sync it, atomically rename it over the target, and sync the parent directory where supported.

#### Scenario: Successful atomic replacement
- **GIVEN** a previous complete snapshot exists
- **WHEN** a new save completes successfully
- **THEN** readers SHALL observe either the previous complete file or the new complete file, never a partial target
- **AND** the final file mode SHALL prohibit group and other access

#### Scenario: Injected write, sync, or rename failure
- **WHEN** deterministic injected filesystem operations fail during a save
- **THEN** the previous complete target SHALL remain readable and unchanged
- **AND** the failure SHALL be logged and counted
- **AND** no request goroutine SHALL block on that filesystem operation

### Requirement: Snapshot capture isolates request handling from I/O
Snapshot capture SHALL copy only immutable key and embedding-slice descriptors while holding the existing cache mutex. Encoding SHALL stream through fixed-size buffers without constructing a duplicate full payload. Payload copying, hashing, file creation, writes, sync, rename, directory sync, and configured I/O-rate delays SHALL occur after releasing the mutex and outside RESP request goroutines. Cache byte slices published to a snapshot view SHALL not be mutated.

#### Scenario: Filesystem is gated during active traffic
- **GIVEN** an accepted save is blocked by a gated filesystem fake after capture
- **WHEN** clients issue cache hits, misses, `EMB`, and `EMB.MULTI`
- **THEN** those operations SHALL complete without waiting for the filesystem gate
- **AND** the cache SHALL remain race-free and within its byte budget

#### Scenario: Active-save performance gate
- **WHEN** manual or periodic saves repeatedly capture and write a representative full cache while concurrent clients drive cache-hit and cache-miss workloads
- **THEN** throughput SHALL regress no more than 3 percent versus the same configured cache without active saves
- **AND** p50 latency SHALL regress no more than 5 percent
- **AND** p99 latency SHALL regress no more than 10 percent
- **AND** snapshot encoding and filesystem frames SHALL not appear on request goroutine stacks

#### Scenario: Save I/O rate limiting
- **GIVEN** `cache_save_rate_limit` is configured to N bytes per second and a fake clock controls rate-limit time
- **WHEN** a snapshot larger than N bytes is written
- **THEN** buffered output SHALL not exceed the configured long-run rate except for one documented buffer burst
- **AND** rate-limit waiting SHALL hold no cache mutex and block no request goroutine

### Requirement: Automatic snapshots preserve server availability
An automatic snapshot SHALL NOT change readiness, stop command admission, close connections, consume an `EMB`/`EMB.MULTI` concurrency slot, or reject inference because a snapshot is active. All inference and control commands SHALL remain operational throughout capture, encoding, rate limiting, writing, syncing, and renaming. Requests MAY contend briefly on the existing cache mutex only during descriptor capture and SHALL never wait for subsequent snapshot work.

#### Scenario: Commands continue throughout a gated automatic save
- **GIVEN** deterministic gates pause an automatic snapshot separately during capture completion, encoding, writing, sync, and rename
- **WHEN** clients concurrently issue `EMB`, `EMB.MULTI`, cache-hit, cache-miss, `PING`, `EMB.READY`, `INFO`, and authenticated `CONFIG GET` commands
- **THEN** every command SHALL complete with its normal non-snapshot semantics
- **AND** `EMB.READY` SHALL continue to report ready
- **AND** no command SHALL wait on any post-capture gate

#### Scenario: Automatic save failure is isolated
- **WHEN** an automatic snapshot fails during encoding or any filesystem operation
- **THEN** live cache contents, readiness, open connections, and inference admission SHALL remain unchanged
- **AND** subsequent inference and control commands SHALL continue normally
- **AND** only snapshot error status and counters SHALL change

### Requirement: Embedding-function compatibility
Every restored entry SHALL be associated with a model fingerprint covering model-weight content, tokenizer content, output tensor, dimension, maximum length, pooling, normalization, pad-output, and resolved quantization. Threading, worker, batching, and execution scheduling settings SHALL be excluded. An entry SHALL be admitted only after its configured model's fingerprint matches exactly.

#### Scenario: Compatible model restores entries
- **GIVEN** a snapshot was created with the same output-affecting model artifacts and settings
- **WHEN** the server restores and validates that model
- **THEN** its retained entries SHALL be available as cache hits with byte-identical embeddings

#### Scenario: Model or tokenizer changes
- **WHEN** model weights, tokenizer bytes, dimension, max length, pooling, normalization, pad-output, output tensor, or resolved quantization differ from the snapshot
- **THEN** every entry for that model SHALL be skipped
- **AND** entries for independently compatible models MAY still restore

#### Scenario: Lazy model validation
- **GIVEN** a snapshot contains entries for a configured lazy model
- **WHEN** the server starts before that model has loaded
- **THEN** those entries SHALL remain quarantined and SHALL NOT serve cache hits
- **WHEN** the model first loads and its fingerprint matches
- **THEN** compatible quarantined entries SHALL be admitted before subsequent lookups

### Requirement: Safe host-memory-bounded streaming restore
When `cache_load=true`, restore SHALL stream through a fixed-size decoder into one unpublished staging cache and SHALL never read or decode the complete file into a second in-memory payload. Its admitted entry bytes, including cache accounting overhead and quarantined lazy-model entries, SHALL not exceed the minimum of the configured cache budget, resolved `cache_restore_limit`, and sampled host headroom after `cache_restore_reserve`. Counts, lengths, and all proportional allocations SHALL be bounded before allocation. The staging cache SHALL become live only after complete structural and checksum validation.

#### Scenario: Missing snapshot
- **WHEN** `cache_file` does not exist at startup
- **THEN** the server SHALL become ready with an empty cache and no restore error

#### Scenario: Startup loading explicitly disabled
- **GIVEN** a valid snapshot exists and `cache_load=false`
- **WHEN** the server starts
- **THEN** it SHALL not open, read, checksum, fingerprint, or restore the snapshot
- **AND** it SHALL become ready with an empty cache

#### Scenario: Corrupt and hostile inputs
- **WHEN** table-driven tests provide bad magic, unknown version, checksum mismatch, truncation at every field boundary, overflowing lengths, excessive entry counts, duplicate keys, wrong dimensions, or an entry larger than the cache budget
- **THEN** parsing SHALL not panic or allocate beyond validated bounds
- **AND** the server SHALL become ready
- **AND** invalid entries SHALL never be served

#### Scenario: Host headroom is lower than snapshot and cache budgets
- **GIVEN** a configured cache budget of 4GB, `cache_restore_limit=3GB`, a 3GB snapshot, and sampled safe host headroom of 700MB after reserve
- **WHEN** restore runs
- **THEN** total admitted entry accounting SHALL not exceed 700MB
- **AND** peak restore RSS SHALL not exceed that admitted memory plus documented fixed decoder and metadata overhead
- **AND** the server SHALL report the effective 700MB limit and skipped-memory entries

#### Scenario: Snapshot checksum fails after staging entries
- **WHEN** a stream contains valid-looking entries but its final checksum is invalid
- **THEN** no staged entry SHALL become visible
- **AND** staging memory SHALL be released before readiness

#### Scenario: Smaller current cache budget
- **GIVEN** a valid snapshot whose entries exceed the current cache budget
- **WHEN** it is restored
- **THEN** the most-recent compatible entries that fit SHALL be retained in original LRU order
- **AND** least-recent overflow entries SHALL be counted as skipped, not evicted

### Requirement: Periodic snapshots are live-configurable and coalesced
An empty `cache_save` SHALL disable periodic saving. A non-empty value SHALL be a positive Go duration and SHALL schedule saves at that interval only while `cache_file` is configured. Live changes to the interval or `cache_save_rate_limit` SHALL replace effective behavior without leaking timers or goroutines. A tick during an active save SHALL be skipped and counted, never queued. A tick SHALL skip writing when the cache mutation generation equals the last successful save generation.

#### Scenario: Timer replacement without sleeps
- **WHEN** a deterministic fake clock advances after `CONFIG SET cache_save` changes the interval
- **THEN** saves SHALL occur only on the new schedule
- **AND** tests SHALL observe one live scheduler with no leaked timer or goroutine

#### Scenario: Slow save coalesces ticks
- **GIVEN** a gated save remains active across multiple periodic ticks
- **WHEN** the fake clock advances through those ticks
- **THEN** no second save SHALL start or queue
- **AND** each coalesced tick SHALL increment the skipped-save counter

### Requirement: Bounded shutdown snapshot
After inference requests drain, graceful shutdown SHALL request a final save only when persistence is configured, `cache_save_on_shutdown=true`, and the cache mutation generation differs from the last successful snapshot. It SHALL wait only within the remaining shutdown context, reuse an already-active save, and terminate even when saving fails or times out.

#### Scenario: Dirty cache saved on graceful shutdown
- **GIVEN** a dirty persistent cache and sufficient shutdown time
- **WHEN** SIGINT or SIGTERM initiates graceful shutdown
- **THEN** active inference SHALL drain first
- **AND** one final atomic snapshot SHALL complete before resources close

#### Scenario: Clean cache skips redundant save
- **GIVEN** no cache mutation since the last successful save
- **WHEN** graceful shutdown begins
- **THEN** no new snapshot SHALL be written

#### Scenario: Shutdown save explicitly disabled
- **GIVEN** a dirty persistent cache and `cache_save_on_shutdown=false`
- **WHEN** graceful shutdown begins
- **THEN** no final save SHALL start
- **AND** shutdown SHALL otherwise follow the normal drain and resource-close sequence

#### Scenario: Shutdown deadline expires
- **GIVEN** a snapshot is blocked beyond the shutdown deadline
- **WHEN** the deadline expires
- **THEN** shutdown SHALL continue and terminate
- **AND** the last complete snapshot SHALL remain valid

### Requirement: Snapshot lifecycle is observable and documented
`INFO cache` and `EMB.STATS` SHALL report snapshot enabled/in-progress state, effective trigger settings, save rate, successes, failures, skipped saves, last success time, last duration, captured entries/bytes, sampled restore RSS/headroom, effective restore limit, restored/skipped-by-reason entries, and capture-lock duration. `EMB.HELP` and project/client documentation SHALL describe `EMB.SAVE`, configuration, asynchronous completion, compatibility, host-memory enforcement, permissions, and the fact that snapshot files contain original input text and embeddings.

#### Scenario: Status transitions are observable
- **WHEN** a gated save starts, completes, and a restore later succeeds
- **THEN** status fields SHALL deterministically reflect in-progress, success, duration/size, and restored-entry transitions
- **AND** all new counters SHALL be non-negative RESP integers where representable

#### Scenario: Snapshot documentation warns about sensitive data
- **WHEN** an operator reads command/config documentation
- **THEN** it SHALL state that `cache_file` contains original texts and embedding bytes
- **AND** it SHALL recommend owner-only permissions and protected or encrypted storage
