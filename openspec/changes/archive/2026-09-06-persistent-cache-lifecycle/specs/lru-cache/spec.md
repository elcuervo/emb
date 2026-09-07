## ADDED Requirements

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

