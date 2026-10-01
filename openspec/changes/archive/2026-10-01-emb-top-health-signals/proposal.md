# Proposal

## Why

`emb-top`'s per-model list and activity heatmap re-sort rows on every poll by
instantaneous req/s, so models swap positions constantly and the dashboard
visibly jumps — an operator cannot track a model or read a row across polls.
Separately, the dashboard exposes many raw gauges but never synthesizes them
into an at-a-glance answer to "is this node healthy?", so judging health means
eyeballing several numbers and mentally comparing them.

## What Changes

- Render the per-model list and the heatmap in **stable server `EMB.MODELS`
  order** (remove the per-render req/s sort); keep the existing `j/k` scroll and
  overflow indicator.
- Give numeric columns a **fixed width** so changing values stop shifting the
  row horizontally.
- Add a **health banner**: one overall status (`healthy` / `degraded` /
  `critical` / `no data`) with reason chips for error ratio, p95 latency,
  CPU, cache hit rate, active requests, and connection state.
- Upgrade the latency chart from a single p95 line to a **p50–p99 band with a
  p95 line**, so tail spread is visible, not just the median-of-tail.
- Add a **per-model health dot**, a direction arrow for latency/errors, and an
  explicit idle state for models with no traffic.
- Add simple fixed **threshold defaults** (no new flags).

## Capabilities

### New Capabilities

_(none — this is all dashboard rendering behavior, owned by the existing spec)_

### Modified Capabilities

- `emb-top-dashboard`: stable model ordering in the per-model list and heatmap;
  health-status synthesis and health-banner rendering; latency band
  visualization; per-model health indicators.

## Impact

- `cmd/emb-top/main.go` — rendering (list, heatmap, charts, new banner),
  threshold helpers, derived metrics.
- `internal/embtop/` — any newly derived per-poll values (e.g. smoothed
  baseline for latency regression) and their tests.
- Tests: `cmd/emb-top/main_test.go`, `cmd/emb-top/width_test.go`,
  `internal/embtop/stats_test.go`.
- `openspec/specs/emb-top-dashboard/spec.md` — heatmap "busiest on top" and
  per-model ordering requirements change; new health/band requirements added.
- `website/tools/topviz/` — the landing-page plate replays a real recorded
  session, so the recording must be re-run and re-published after the layout
  change.
- No server, wire-protocol, or dependency changes: the tool stays a pure RESP2
  client (builds with `CGO_ENABLED=0`).
- Deferred (not in this change): server-side signals such as batch queue depth
  or per-model in-flight counts would need `EMB.STATS`/`EMB.INFO` changes and
  are out of scope here.
