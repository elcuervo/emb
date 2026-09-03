## MODIFIED Requirements

### Requirement: EMB.STATS RESP array count matches emitted fields

The server SHALL write a RESP array count that exactly matches the number of elements emitted by `EMB.STATS`, so clients never stall on a partial read.

#### Scenario: Count-parity with and without cache

- **WHEN** `EMB.STATS` is parsed by a Go test
- **THEN** the declared array count SHALL equal the number of elements actually written
- **THEN** the same parity SHALL hold whether or not an embedding cache is configured

#### Scenario: Existing fields preserved

- **WHEN** `EMB.STATS` is called
- **THEN** all pre-existing fields (`uptime_secs`, `total_requests`, `total_tokens`, `total_errors`, `models_loaded`, `per_model`, cache fields) SHALL remain present
- **THEN** the resource fields `mem`, `cpu_user_usec`, `cpu_sys_usec`, and `goroutines` SHALL also be present

## ADDED Requirements

### Requirement: EMB.STATS reports process resource usage

`EMB.STATS` SHALL report the `mem` field as the process resident-set size in megabytes (falling back to Go heap when RSS is unavailable), replacing any constant zero, and SHALL include `cpu_user_usec`, `cpu_sys_usec` (cumulative microseconds since start), and `goroutines` alongside the existing fields.

#### Scenario: Memory field is live

- **WHEN** `EMB.STATS` returns `mem`
- **THEN** the value SHALL be a positive integer once models are loaded and inference has run (platforms without RSS report heap-derived MB)

#### Scenario: CPU fields present

- **WHEN** `EMB.STATS` is called
- **THEN** the response SHALL include `cpu_user_usec` and `cpu_sys_usec` as non-negative integers and `goroutines`