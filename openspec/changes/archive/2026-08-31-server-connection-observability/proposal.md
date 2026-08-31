## Why

A production incident — a consumer pool restart drove `emb` CPU from ~40% to ~97% and held it there until the consumers disconnected — was diagnosable only by guesswork because the server exposes **no connection or active-request telemetry** (`EMB.STATS` hardcodes `active_requests` to `"0"`) and has **no connection lifecycle policy** (idle connections are never reaped, no cap on connections or concurrent requests, no request logging). The server-side cost model is demand-driven, so "stuck CPU" can only be classified as volume, batching-efficiency, or retry-storm churn — and today none of those three is observable from the server.

## What Changes

- **`EMB.STATS` becomes live and accurate**: replaces the hardcoded `active_requests: "0"` with a real in-flight counter, adds `connections` (accepted minus closed), and echoes the effective policy (`idle_timeout_ms` — the default 900000 when unset, `0` when explicit-disabled; `max_connections`/`max_concurrent_requests` — `0` when unlimited). Additive fields only — `EMB.INFO` and the shutdown lifecycle are untouched.
- **Connection reaping (on by default)**: connections that have sent no command for `idle_timeout` are closed. Default TTL is **15 minutes**; explicit `-idle-timeout 0` disables reaping (strict Redis semantics). Implemented via redcon `SetIdleClose`.
- **Connection cap**: `-max-connections <n>` rejects and closes new connections beyond `<n>` at accept time. Default `0` = unlimited.
- **Request concurrency cap (optional policy)**: `-max-concurrent-requests <n>` answers new `EMB`/`EMB.MULTI` commands with a RESP busy error while `<n>` requests are in flight. Default `0` = unlimited.
- Same knobs available as `config.yaml` keys (`idle_timeout`, `max_connections`, `max_concurrent_requests`).
- Go tests for connection accounting, reaping, caps, and `EMB.STATS` RESP structure.

## Capabilities

### New Capabilities

- `server-connection-lifecycle`: idle-connection reaping (15m default TTL, `0` disables), the accepted-connection cap, and the in-flight request cap — the server's connection/job-bounding policy surface. The caps are opt-in with `0` = unlimited defaults.
- `server-stats-observability`: `EMB.STATS` reports accurate connection and active-request counters plus the effective policy, with a RESP array count that exactly matches the emitted fields.

### Modified Capabilities

None. `EMB.INFO` shape (`emb-info-array-count`) and shutdown behavior (`server-lifecycle`) are unchanged; the in-flight `ruby-client-observability` spec already passes unknown `EMB.STATS` keys through as strings, so the added fields are client-compatible without changes.

## Impact

- `internal/server/server.go`: connection accounting atomics (redcon accept/closed handlers), concurrency gate at `EMB`/`EMB.MULTI` entry, `EMB.STATS` handler rewrite.
- `internal/config/config.go`: tri-state `IdleTimeout` (`nil` = default 15m TTL, explicit `0` = disabled) plus the two int caps; flags and `config.yaml` keys.
- `internal/server/server_test.go`: new behavior + RESP structure tests.
- Dependency: none new — uses redcon's existing `SetIdleClose` and accept-handler hooks.
- Protocol: `EMB.STATS` array count grows (additive); current Ruby client passes unknown keys through unchanged.