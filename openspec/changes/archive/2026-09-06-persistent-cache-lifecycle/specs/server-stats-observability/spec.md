## MODIFIED Requirements

### Requirement: EMB.STATS RESP array count matches emitted fields

The server SHALL write a RESP array count that exactly matches the number of elements emitted by `EMB.STATS`, including cache-administration and snapshot fields, so clients never stall on a partial read.

#### Scenario: Count-parity across cache lifecycle modes

- **WHEN** `EMB.STATS` is parsed by a Go test
- **THEN** the declared array count SHALL equal the number of elements actually written
- **THEN** the same parity SHALL hold with cache disabled, cache enabled without persistence, persistence idle, and persistence saving

#### Scenario: Existing fields preserved

- **WHEN** `EMB.STATS` is called
- **THEN** all pre-existing fields (`uptime_secs`, `total_requests`, `total_tokens`, `total_errors`, `models_loaded`, `per_model`, cache fields) SHALL remain present
- **THEN** the resource fields `mem`, `cpu_user_usec`, `cpu_sys_usec`, and `goroutines` SHALL also be present

## ADDED Requirements

### Requirement: Cache lifecycle metrics
`EMB.STATS` and the Redis-style `INFO cache` section SHALL expose administrative invalidation and snapshot lifecycle without scanning the cache or touching the filesystem while rendering. Fields SHALL include flush count, flushed entries, last flush duration, persistence enabled, effective startup/periodic/shutdown trigger settings, effective save rate, save in progress, successful/failed/skipped saves, last successful save time and duration, captured entries and bytes, capture-lock duration, sampled restore RSS/headroom, effective restore memory limit, and restored/skipped-by-reason entries.

#### Scenario: Idle persistence status
- **GIVEN** persistence is configured and no save has run
- **WHEN** `EMB.STATS` and `INFO cache` are called
- **THEN** enabled SHALL be true, in-progress SHALL be false, and all lifecycle counters SHALL be zero

#### Scenario: Failure and success counters
- **WHEN** one injected save fails and a later save succeeds
- **THEN** failed and successful counts SHALL each increase by one
- **AND** last-success fields SHALL describe only the successful save

#### Scenario: Metrics rendering has no storage side effects
- **WHEN** lifecycle metrics are polled concurrently with inference and an active save
- **THEN** rendering SHALL read only atomically published status
- **AND** it SHALL not capture the cache, access a snapshot file, wait for the save, or race
