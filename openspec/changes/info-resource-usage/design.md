# Design: Resource usage reporting (INFO Memory/CPU + net bytes)

## Context

See `proposal.md` Why and the `resource-usage-reporting` spec for the full requirements. Current state that shapes this design:

- `INFO` is built from an immutable `infoSnapshot` gathered in `infoSnapshot()` and rendered by the pure `buildInfoSections()` (`internal/server/info.go`); five sections, no resource fields.
- `pipeline.Stats.MemoryMB` (`internal/pipeline/pipeline.go:26`) is declared but never assigned — `EMB.STATS` prints `mem=0mb` (`server.go:555`).
- `internal/registry/sysmem_{linux,darwin,fallback}.go` expose only `TotalSystemMemory()` (host RAM); no RSS reader exists.
- The server uses `redcon` v1.6.2: per-command handlers on a `redcon.ServeMux`, all replies via `redcon.Conn` write methods. redcon does no byte accounting.
- ONNX Runtime is CGo — its allocations live outside the Go heap, so Go memstats alone would under-report true process memory.
- Go 1.25: `runtime/metrics` provides non-blocking, cross-platform heap and CPU-class samples.

## Goals / Non-Goals

**Goals**
- Truthful per-process numbers in INFO: RSS (incl. CGo), Go heap, goroutines, cumulative CPU user/sys, GOMAXPROCS echo.
- Aggregate bytes-in/out so `# Stats` can be overlaid on cloud network metrics and CPU deltas.
- Fix the misleading `EMB.STATS mem=0mb` without touching client wire contracts beyond adding fields.
- Add an automated leak-regression guard (goroutine/heap flatness under request load).

**Non-Goals**
- Per-connection byte accounting or `EMB.CONN`-style per-connection stats (aggregate counters suffice for the TX/RX-vs-CPU correlation; per-conn can be a follow-up).
- CPU *percent* sampling or rate windows — INFO is a stateless snapshot; operators derive rates from cumulative counters (delta / poll interval).
- Fixing the investigation's adjacent findings (cache auto-sizing vs. cgroup limits, `max_connections` default 0, giant-command parse-cost) — out of scope, tracked as follow-ups.

## Decisions

### D1 — RSS source: Linux `/proc/self/status` VmRSS, macOS `mach_task_basic_info`, fallback Go heap

Process RSS is the only number that includes ONNX Runtime's native arena (`internal/onnx/runtime.go` sets an ORT CPU memory arena; Go `runtime.ReadMemStats` would miss it). This matches the archived `metrics-and-cleanup`/`redis-style-info` design intent ("real RSS via `registry/sysmem`") that never landed.

- Linux: parse `VmRSS` from `/proc/self/status` (zero syscalls beyond one file read; cheapest truthful source).
- macOS: heap fallback is the shipped implementation (`CurrentMemoryUsage` in `sysmem_darwin.go` returns `HeapInUseBytes()`); a true `mach_task_basic_info` source requires cgo and is a documented future option (build-tagged cgo would also break `GOOS=darwin` cross-builds from non-mac hosts).
- Other GOOS: fall back to Go heap in-use.
- Alternatives: `runtime.ReadMemStats` (STW cost and misses CGo — rejected), `Getrusage(RUSAGE_SELF)` (non-portable and granularity quirks — rejected in favor of `/proc`).

New funcs in `internal/registry/sysmem_*.go` (build-tagged, same files as `TotalSystemMemory`): `CurrentMemoryUsage() uint64` returning RSS bytes and a `SupportsRSS() bool` — or a single `CurrentMemoryUsage() (rssBytes uint64, fromRSS bool)` so the INFO renderer can decide fallback semantics (spec: RSS field equals heap when platform lacks RSS).

### D2 — CPU sampling: `runtime/metrics` CPU classes (derived system time)

Cumulative processor time via `runtime/metrics` (non-blocking, no STW). Go 1.26's metric set exposes `/cpu/classes/total:cpu-seconds` and `/cpu/classes/user:cpu-seconds` only (there is no system class), so `used_cpu_sys_usec` is derived as **total − user**. The classes are NaN until the first GC, so `resources.go` runs one startup `runtime.GC()` via `init()` (standard practice) to make INFO CPU meaningful from boot. Heap sampling uses `/memory/classes/heap/objects:bytes` (the in-use heap; the older `heap/in-use` name is gone from recent metric sets).

- Convert to microseconds (`used_cpu_user_usec`, `used_cpu_sys_usec`) to match the Redis `used_cpu_user`/`used_cpu_sys` semantics while keeping explicit units (emb convention).
- `gomaxprocs` echoes `runtime.GOMAXPROCS(0)` — real config, explains thread pressure (investigation: two model sessions each run `intra_op_threads=cores−2`, so `2×(cores−2)` ONNX threads can oversubscribe).
- Alternatives: `syscall.Getrusage` (Linux/darwin only — rejected for portability), `runtime.ReadMemStats` CPU fields (none).

### D3 — Net byte counters via handler + connection wrappers in `server.go`

redcon does no byte accounting (`connState` holds only auth state). Hook at the single choke point:

- **RX**: in the dispatch handler, add `len(cmd.Raw)` (the exact RESP wire bytes, framing included) to a global `totalNetInput` atomic before the auth check, so all traffic is accounted.
- **TX**: redcon v1.6.2's `Write*` methods return **nothing**, so instead of summing returned counts the wrapper (`countingConn`, `internal/server/counting.go`) computes the deterministic RESP2 wire size of every `Write*` call (e.g. `$n\r\n…\r\n` for bulks, `*n\r\n` for arrays, `$-1\r\n` for nulls) and adds it to `totalNetOutput`. `WriteAny` sizes via `redcon.AppendAny(nil, v)` (emb never calls it). Every reply flows through the wrapper passed to handlers, so zero per-handler changes are needed.

Simpler alternative considered: counting `len(cmd.Args)`/reply sizes per handler — rejected as repetitive and drift-prone (each new handler must remember to count). The wrapper approach is centrally enforced and survives new commands.

The counters are per-process aggregate `atomic.Uint64` on the `Server`; exposed in `infoSnapshot` and rendered in `# Stats` as `total_net_input_bytes` / `total_net_output_bytes` (Redis-compatible names).

### D4 — New INFO sections: `# Memory` and `# CPU`, inserted after `# Stats`

Rendered by `buildInfoSections()` with the existing keyspace-selection mechanism (`info.go` selection map gains `memory`/`cpu`). Order becomes `Server → Cache → Keyspace → Stats → Memory → CPU → Clients` (keeps existing "in that order" slices intact; `redis-style-info` delta updated accordingly).

```
# Memory
used_memory_rss_bytes:...
used_memory_heap_bytes:...
goroutines:...
total_system_memory_bytes:...     (from TotalSystemMemory(), host RAM context)

# CPU
used_cpu_user_usec:...
used_cpu_sys_usec:...
gomaxprocs:...
```

Only measured values — satisfies the "no invented fields" rule without hardcoding.

### D5 — Repair `EMB.STATS` dead memory field

`handleSTATS` (`server.go:537-571`): replace the always-zero per-model `st.MemoryMB` (field removed from `pipeline.Stats`) with a server-wide `mem` field (MB, RSS with heap fallback) and `cpu_user_usec`, `cpu_sys_usec`, `goroutines` sourced from the same `resourceStats()` snapshot INFO uses, so the two cannot drift. The 16-pair array becomes 20 pairs: `conn.WriteArray(32)` → `conn.WriteArray(40)` (spec: count parity maintained; `TestStatsRESPParity` updated — the Ruby client decodes generically via `each_slice(2).to_h`, verified end-to-end).

### D6 — Leak regression test uses gated fakes + heap/goroutine assertions

Pattern from the deterministic batcher tests (`.pi/`-documented: gated fakes, no sleeps): run many batches through the pipeline with gated fake sessions, then assert (a) `runtime.NumGoroutine()` returns to the pre-test baseline, (b) `runtime/metrics` heap in-use growth stays flat (allow GC noise via several `runtime.GC()` passes and generous-but-bounded tolerance). This automates the "no memory leak" guarantee instead of requiring manual RSS watching.

## Risks / Trade-offs

- [INFO is auth-exempt] → The new Memory/CPU fields remain world-readable on password-protected servers (existing probe semantics, consistent with `EMB.READY`/`PING`). → No secret material is leaked (RSS/CPU are not credentials); document in `EMB.HELP` as part of INFO. AUTH-gating INFO would be a separate change.
- [RSS parse cost on every INFO call] → `/proc/self/status` read is sub-microsecond; INFO is not hot-path (operators poll at Hz rates).
- [Counter drift] → `len(cmd.Raw)` counts exact wire bytes; `countingConn` counts every reply byte, including multiplexed replies that bypass individual `Write*` returns — verify with a count-parity test comparing a measured exchange (send/receive known bytes, assert counters ≥ that).
- [runtime/metrics cumulative seconds can be NaN in early startup before first GC] → guard: treat NaN as 0.
- [goroutines baseline noise] → leak test asserts return-to-baseline after work completes, not during; tolerance for runtime-internal goroutines.
- [Write-method byte counts may not include RESP framing in all redcon versions] → the TX count is a lower bound on wire bytes; spec's "at least reply wire size" scenario accommodates this; verified against redcon v1.6.2 at implementation time — sizing is computed from the exact RESP2 encoding rules, matching framing exactly.

## Migration Plan

Additive only: new INFO sections + new stats fields + repaired `mem`. Deploy as a normal release; no config flags, no wire-format breaks (`INFO <section>` filtering and count parity are preserved). Rollback is a version revert. Follow-up hardening (cgroup-aware cache sizing, connection caps) stays out of this change.

## Open Questions

None that block specs/approach. (Decided: field names use explicit `_bytes`/`_usec`/`_mb` units over bare Redis names — consistent with existing emb INFO; per-connection bytes deferred.)