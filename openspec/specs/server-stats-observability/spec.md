# server-stats-observability Specification

## Purpose
Makes `EMB.STATS` an accurate, live window into server load: real in-flight request count (replacing the hardcoded `"0"`), live connection count, and the effective connection/request policy. Enables operator classification of "stuck CPU" incidents (volume vs. churn vs. saturation) without code changes.

## Requirements

### Requirement: EMB.STATS reports live connection count

The server SHALL report the number of currently accepted, unclosed connections in `EMB.STATS` as the integer field `connections`.

#### Scenario: Connections counted on open and close

- **WHEN** a client opens a connection to the server
- **THEN** `EMB.STATS` SHALL report `connections` incremented by one
- **WHEN** that client closes the connection
- **THEN** `EMB.STATS` SHALL report `connections` decremented by one

#### Scenario: Rejected connections are not counted

- **WHEN** a connection is refused due to `max_connections`
- **THEN** it SHALL NOT be reflected in `connections`

### Requirement: EMB.STATS reports live active requests

The server SHALL report the number of inference requests currently being processed in `EMB.STATS` as the integer field `active_requests`, replacing the previously hardcoded `"0"`. The counted commands SHALL be `EMB`, `EMB.MULTI`, `EMB.IMG`, `EMB.IMGMULTI`, `EMB.EVAL`, and `EMB.EVSHA`.

#### Scenario: Active requests during in-flight work

- **GIVEN** N `EMB` requests are being processed concurrently
- **WHEN** `EMB.STATS` is called
- **THEN** `active_requests` SHALL report N (or the capped value under `max_concurrent_requests`)

#### Scenario: Idle server reports zero

- **WHEN** no inference requests are in flight
- **THEN** `active_requests` SHALL report `0`

#### Scenario: Scripted evaluations count as active requests

- **GIVEN** a long-running `EMB.EVSHA` evaluation is in flight
- **WHEN** `EMB.STATS` is called from another connection
- **THEN** `active_requests` SHALL include it

### Requirement: EMB.STATS echoes the effective policy

The server SHALL include the effective policy values in `EMB.STATS`: `idle_timeout_ms` SHALL report the applied TTL (the default 900000 when unset, `0` when explicitly disabled), and the caps SHALL report `0` when left unlimited.

#### Scenario: Policy fields present

- **WHEN** `EMB.STATS` is called
- **THEN** the response SHALL include `idle_timeout_ms`, `max_connections`, and `max_concurrent_requests`
- **THEN** `idle_timeout_ms` SHALL be the applied TTL, or `0` when reaping is explicitly disabled
- **THEN** each cap SHALL be the configured value, or `0` when unlimited

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

### Requirement: EMB.STATS reports process resource usage

`EMB.STATS` SHALL report the `mem` field as the process resident-set size in megabytes (falling back to Go heap when RSS is unavailable), replacing any constant zero, and SHALL include `cpu_user_usec`, `cpu_sys_usec` (cumulative microseconds since start), and `goroutines` alongside the existing fields.

#### Scenario: Memory field is live

- **WHEN** `EMB.STATS` returns `mem`
- **THEN** the value SHALL be a positive integer once models are loaded and inference has run (platforms without RSS report heap-derived MB)

#### Scenario: CPU fields present

- **WHEN** `EMB.STATS` is called
- **THEN** the response SHALL include `cpu_user_usec` and `cpu_sys_usec` as non-negative integers and `goroutines`

### Requirement: Cache lifecycle metrics
`EMB.STATS` and the Redis-style `INFO cache` section SHALL expose administrative invalidation and snapshot lifecycle without scanning the cache or touching the filesystem while rendering. Fields SHALL include flush count, flushed entries, last flush duration, persistence enabled, effective startup/periodic/shutdown trigger settings, effective save rate, save in progress, successful/failed/skipped saves, last successful save time and duration, captured entries and bytes, capture-lock duration, sampled restore RSS/headroom, effective restore memory limit, and restored/skipped-by-reason entries.

#### Scenario: Idle persistence status
- **GIVEN** persistence is configured and no save has run
- **WHEN** `EMB.STATS` and `INFO cache` are called
- **THEN** enabled SHALL be true, in-progress SHALL be false, and all save-related counters SHALL be zero
- **AND** restore counters (restored, skipped-by-reason, quarantined, effective limit, sampled RSS/headroom) SHALL reflect startup restoration when one ran, and SHALL be zero when no restore ran

#### Scenario: Failure and success counters
- **WHEN** one injected save fails and a later save succeeds
- **THEN** failed and successful counts SHALL each increase by one
- **AND** last-success fields SHALL describe only the successful save

#### Scenario: Metrics rendering has no storage side effects
- **WHEN** lifecycle metrics are polled concurrently with inference and an active save
- **THEN** rendering SHALL read only atomically published status
- **AND** it SHALL not capture the cache, access a snapshot file, wait for the save, or race

### Requirement: Scripted evaluations are observable

Scripted evaluations SHALL be first-class in the server's observability surface, so operators can see them alongside native embedding traffic rather than only through `max_concurrent_requests` behaviour.

- Every completed `EMB.EVAL`/`EMB.EVSHA` evaluation SHALL append a `MONITOR` event with the model, the number of texts, the latency in microseconds, and an error flag. Request text payloads SHALL NOT be recorded, exactly as for `EMB`.
- `EMB.STATS` SHALL report cumulative scripted counters: `script_requests`, `script_errors`, and `script_avg_latency_us`.
- Per-model breakdowns in `EMB.STATS`/`EMB.INFO` SHALL include scripted counts for models that have served scripted traffic, distinct from embedding counts.
- Stats SHALL expose the scripted resource footprint per model: the number of named-tensor sessions open and whether the script tokenizer is loaded, so the memory cost of scripting is inspectable without reading process memory.
- Counter reads SHALL be atomic and SHALL NOT allocate unboundedly or scan caches.

#### Scenario: Scripted request appears in MONITOR

- **WHEN** a client sends `EMB.EVSHA` and then queries `MONITOR`
- **THEN** the event ring contains an event for that evaluation with the model, text count, latency, and error flag, and no text payload

#### Scenario: Scripted counters are cumulative

- **WHEN** N scripted evaluations complete, one of which fails
- **THEN** `script_requests` is N, `script_errors` is 1, and `script_avg_latency_us` reflects the completed evaluations

#### Scenario: Per-model scripted counts are distinguishable

- **WHEN** a model has served both `EMB` requests and scripted evaluations
- **THEN** its per-model entry reports the embedding request count and the scripted request count separately

#### Scenario: Script resource footprint is reported

- **WHEN** `EMB.EVSHA` runs a script that uses `emb.run`
- **THEN** the model's reported script session count is at least 1, and it is 0 for a model that has only run `emb.embed` scripts

#### Scenario: RESP array count still matches

- **WHEN** `EMB.STATS` is parsed after the new fields are added
- **THEN** the declared RESP array count equals the number of elements emitted, with and without scripted traffic
