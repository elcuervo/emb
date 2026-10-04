# Design

## Context

See `proposal.md - Why`. Current state that shapes the approach:

- `cmd/emb-top/main.go` renders everything. `modelsView()` (line ~763) and
  `hotRows()` (line ~722) each re-sort by instantaneous req/s every render —
  the source of the jump.
- Colors already come from `modelColorIdx(name, m.modelOrder)`, keyed to server
  order, so a stable order keeps colors stable for free.
- The latency chart is a single-series `streamlinechart` (p95 only).
- The heatmap is hand-rolled because ntcharts cannot style whitespace-only
  cells; that precedent applies to any new chart that needs fills.
- `emb-top` is a pure RESP2 client (no server, no CGo). Headless `-once`
  (`internal/embtop/once.go`) and `-frames` (`View()`) are separate paths.
- The landing-page plate replays a real recorded session
  (`website/tools/topviz/`), so layout changes must be re-recorded.

## Goals / Non-Goals

**Goals**

- A per-model list and heatmap that never reorder on their own.
- One synthesized health verdict an operator can trust at a glance.
- A latency view that shows distribution spread, not just one percentile.

**Non-Goals**

- Server changes, new wire commands, or new dependencies.
- New CLI flags or configurable thresholds (fixed defaults only).
- Reworking the headless `-once` line format (out of scope; see Open Questions).

## Decisions

### 1. Stable order = server `EMB.MODELS` order

Delete the two `sort.SliceStable` calls; render `m.modelOrder` as-is in both the
list and the heatmap. Colors and scrolling already key off `m.modelOrder`.

- *Alternatives:* EMA/dwell sorting (still drifts, adds state and tuning);
  alphabetical (stable but arbitrary). Server order is the least code and the
  most predictable.

### 2. Fixed-width metric columns

Format each per-model segment to a constant column width (pad/right-align) and
compute the row-fit budget from those fixed widths, so a value changing width
(`9.9` → `10.0`) cannot shift the columns after it.

- *Alternative:* leave variable widths — rejected; it is a second, subtler jump.

### 3. Health status as a pure function with fixed thresholds

New `cmd/emb-top/health.go`: `health(snapshot) (status, reasons)` where
`status ∈ {no-data, critical, degraded, healthy}` and `reasons` names the
offending signals. Precedence: `no-data` (fewer than two polls) → `critical`
(disconnected, or any signal past its critical bound) → `degraded` → `healthy`.
Thresholds are package constants in one place:

| Signal | degraded | critical |
|---|---|---|
| recent error ratio (`err/s ÷ req/s`) | > 1% | > 5% |
| p95 vs session baseline (slow EMA) | > 2× | > 4× |
| CPU (percent of total capacity) | > 85% | > 95% |
| cache hit rate (when cache is in use) | < 30% | < 10% |
| connection | — | disconnected |

Chips that are informational (active requests, uptime) render neutral and do not
change the verdict. CPU is `cpu_user_usec`/`cpu_sys_usec` deltas divided by
`runtime.NumCPU()`, because `CPUPercent` is percent of a single core and a
healthy multi-core node routinely exceeds 100% of one core; the cache
threshold is low because a workload-sized working set legitimately keeps the
hit rate near 50%.

- *Alternatives:* thresholds inline in `View()` (rejected: untestable);
  configurable flags (rejected: the user asked for simple). A pure function is
  a table-test seam.

### 4. Per-model health from existing `ModelPoint`

One `modelHealth(mp)` helper returns a status dot, a rising/falling arrow for
error rate and latency (latest vs previous sample), and an explicit `idle`
state when the model saw no traffic in the window. Reuses the same status enum
and colors as the banner so the language is consistent.

### 5. Latency band: hand-rolled, reusing the heatmap precedent

Record per-poll `p50/p95/p99` into a bounded `latHist` ring on the TUI model
(computed from the existing `Sampler.Latency()` call in `applyResult`; no
sampler change). Render a fixed-height band: for each poll column, shade cells
between the p50 and p99 positions and draw p95 as a bright rune — the same
technique `heatmapView` already uses. This replaces the single-series
`latChart`.

- *Alternatives:* multi-series `streamlinechart` (`PushDataSet`) — trivial but
  gives an unshaded envelope without the requested fill. Kept as a fallback if
  the hand-rolled band proves fiddly.

## Risks / Trade-offs

- **New banner line / indicator changes vertical budget** → the per-model
  reservation (`m.height - 20`) and the fixed `frameHeight` (40) in `-frames`
  mode must be re-checked so the 120×40 frame still fits; adjust `frameHeight`
  or the reservation together in one task.
- **Health thresholds can mislabel a slow-but-healthy node** → all thresholds
  live in one constants block, easy to tune; the baseline is relative (slow
  EMA) rather than an absolute latency, which is the fragile choice.
- **Hand-rolled band has width/height edge cases** (single sample, all-equal
  values, zero events) → unit-test the band renderer as a pure function with a
  table of small cases.
- **Recording is now stale** → re-run `just website-topviz` after the layout
  settles; it is part of the change, not a follow-up.
- **Health dot colors may collide with model colors** → use the reserved
  green/amber/red semantic styles, distinct from the `modelColors` palette.
