# lru-cache Specification

## Purpose
The embedding cache: a byte-budgeted LRU store of text and image embeddings so repeated requests skip inference, with hit/miss/eviction metrics, model-scoped invalidation, and snapshot persistence. The cache retains only what fits its budget and never serves a value that no longer matches the model that produced it.
## Requirements
### Requirement: Cache configuration

The server SHALL accept a cache configuration via the YAML `cache` field or the `-cache` CLI flag. The value SHALL be a string: empty (disabled), `"auto"` (auto-tune), a human-readable size (e.g., `"1GB"`, `"256MB"`), or a percentage of total system RAM (e.g., `"25%"`).

#### Scenario: Default is disabled

- **WHEN** neither `cache` in YAML nor `-cache` flag is provided
- **THEN** the cache SHALL be nil (no memory allocated, no overhead)

#### Scenario: Auto-tune from system memory

- **WHEN** `cache: "auto"` is set
- **THEN** the cache SHALL use `totalSystemMemory()` to estimate available memory
- **THEN** the budget SHALL be 20% of remaining memory after a 10% safety margin and a 25% model reserve (~13% of total RAM)
- **THEN** the budget SHALL be floored at 64MB and capped at 50% of total RAM
- **THEN** no fixed byte ceiling (e.g., 500MB) SHALL be applied

#### Scenario: Explicit size

- **WHEN** `cache: "1GB"` is set
- **THEN** the cache SHALL parse `"1GB"` via `docker/go-units.FromHumanSize()`
- **THEN** the cache budget SHALL be the parsed byte value

#### Scenario: Percentage size

- **WHEN** `cache: "25%"` is set
- **THEN** the cache budget SHALL be 25% of total system RAM (explicit operator choice, no auto margin applied)

#### Scenario: Invalid size

- **WHEN** `cache: "invalid"` or `cache: "150%"` or `cache: "0%"` is set
- **THEN** the server SHALL fail to start with a clear error message

### Requirement: Cache behavior

The cache SHALL store text embeddings keyed by `"txt:<model>:<sha256(text)>"` and return them on subsequent requests for the same `(model, text)` pair.

#### Scenario: Cache hit returns immediately

- **GIVEN** the cache contains an embedding for `(minilm, "hello world")`
- **WHEN** a client sends `EMB minilm hello world`
- **THEN** the server SHALL return the cached embedding without calling `GetOrInit` or `Pool.Embed`

#### Scenario: Cache miss runs inference and stores

- **GIVEN** the cache is empty
- **WHEN** a client sends `EMB minilm hello world`
- **THEN** the server SHALL run inference normally
- **THEN** the resulting embedding SHALL be stored in the cache
- **THEN** a subsequent `EMB minilm hello world` SHALL be a cache hit

#### Scenario: Partial hit with multiple texts

- **GIVEN** the cache contains an embedding for `(minilm, "hello")`
- **WHEN** a client sends `EMB minilm hello world`
- **THEN** the server SHALL return the cached `"hello"` embedding immediately
- **THEN** the server SHALL run inference for `"world"` only
- **THEN** both embeddings SHALL be returned in the correct order

#### Scenario: Cache hit on EMB.MULTI

- **GIVEN** the cache contains an embedding for `(minilm, "hello")`
- **WHEN** a client sends `EMB.MULTI minilm hello bge "some text"`
- **THEN** the `minilm:"hello"` embedding SHALL be served from cache
- **THEN** the `bge:"some text"` embedding SHALL run inference normally

### Requirement: Eviction

When the cache reaches its byte budget, the least recently used entries SHALL be evicted. After every successful insert or replacement with a positive configured budget, accounted live entry bytes SHALL NOT exceed that budget. Entry admission SHALL include the key, value, and existing per-entry accounting overhead. Values too large to fit by themselves SHALL be rejected before copying their payload or evicting any existing entry. Rejected writes SHALL leave values, LRU order, byte accounting, counters, and mutation generation unchanged. Existing non-positive internal budget behavior SHALL remain unchanged.

#### Scenario: Eviction on insert

- **GIVEN** the cache is at its byte budget
- **WHEN** a new text is embedded and its entry fits the budget by itself
- **THEN** the necessary LRU entries SHALL be evicted before the new entry is stored
- **AND** the eviction counter SHALL be incremented for each removed entry

#### Scenario: Oversized insert preserves useful entries

- **GIVEN** a positive cache budget and existing entries
- **WHEN** a new key and value exceed that budget including accounting overhead
- **THEN** the write SHALL be skipped before allocating an owned payload copy
- **AND** all existing entries, order, accounting, counters, and generation SHALL remain unchanged

#### Scenario: Oversized replacement preserves the old value

- **GIVEN** a cached key under a positive budget
- **WHEN** its proposed replacement cannot fit the budget by itself
- **THEN** the original value and recency SHALL remain unchanged
- **AND** no eviction, accounting, counter, or generation change SHALL occur

#### Scenario: Growing replacement evicts other least-recent entries

- **GIVEN** a cached key whose replacement fits the budget by itself but exceeds the remaining free space
- **WHEN** the replacement is stored
- **THEN** the replacement SHALL become most recently used
- **AND** other least-recent entries SHALL be evicted until live accounted bytes fit the budget
- **AND** byte, per-model, eviction, and mutation-generation accounting SHALL reflect the actual mutations

#### Scenario: Exact-fit and shrinking writes

- **WHEN** a write makes accounted bytes equal the positive budget, or replaces a value with a smaller one without exceeding the budget
- **THEN** the write SHALL succeed without unnecessary eviction
- **AND** previously captured snapshot values SHALL remain immutable

### Requirement: Metrics

The server SHALL expose cache statistics.

#### Scenario: EMB.INFO shows cache stats

- **WHEN** a client sends `EMB.INFO minilm`
- **THEN** the response SHALL include `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, `cache_memory_bytes`

#### Scenario: EMB.STATS includes cache totals

- **WHEN** a client sends `EMB.STATS`
- **THEN** the response SHALL include aggregate cache stats across all models

### Requirement: No cache when disabled

When cache is not configured, the server SHALL behave identically to today.

#### Scenario: Cache disabled, normal operation

- **WHEN** cache is not configured
- **THEN** all `EMB` and `EMB.MULTI` commands SHALL work without any cache overhead
- **THEN** `EMB.INFO` SHALL not include cache fields (or show zeros)

### Requirement: Cache invalidation preserves core LRU semantics
Administrative invalidation and snapshot capture SHALL use the existing `"txt:<model>:<sha256(text)>"` key identity and SHALL NOT change the observable hit, miss, insert, byte-budget, or least-recently-used eviction behavior of entries that remain in the cache.

#### Scenario: Scoped invalidation preserves remaining order
- **GIVEN** interleaved LRU entries for two models with known recency order
- **WHEN** all entries for one model are invalidated
- **THEN** the remaining model's relative recency order SHALL be unchanged
- **AND** its next insertion SHALL evict the same least-recent entry it would have evicted if the removed entries had never existed

### Requirement: Snapshot views preserve immutable published values
The cache SHALL permit a snapshot view to retain references to published embedding byte slices after releasing the cache mutex. Cache operations SHALL never mutate a published byte slice in place; replacement SHALL publish a new slice, and removal SHALL not invalidate an existing snapshot reference.

#### Scenario: Replace and evict during snapshot encoding
- **GIVEN** a snapshot view has captured an entry value
- **WHEN** the live cache replaces or evicts that key while encoding is gated
- **THEN** the snapshot view SHALL retain the originally captured bytes without a race
- **AND** the live cache SHALL contain the replacement or absence independently

### Requirement: Cache mutation generation
The cache SHALL maintain a monotonic mutation generation for successful inserts, replacements, evictions, resizes that evict, restores, and flushes. Reading or updating this generation SHALL reuse the cache's existing mutation critical section and SHALL NOT add synchronization to cache hits beyond the existing lock.

#### Scenario: Successful save records a clean generation
- **GIVEN** a snapshot captured cache generation N
- **WHEN** that snapshot completes successfully and no later mutation occurred
- **THEN** shutdown SHALL recognize the cache as clean and skip a redundant save
- **WHEN** a later mutation advances the generation
- **THEN** shutdown SHALL recognize the cache as dirty

### Requirement: Image embedding cache entries

The cache SHALL store image embeddings keyed by `img:<model>:<content-hash>`, where `<content-hash>` is derived from the source image bytes. Image entries SHALL participate in the same byte budget, LRU eviction, hit/miss/eviction statistics, snapshot capture, and mutation-generation accounting as text entries. Model-scoped invalidation (`EMB.CACHE.FLUSH <model>`) SHALL remove a model's image entries as well as its text entries, and SHALL NOT remove other models' image entries.

#### Scenario: Image entry is cached and hits

- **WHEN** the same image bytes are embedded twice for the same model
- **THEN** the second request is a cache hit under the same `img:<model>:<content-hash>` key

#### Scenario: Model-scoped flush removes image entries

- **WHEN** a client sends `EMB.CACHE.FLUSH <model>`
- **THEN** that model's image entries are removed and other models' entries are retained

#### Scenario: Image entries obey the byte budget

- **WHEN** image entries fill the cache byte budget
- **THEN** least-recently-used image and text entries are evicted under the same policy and counted as evictions

### Requirement: Cache stores owned copies of values

`Set` SHALL store an independent copy of the value bytes rather than a caller-owned slice. A stored entry SHALL NOT alias a shared backing buffer (such as a batch-wide embedding buffer whose per-row values are sub-slices), so retaining one small entry never pins a much larger buffer. The byte accounting reported by cache stats SHALL reflect the bytes the cache actually retains.

#### Scenario: Mutating the source does not change the cached value

- **WHEN** a value slice is stored with `Set` and the caller then mutates the same underlying array
- **THEN** a subsequent `Get` SHALL return the bytes as stored, not the mutated bytes

#### Scenario: A single cached row does not retain its batch

- **GIVEN** a batch inference that produced many rows in one backing buffer
- **WHEN** exactly one row is stored in the cache
- **THEN** the memory the cache retains SHALL be proportional to that row, not to the whole batch
