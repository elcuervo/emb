# Proposal

## Why

`emb-top`'s banner can read `CRITICAL` on a node that is serving fine: a low
cache hit rate is below the built-in critical threshold, so it forces the
overall verdict even when connection, error ratio, p95 latency and CPU are all
healthy. A low hit rate is a property of the workload (diverse texts never
hit), not a fault an operator can act on, and a freshly started node with a
cache reads critical at 0% before any lookups land. The verdict should only
escalate on signals an operator can respond to.

## What Changes

- Health verdicts are driven only by actionable signals: connection state,
  error ratio, p95 latency vs the session baseline, and CPU usage.
- Cache hit rate becomes an informational banner chip: still rendered, but it
  can no longer raise the overall status.
- The cache chip loses its severity thresholds and renders neutral.
- Per-model health is unaffected (it never used cache).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `emb-top-dashboard`: the health-banner requirement is narrowed from
  "reason chips for the signals that drive it: error ratio, p95 latency, CPU
  usage, cache hit rate, active requests, and connection state" to verdicts
  driven only by actionable node signals, with cache hit rate rendered as an
  informational readout. The stale "active requests" chip name (listed in the
  requirement but never implemented) is corrected in the same edit.

## Impact

- `cmd/emb-top/health.go`: `healthInput`, `health()`, `healthState()`, and the
  `cacheDegradedPct`/`cacheCriticalPct` constants.
- `cmd/emb-top/health_test.go`: the "cache degrade" verdict case and the
  fixed-width chip test inputs.
- `openspec/specs/emb-top-dashboard/spec.md`: health-banner requirement and
  the healthy-idle scenario.
- The cache gauge, hit/miss/eviction rates, and per-model rows are unchanged.
