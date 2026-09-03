## Why

In production the operator sees TX/RX traffic spikes that coincide with CPU increases and cannot tell whether the instance is healthy, saturated, or leaking. Today `INFO` reports zero CPU, memory, or traffic-usage — the only memory number on the wire is `cache_memory_bytes` (the LRU's accounted entries), `EMB.STATS` reports a dead `mem=0mb` (`pipeline.Stats.MemoryMB` is declared but never populated), and there are no bytes-in/out counters. A three-agent investigation of the server confirmed the steady-state design is bounded (byte-budgeted LRU, fixed-cap queues, bounded goroutines) but found **no in-process observability to prove it**: no RSS reader, no CPU accounting, no goroutine count, and no leak regression test. An operator cannot currently answer "is memory growing?" from the server itself.

## What Changes

- **`INFO` gains `# Memory` and `# CPU` sections** (Redis-format, added to the default no-args section set):
  - `# Memory`: process RSS (Linux `/proc/self/status` VmRSS; macOS `mach_task_basic_info`; fallback Go heap), Go heap in-use, goroutine count, and total system memory. RSS, not Go memstats, is the truthful figure because ONNX Runtime allocations live outside the Go heap (CGo).
  - `# CPU`: cumulative user/system CPU seconds from `runtime/metrics` (cross-platform, covers cgo work) and `GOMAXPROCS`.
- **Total net RX/TX byte counters** (`total_net_input_bytes`, `total_net_output_bytes`) in `# Stats`, summed per connection via a counting wrapper — lets the operator overlay bytes-in/out on CPU seconds and request rates to diagnose the TX/RX-spike-vs-CPU correlation directly.
- **`EMB.STATS` fixes the dead `mem` field** to real RSS and gains `cpu_user_usec`/`cpu_sys_usec` and `goroutines`, keeping RESP array-count parity.
- **A leak regression test** (white-box, gated-fake pattern used by the batcher budget tests) asserting goroutine count and heap/RSS stay flat across many batches — an automated "no memory leak" guard, plus test coverage for the new INFO sections and filter behavior.

All fields report only measured or real-configuration values (the `redis-style-info` "no invented fields" rule continues to hold). Out of scope (found by investigation, tracked as follow-ups): cache auto-sizing from host RAM instead of cgroup limits, unbounded `max_connections` default, giant-command parse-cost spikes.

## Capabilities

### New Capabilities
- `resource-usage-reporting`: live process CPU/memory/traffic reporting in `INFO` — RSS, Go heap, goroutines, CPU seconds, and aggregate net bytes, with Redis-style sections and no invented values.

### Modified Capabilities
- `redis-style-info`: the no-args `INFO` section set gains `# Memory` and `# CPU` (in fixed order), and `# Stats` gains the net byte counters; the existing no-invented-fields rule is extended to the new sections.
- `server-stats-observability`: `EMB.STATS` `mem` field becomes real RSS instead of the always-zero value, and the response gains `cpu_user_usec`, `cpu_sys_usec`, and `goroutines` while preserving RESP array-count parity.

## Impact

- **Code**: `internal/server/info.go` (sections, snapshot), `internal/server/server.go` (stats, counting wrapper, dispatch), `internal/registry/sysmem_*.go` (new `CurrentMemoryUsage()` on linux/darwin/fallback), new `internal/registry/resources.go` or equivalent (`runtime/metrics` CPU + heap sampling), `internal/pipeline/pipeline.go` (wire `MemoryMB` rather than leaving it dead).
- **Tests**: `internal/server/info_config_test.go`, new leak-regression test in `internal/server/` or `internal/pipeline/`; `go vet`/`golangci-lint` clean.
- **Clients**: Ruby gems parse INFO generically (`parse_info`) — no client changes required.
- **Dependencies**: none new — `runtime/metrics`, `os`, `syscall` are stdlib.
- **Ops**: new fields are readable via existing `INFO` / `EMB.STATS`; no config or wire-format breaks (`INFO <section>` filtering unchanged, unknown sections still return empty).