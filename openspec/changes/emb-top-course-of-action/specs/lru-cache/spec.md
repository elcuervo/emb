# Spec Delta

## MODIFIED Requirements

### Requirement: Metrics

The server SHALL expose cache statistics, scoped so that a caller can read one model's cache behaviour and the process's. `EMB.INFO <model>` SHALL report the counters of that model's own entries: `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries` and `cache_memory_bytes` (the bytes those entries occupy). `cache_max_bytes` is the cache's own budget, which every model's entries share, and SHALL NOT be reported as a per-model allowance. `EMB.STATS` SHALL report the aggregate across all models. Both SHALL keep the field names, encodings and RESP array counts they have today, and a model whose cache is untouched SHALL report zeroes rather than another model's values.

#### Scenario: EMB.INFO shows cache stats

- **WHEN** a client sends `EMB.INFO minilm`
- **THEN** the response SHALL include `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries` and `cache_memory_bytes` for `minilm`'s own entries, and `cache_max_bytes` as the budget they share

#### Scenario: Per-model counters are not the process's

- **GIVEN** two loaded models whose entries were looked up different numbers of times
- **WHEN** a client sends `EMB.INFO` for each
- **THEN** each reply SHALL carry its own model's hit and miss counts, and they SHALL differ from each other and from `EMB.STATS`' aggregate

#### Scenario: EMB.STATS includes cache totals

- **WHEN** a client sends `EMB.STATS`
- **THEN** the response SHALL include aggregate cache stats across all models
