# lru-cache Specification

## Purpose
TBD - created by archiving change 2026-07-02-emb-lru-cache. Update Purpose after archive.
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

The cache SHALL store embeddings keyed by `"<model>:<text>"` and return them on subsequent requests for the same `(model, text)` pair.

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

When the cache reaches its byte budget, the least recently used entries SHALL be evicted.

#### Scenario: Eviction on insert

- **GIVEN** the cache is at its byte budget
- **WHEN** a new text is embedded
- **THEN** the LRU entry SHALL be evicted before the new entry is stored
- **THEN** the eviction counter SHALL be incremented

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
Administrative invalidation and snapshot capture SHALL use the existing `"<model>:<text>"` key identity and SHALL NOT change the observable hit, miss, insert, byte-budget, or least-recently-used eviction behavior of entries that remain in the cache.

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

