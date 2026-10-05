# Design

## Context

See `proposal.md - Why`. Today `cmd/emb-top/main.go` renders two independent
windows over one `modelOrder` slice: `heatmapView` (`hotRows`, capped at 8, never
scrolls) and `modelsView` (a `m.scroll` window, two lines per model — stats plus
an optional meta line). The order itself is already first-seen stable; the
jumpiness comes from the second window, the cap, and rows whose height depends on
whether `EMB.INFO` answered. `-frames` renders the same `View()`, so the website
(which consumes frames read-only) and the recorded plate follow whatever the TUI
does.

## Goals / Non-Goals

**Goals:**

- One model order and one scroll offset for the whole view; a model has exactly
  one row position everywhere.
- Constant row height, so a missed `EMB.INFO` never shifts the rows below.
- Every loaded model's activity is reachable by scrolling.
- `-once` emits models in the same stable order.
- Keep the at-a-glance activity visualization and the live TUI feel; keep the
  aggregate req/s stream, latency band, gauges, banner and per-model health dot.

**Non-Goals:**

- Adding input to the website's `/stats` view (it stays read-only; it shows the
  list's top window).
- Auto-paging or animation of the list (that would reintroduce motion).
- Preserving the separate 8-row heatmap block.
- New flags or dependencies.

## Decisions

### 1. The activity strip moves into the row

Each model row becomes `● name  spark  strip  req/s  tok/s  p95  err  meta`,
rendered by the existing `modelsView`, and `heatmapView`/`hotRows` are removed.
The strip uses the existing `heatColors`, `heatCellStyle` and time-direction
legend logic.

- *Why:* a per-row strip cannot disagree with the row it sits in, so the
  "one position per model" invariant is structural rather than kept in sync by
  hand. It also removes the fixed cap, so the list scales past eight models.
- *Alternatives:* keep the block but scroll-lock it to the list window — the same
  models then render twice (block plus list), which is more surface area for the
  same information; keep the block and cap — leaves models unreachable.

### 2. Single-line rows with inline metadata

The two-line layout collapses to one line: `dim · pooling · quant` (and batching,
when present) is appended after the numeric segments when it fits the width
budget, and dropped otherwise. `colRate`/`colLat`/`colErr` fixed-width segments
and the existing `appendSeg` fit loop are reused unchanged.

- *Why:* one line is what makes a 40-row frame fit ~25 models instead of ~9, and
  it removes the meta line whose absence caused the vertical jump.
- *Alternatives:* always reserve a second blank line — doubles the height for
  metadata that is often absent and wastes the web frame's fixed rows.

### 3. Last-known values and the stale marker

The sampler keeps the previous `ModelPoint` and raw `ModelStats` for a model
whose `EMB.INFO` is absent for a poll, instead of overwriting with zero, and
marks the sample stale for that poll. The row renders the last-known values with
the model's indicator dimmed while stale.

- *Why:* satisfies "a missed poll does not move rows" without fabricating fresh
  numbers — the values shown are the last measured ones, visually marked.
- *Alternatives:* render zeros/blank — changes what the row says and shrinks it.

### 4. Persistent row slots

`modelOrder` becomes an append-only ever-seen list. A set of currently-announced
models (from each `EMB.MODELS` reply) decides which rows render; an absent model
keeps its slot in `modelOrder` but contributes no row, so the rows around it do
not move, and it reappears in place when announced again. Scroll indexes the
filtered (present) rows, but each row's relative order is its `modelOrder` order.

- *Why:* position retention falls out of the data structure instead of a
  remembered-index map.
- *Alternatives:* remember last index per name and reinsert — more state and the
  same result.

### 5. Range indicator and scroll bounds

The indicator reads `rows <a>–<b> of <n>` from the present-row count, replacing
`<maxRows> of <n>`. The scroll offset is clamped in `Update` (a real model
mutation) and again at render, so it can never point past the end.

- *Why:* the old indicator only ever printed the window size, so scrolling gave
  no feedback.

### 6. `-once` shares the first-seen reconciliation

`RunOnce` keeps an ordered `known` list: filter out models no longer announced,
append newly announced ones, and emit per-model sections in that order.

- *Why:* the stable-rows change fixed the TUI but left `-once` rebuilding order
  from `res.Models` every poll; this closes the last reordering surface.
- *Alternatives:* extract a shared reconciler type used by both the TUI and
  `RunOnce` — the logic is a few lines and the TUI's owns extra per-model state
  (sparks), so a shared type would be an abstraction for two callers with
  different needs.

### 7. Layout budget

The model area is `height - chrome`, where chrome is the fixed row count of the
header, banner, aggregate charts, gauges, ticker, footer and indicator. Removing
the block and the meta lines leaves roughly `height - 15` single-line rows.

- *Why:* a fixed chrome count keeps the panel bounded and the aggregate
  visualization intact in both the TUI and the 120×40 web frame.

## Risks / Trade-offs

- [Losing the compact all-models heat block weakens the "whole node at a glance"
  read] → the aggregate req/s stream, latency band and gauges remain the
  at-a-glance surfaces, and per-model bands still read as bands in the rows.
- [One line is tight at narrow widths] → the existing width-budget `appendSeg`
  drops trailing segments (meta first) rather than wrapping; `-frames` is fixed
  at 120 columns and tested for overflow.
- [Ever-seen `modelOrder` grows with model churn] → bounded by distinct model
  names ever announced, which is small; no eviction needed.
- [The read-only web view cannot scroll] → single-line rows maximize how many
  models fit the 120×40 frame; the top window is shown. Adding input to `/stats`
  is a separate change.
- [The recorded plate and website frames go stale with the new layout] →
  re-run `just website-topviz` as part of the change.

## Migration Plan

No data or API migration: `emb-top` is a RESP2 client and `EMB.MODELS`,
`EMB.INFO`, `EMB.STATS` and `MONITOR` are unchanged. Ship as one binary change;
rollback is reverting the build. The website's `/stats` viewer needs no code
change and picks up the new frames automatically once the sandbox producer
binary is rebuilt.

## Open Questions

None.
