# Proposal

## Why

The `emb-top` gauges (cache / cpu / mem) sit on their own three-up grid — bars
start at columns 1, 40 and 79 on a 120-column terminal — while the per-model
rows above them use a different grid (identity ends at 32, metrics at 95, strip
starts at 98). Nothing lines up: the bottom of the dashboard reads as a separate
panel bolted under the table rather than the table's own footer.

A second, latent problem blocks any fix: the model row only reserves its p50/p95
columns when latency events exist, so with the `avg` fallback the strip starts
twelve columns early. The row grid is not stable, so there is no grid to align to.

## What Changes

- Derive one column grid from the terminal width — identity block, metric slots,
  activity strip — and use it for both the model rows and the gauges.
- Always reserve the p50 and p95 slots; the `avg` fallback occupies the first
  latency slot and leaves the second blank, so a row's strip start no longer
  moves when events appear or lapse.
- Place the three gauges on that grid: `cache` under the identity block, `cpu`
  under the metric block, `mem` under the activity strip, each bar spanning its
  zone's columns.
- Re-record the landing-page plate and the documentation capture for the new
  layout (the recorded frame, cast and gif go stale otherwise).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `emb-top-dashboard`: the dashboard's gauges must share the per-model row's
  column grid, and that grid must be stable across rows and polls.

## Impact

- `cmd/emb-top/main.go` — row layout, `modelRow`, `layoutCharts`, `gaugesView`,
  `caption`.
- `cmd/emb-top/main_test.go` / `width_test.go` — a grid-alignment test.
- `website/index.html`, `website/docs/index.html`, `website/assets/cast/`,
  `website/tools/published-tree.py`, `docs/assets/` — re-recorded by
  `just website-topviz`.
