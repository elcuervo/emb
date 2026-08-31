## MODIFIED Requirements

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

### Requirement: Metrics

The server SHALL expose cache statistics.

#### Scenario: EMB.INFO shows cache stats

- **WHEN** a client sends `EMB.INFO minilm`
- **THEN** the response SHALL include `cache_hits`, `cache_misses`, `cache_hit_rate`, `cache_evictions`, `cache_entries`, `cache_max_bytes`, `cache_memory_bytes`

#### Scenario: EMB.STATS includes cache totals

- **WHEN** a client sends `EMB.STATS`
- **THEN** the response SHALL include aggregate cache stats across all models