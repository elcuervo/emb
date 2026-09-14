## ADDED Requirements

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
