# Design: server connection lifecycle + stats observability

## Summary

Give the server a bounded, observable connection and request lifecycle. Idle connections are reaped by a **15-minute default TTL** (explicit `0` disables, preserving strict Redis semantics); two opt-in caps (`max_connections`, `max_concurrent_requests`, default `0` = unlimited) are enforced through redcon's existing hooks; accurate live counters land in `EMB.STATS`. No new dependencies; no `EMB.INFO` / shutdown changes; additive `EMB.STATS` fields only (array count grows).

```
flags / config.yaml
        │  idle_timeout, max_connections, max_concurrent_requests
        ▼
┌─────────────────────────── Server ───────────────────────────┐
│  accept handler (redcon)                                       │
│    ├─ if max_connections>0 && conns >= max  → return false     │
│    │     (redcon closes; never counted — closed handler        │
│    │      doesn't fire for rejected conns)                     │
│    └─ conns.Add(1)  (atomic)                                   │
│  closed handler (redcon)                                       │
│    └─ conns.Add(-1)                                            │
│  command handler                                                │
│    └─ EMB / EMB.MULTI only:                                    │
│         active := activeReqs.Add(1); if limit>0 && active>cap  │
│             → WriteError "ERR busy ..." and return (no Add)    │
│         defer activeReqs.Add(-1)                               │
│  Serve: when idle_timeout>0 → srv.SetIdleClose(idle_timeout)   │
│         (redcon applies a per-command read deadline,            │
│          redcon.go:416 — no code fork needed)                  │
└────────────────────────────────────────────────────────────────┘
        │
        ▼
EMB.STATS: connections (live), active_requests (live, replaces
           hardcoded "0"), idle_timeout_ms, max_connections,
           max_concurrent_requests → new array count
```

## Redcon mechanics (verified against v1.6.2)

- `Server.idleClose` is copied onto each connection (`redcon.go:376`) and applied as `SetReadDeadline(time.Now().Add(idleClose))` before every command read (`redcon.go:416`). A connection that sends nothing for the duration errors out of the read loop and is closed via the normal path — **which fires the `closed` handler** (see below), so a reap correctly decrements `connections`.
- The **accept handler** is invoked per accepted conn; returning `false` deletes the conn from the server map and closes it, **without** invoking the `closed` handler. So accounting must be: increment only on accept (after the cap check), decrement only in `closed`. Rejected conns are never counted — no bookkeeping imbalance.
- `SetIdleClose` is mutex-guarded and safe to call any time; we call it once in `New`/`ListenAndServe` when `idle_timeout > 0`.

## Accounting semantics

- `conns atomic.Int64` — incremented in the redcon accept callback (post cap-check), decremented in the redcon closed callback. `EMB.STATS connections` reads it.
- `activeReqs atomic.Int64` — incremented at the top of `handleEMB` / `handleEMBMULTI` (only when under cap / cap disabled), decremented via `defer`. Reuses the same entry points where `active.Add(1)/defer active.Done()` already exist for shutdown draining — both counters live side by side.
- Under `max_concurrent_requests`, when the cap is hit the command returns `ERR busy …` *without* incrementing `activeReqs`, so `active_requests` never exceeds the cap (matches spec scenario "N or the capped value").
- Control commands (`PING`, `AUTH`, `EMB.READY`, `EMB.STATS`, `EMB.MODELS`, `EMB.INFO`, `EMB.HELP`) bypass the request cap entirely — they are cheap and must remain reachable during saturation (spec: control commands exempt).

## Config surface

Follows existing single-dash flag + `config.yaml` conventions (`-listen`, `-password`, `-cache`):

- `-idle-timeout <dur>` → tri-state. Unset (nil) resolves to `config.DefaultIdleTimeout` (15m) at build; explicit `0` disables reaping; any positive duration applies. YAML: `idle_timeout` (Go duration string, e.g. `5m`).
- `-max-connections <n>` → `int`, default `0`. YAML: `max_connections`.
- `-max-concurrent-requests <n>` → `int`, default `0`. YAML: `max_concurrent_requests`.
- `Config.IdleTimeout` is `*time.Duration` so "unset" and "explicit 0" are distinguishable; a negative value is rejected at parse time.
- Effective resolution lives in `cmd/emb/main.go`; the `Server` constructor itself defaults `idleTimeout` to `config.DefaultIdleTimeout`, so every entrypoint (flags, YAML, bare `New`) reaps by default and reports the effective TTL in `EMB.STATS`.

## EMB.STATS response shape

Today `EMB.STATS` writes 20 elements (10 pairs): `uptime_secs`, `total_requests`, `active_requests` (hardcoded "0"), `total_tokens`, `total_errors`, `models_loaded`, `per_model`, `cache_hits`, `cache_misses`, `cache_evictions`.

After the change (28 elements, 14 pairs — additive):

```
uptime_secs, total_requests, active_requests (live),
total_tokens, total_errors, models_loaded, per_model,
connections, idle_timeout_ms, max_connections, max_concurrent_requests,
cache_hits, cache_misses, cache_evictions
```

- `idle_timeout_ms` = `int(idle_timeout / time.Millisecond)`, `0` when disabled.
- `max_connections` / `max_concurrent_requests` echo config, `0` when unset (spec: policy echo).
- The Go test parses the declared count and all elements — count-parity enforced (lifecycle of `emb-info-array-count` pattern, but for `EMB.STATS`).
- Ruby client: `stats` currently returns the raw array; the in-flight `ruby-client-observability` change types known keys and passes unknown keys through — additive fields are safe either way.

## Rationale for defaults

- **Idle reaping defaults to 15 minutes** (`config.DefaultIdleTimeout`): active pooled clients (connection_pool/redis-client, redis-py) never sit idle that long between calls in normal operation, and `redis-client` reconnects transparently when a socket is closed — so the TTL is safe for this repo's ecosystem while bounding FD growth and making the "zombie connections survive a consumer restart" failure mode self-healing within a fixed window. A human leaving an interactive `redis-cli` idle past the TTL is disconnected and must reconnect (matches Redis's own `timeout` when configured). Explicit `-idle-timeout 0` restores strict Redis semantics (never close idle).
- **Caps default to `0` = unlimited**: limiting connections/requests in flight changes behavior for existing deployments, so the two caps stay opt-in policy. Idle connections are cheap (parked goroutines); the caps guard pathological pools/leaks (FD/memory), while the **real CPU lever the incident exposed is request volume, which `active_requests` + policy echo make visible**.

## Edge cases

- **Accept race**: redcon serializes accept with its own mutex; our cap check + increment are atomic — worst case one over by N under concurrent accepts; acceptable for a soft cap (documented).
- **Reap vs in-flight**: `SetIdleClose` only deadlines the *read*; an in-flight `EMB` handler completes before the connection closes (read loop is blocked in handler work) — consistent with existing shutdown drain semantics.
- **Long-idle interactive clients**: a `redis-cli` left idle beyond the TTL is closed; it does not auto-reconnect for command-less idle sessions (documented in README). Pooled clients reconnect transparently.
- **EMB.MULTI fan-out**: the cap counts the *command* (one entry), not the paired sub-requests — matches the spec's "process one command" semantics and keeps the gate cheap.
- **Cache interplay**: none — cache path is inside `handleEMB`; the gate sits above it.

## Tests (internal/server + internal/config)

1. **Accounting**: connect N clients → `EMB.STATS connections == N`; close one → `N-1`; close all → `0`.
2. **Reaping**: server with an explicit short TTL (`1s`); idle conn closed ~1s (read EOF / write fails); paced conn survives.
3. **Defaults**: unset idle timeout → `EMB.STATS idle_timeout_ms == 900000` (config: nil pointer); explicit `0` (flag and YAML) → disabled, stats report `0`; negative → parse error.
4. **Connection cap**: `-max-connections 1`; second conn closed without counting; first still answers; after first closes, a new conn is accepted.
5. **Request cap**: `-max-concurrent-requests 1` with a blocking session; second command gets `ERR busy`; `PING`/`EMB.STATS` still answered.
6. **RESP parity**: parse `EMB.STATS` declared count vs elements written, with and without cache; new fields present; old fields preserved.
7. **Regression**: existing `EMB.INFO` count tests and shutdown tests unchanged and green.

## Open Questions

- **Busy error text**: `ERR busy …` prefix is required by spec; exact suffix (e.g., `max concurrent requests exceeded (N)`) is free.
- **Metrics exposure beyond EMB.STATS**: out of scope here (no Prometheus endpoint exists); noted as future work only if requested.

*(Idle-timeout default resolved during implementation: 15m TTL, explicit 0 disables.)*