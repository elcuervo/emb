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

The server SHALL report the number of `EMB`/`EMB.MULTI` requests currently being processed in `EMB.STATS` as the integer field `active_requests`, replacing the previously hardcoded `"0"`.

#### Scenario: Active requests during in-flight work

- **GIVEN** N `EMB` requests are being processed concurrently
- **WHEN** `EMB.STATS` is called
- **THEN** `active_requests` SHALL report N (or the capped value under `max_concurrent_requests`)

#### Scenario: Idle server reports zero

- **WHEN** no `EMB`/`EMB.MULTI` requests are in flight
- **THEN** `active_requests` SHALL report `0`

### Requirement: EMB.STATS echoes the effective policy

The server SHALL include the effective policy values in `EMB.STATS`: `idle_timeout_ms` SHALL report the applied TTL (the default 900000 when unset, `0` when explicitly disabled), and the caps SHALL report `0` when left unlimited.

#### Scenario: Policy fields present

- **WHEN** `EMB.STATS` is called
- **THEN** the response SHALL include `idle_timeout_ms`, `max_connections`, and `max_concurrent_requests`
- **THEN** `idle_timeout_ms` SHALL be the applied TTL, or `0` when reaping is explicitly disabled
- **THEN** each cap SHALL be the configured value, or `0` when unlimited

### Requirement: EMB.STATS RESP array count matches emitted fields

The server SHALL write a RESP array count that exactly matches the number of elements emitted by `EMB.STATS`, so clients never stall on a partial read.

#### Scenario: Count-parity with and without cache

- **WHEN** `EMB.STATS` is parsed by a Go test
- **THEN** the declared array count SHALL equal the number of elements actually written
- **THEN** the same parity SHALL hold whether or not an embedding cache is configured

#### Scenario: Existing fields preserved

- **WHEN** `EMB.STATS` is called
- **THEN** all pre-existing fields (`uptime_secs`, `total_requests`, `total_tokens`, `total_errors`, `models_loaded`, `per_model`, cache fields) SHALL remain present
