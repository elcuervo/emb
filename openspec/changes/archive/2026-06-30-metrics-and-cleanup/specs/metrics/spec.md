## ADDED Requirements

### Requirement: EMB.STATS exposes memory and error metrics

The `EMB.STATS` command SHALL return memory usage, active requests, and error counts in addition to current fields.

#### Scenario: Stats include memory pressure

- **WHEN** a client calls `EMB.STATS`
- **THEN** the response SHALL include `memory_rss_mb` (process RSS in MB)
- **THEN** the response SHALL include `active_requests` (in-flight at query time)

#### Scenario: Stats include error counts

- **WHEN** a client calls `EMB.STATS`
- **THEN** the response SHALL include `total_errors` (cumulative errors across all models)

### Requirement: EMB.INFO exposes model configuration

The `EMB.INFO` command SHALL return model configuration and resource usage in addition to current fields.

#### Scenario: Info includes model config

- **WHEN** a client calls `EMB.INFO <model>`
- **THEN** the response SHALL include `max_length`, `pooling`, `normalize`, `batching_timeout_ms`, `batching_max_batch`

#### Scenario: Info includes resource usage

- **WHEN** a client calls `EMB.INFO <model>`
- **THEN** the response SHALL include `tokens_processed`, `errors`, `memory_mb`
