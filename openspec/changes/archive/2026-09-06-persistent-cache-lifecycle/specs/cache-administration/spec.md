## ADDED Requirements

### Requirement: Whole-cache invalidation
The server SHALL expose `EMB.CACHE.FLUSH` as an authenticated control command that atomically removes all live cache entries without disabling the cache, changing its byte budget, or resetting cumulative hit, miss, and eviction counters. The reply SHALL be a RESP integer containing the number of removed entries.

#### Scenario: Flush populated cache
- **GIVEN** the cache contains entries for multiple models
- **WHEN** an authenticated client sends `EMB.CACHE.FLUSH`
- **THEN** the reply SHALL equal the total number of entries removed
- **AND** subsequent requests for those entries SHALL miss and run inference
- **AND** the configured cache budget SHALL remain unchanged

#### Scenario: Flush empty or disabled cache
- **WHEN** a client sends `EMB.CACHE.FLUSH` while the cache is empty or disabled
- **THEN** the server SHALL return integer `0`
- **AND** inference behavior SHALL remain unchanged

### Requirement: Model-scoped invalidation
The server SHALL accept `EMB.CACHE.FLUSH <model>` and remove only entries whose cache key belongs to that configured model. It SHALL return the number removed and SHALL reject an unknown model without changing the cache.

#### Scenario: Flush one model
- **GIVEN** the cache contains two entries for `minilm` and three entries for `bge`
- **WHEN** an authenticated client sends `EMB.CACHE.FLUSH minilm`
- **THEN** the reply SHALL be integer `2`
- **AND** the `minilm` entries SHALL miss on their next requests
- **AND** all `bge` entries SHALL remain cache hits in the same LRU order

#### Scenario: Unknown model is rejected atomically
- **WHEN** a client sends `EMB.CACHE.FLUSH nonexistent`
- **THEN** the server SHALL return an error identifying the unknown model
- **AND** no cache entry SHALL be removed

### Requirement: Flush concurrency semantics
Cache invalidation SHALL be race-free and linearizable with cache `Get`, `Set`, eviction, resizing, snapshot capture, `EMB`, and `EMB.MULTI`. A cache insertion that acquires the cache lock after flush completes SHALL be treated as a new post-flush insertion, including when its inference began before the flush.

#### Scenario: Concurrent get, set, flush, resize, and snapshot
- **WHEN** gated tests execute cache hits, misses, inserts, eviction, resizing, snapshot capture, and both flush forms concurrently under the Go race detector
- **THEN** no data race, deadlock, corrupted LRU link, duplicate key, negative entry count, or byte-accounting inconsistency SHALL occur
- **AND** the resulting cache SHALL satisfy its configured byte budget

#### Scenario: In-flight inference completes after flush
- **GIVEN** an inference miss began before a model-scoped flush and has not inserted its result
- **WHEN** the flush completes and the inference subsequently inserts its result
- **THEN** that insertion SHALL be present as a post-flush cache entry

### Requirement: Flush accounting and documentation
Administrative flushes SHALL NOT increment LRU eviction counters. The server SHALL report cumulative `cache_flushes`, `cache_flushed_entries`, and last flush duration through `EMB.STATS` and `INFO cache`, and `EMB.HELP` SHALL document the command syntax.

#### Scenario: Flush counters are distinct from evictions
- **GIVEN** a cache with five entries and zero evictions
- **WHEN** `EMB.CACHE.FLUSH` removes all five entries
- **THEN** `cache_flushes` SHALL increase by one
- **AND** `cache_flushed_entries` SHALL increase by five
- **AND** `cache_evictions` SHALL remain zero

#### Scenario: Authentication and help
- **GIVEN** a password-protected server
- **WHEN** an unauthenticated client sends `EMB.CACHE.FLUSH`
- **THEN** it SHALL receive `NOAUTH Authentication required.`
- **WHEN** an authenticated client sends `EMB.HELP`
- **THEN** the help response SHALL contain `EMB.CACHE.FLUSH [model]`

### Requirement: Flush work stays off the ordinary request path
Adding cache administration SHALL NOT add a per-hit or per-insert index, generation check, allocation, goroutine, or callback. Whole-cache flush SHALL reset cache storage in O(1); model-scoped flush MAY perform O(total entries) work only while that explicit command executes.

#### Scenario: Ordinary cache microbenchmarks remain allocation-identical
- **WHEN** cache `Get` and `Set` benchmarks are compared before and after this change without a flush running
- **THEN** allocations per operation SHALL be unchanged
- **AND** median throughput regression SHALL NOT exceed 2 percent across repeated benchmark runs

