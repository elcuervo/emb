## 1. Config surface

- [x] 1.1 Add `idle_timeout` (`*time.Duration`; nil = default 15m TTL, explicit 0 = disabled), `max_connections` (int, default 0), `max_concurrent_requests` (int, default 0) to `Config` + `FlagConfig` in `internal/config/config.go` with `-idle-timeout`, `-max-connections`, `-max-concurrent-requests` flags (negative duration rejected) and top-level YAML keys; **done** — verify `go test ./internal/config` passes
- [x] 1.2 Document the knobs (`idle_timeout` default 15m / 0 disables; caps 0 = unlimited) in `config.yaml` sample and README; **done** — verify docs render

## 2. Connection lifecycle

- [x] 2.1 Pass the knobs into `server.New(...)` and call `srv.SetIdleClose(idleTimeout)` when `idle_timeout > 0`; verify an idle socket is closed after the timeout and a paced socket survives (integration test `TestIdleClose`)
- [x] 2.2 Implement connection accounting: increment `conns` atomic in the redcon accept callback (after cap check), decrement in the closed callback; verify `EMB.STATS connections` tracks opens/closes and rejected conns are never counted (test `TestConnAccounting`)
- [x] 2.3 Enforce `max_connections` in the accept callback (return false above cap); verify second connection is refused while first stays healthy, and a slot frees after a close (test `TestMaxConnections`)
- [x] 2.4 Enforce `max_concurrent_requests` at `EMB`/`EMB.MULTI` entry with RESP `ERR busy …`; verify busy error under load, `active_requests` never exceeds the cap, and `PING`/`EMB.STATS` remain answered (test `TestMaxConcurrentRequests`)

## 3. Stats observability

- [x] 3.1 Rewrite `EMB.STATS` in `internal/server/server.go`: replace hardcoded `active_requests` with the live counter, add `connections`, `idle_timeout_ms`, `max_connections`, `max_concurrent_requests`; update the array count; verify count-parity test parses declared count == elements written with and without cache (test `TestStatsRESPParity`)
- [x] 3.2 Confirm existing fields remain (`uptime_secs`, `total_requests`, `total_tokens`, `total_errors`, `models_loaded`, `per_model`, cache fields) and `EMB.INFO` array-count tests (`emb-info-array-count`) still pass unmodified

## 4. Validation stage

- [x] 4.1 `just test` passes including the new accounting/reap/cap/RESP tests
- [x] 4.2 `just lint` clean
- [x] 4.3 Manual smoke: run `emb -max-connections 1 -idle-timeout 2s`, open two clients — second refused, first answers; `EMB.STATS` shows `connections: 1`, `active_requests: 0`, policy fields echoed