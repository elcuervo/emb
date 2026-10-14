# Proposal

## Why

Standing up a new `emb` cluster gives an operator no way to answer the two
questions that decide its fate: *can it take production inference*, and *must I
buy CPU or memory?* Every signal needed is already on the wire — `EMB.STATS`
reports each node's CPU budget and its admission waiting/timeouts, cgroup
throttling and stolen time, RSS, and the cache-restore headroom — but nothing
drives load across a fleet, and nothing turns those counters into an action.
Operators either guess at capacity or read raw counters and infer.

## What Changes

- New `emb-stress` CLI: it takes an `emb` server configuration (which models to
  exercise, and their settings), a DNS target, expands the name into the node
  set behind it, and drives synthetic embedding load across every node.
- `emb-top` gains a **resource-sizing recommendation** layer. Beside the health
  verdict it states what to change: the CPU budget is the binding constraint,
  the host is throttling or stealing CPU, memory/cache headroom is nearly spent,
  RSS is climbing at flat load, or one node trails its peers.
- Node resolution (DNS name → node set) and the health/verdict logic move from
  `cmd/emb-top` into `internal/embtop`, so both tools share one definition of a
  node set and one definition of "unhealthy" and "undersized".
- The `embtop` poll decoder reads the `EMB.STATS` fields it ignores today:
  `cpu_throttle_periods`, `cpu_throttled_periods`, `cpu_throttle_usec`,
  `cpu_steal_usec`, `cache_restore_headroom_bytes`, `cache_restore_skipped_memory`.
- No server-side behavior changes: every signal is already published. No
  **BREAKING** changes; no existing flag changes meaning.

## Capabilities

### New Capabilities

- `emb-stress-load`: synthetic embedding load driven across a DNS-resolved
  fleet, parameterized by an `emb` server configuration — the load generator
  counterpart to `emb-top`'s observer, with the client-side result (per-node and
  aggregate throughput, latency, errors) reported when the run ends.

### Modified Capabilities

- `emb-top-dashboard`: the health banner gains a sizing recommendation derived
  from the node's own CPU budget and admission pressure, host throttle/steal,
  and memory and cache-restore headroom. The recommendation is advisory: like
  the cache chip, it never raises or lowers the health status.
- `emb-top-cluster-view`: the fleet verdict gains a fleet-level sizing
  recommendation — aggregate headroom and the node to grow or inspect first —
  alongside the existing cross-node signals.

## Impact

- New: `cmd/emb-stress/` and `internal/embstress/` (config-driven workload
  generation, fleet fan-out, client-side result).
- Modified: `internal/embtop/` (resolver lifted from `cmd/emb-top/nodes.go`;
  health rules and thresholds lifted from `cmd/emb-top/health.go`; the new
  `Point` fields), `cmd/emb-top/` (consume the shared package, render the
  recommendations), `justfile` (a `stress` target), `docs/operations.md`.
- Unchanged: the server, its RESP surface, the configuration schema, and all
  on-disk formats. `emb-stress` builds without CGo, like `emb-top`, since it
  reuses `internal/config` and `internal/resp` and never opens a model.
