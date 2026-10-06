# Design

## Context

The model row is built by appending segments left to right: a status dot, a
13-column name, a 16-column metadata block, then rate, token, latency and error
segments that are only appended while they fit, and finally the activity strip
sized to whatever columns remain. The gauges are built independently: three
`ntcharts` bars each resized to `(width-8)/3` and joined with a two-column pad.

The append-if-it-fits rule makes the row's strip start depend on which segments
are present, and the latency segment count depends on whether recent `MONITOR`
events exist: p50+p95 when they do, a single `avg` when they do not. So the strip
starts twelve columns earlier on a poll with no events. The grid is not currently
a function of width alone.

## Goals / Non-Goals

**Goals:**

- One column grid for the model rows and the gauges.
- A grid that depends only on the terminal width, not on polled values.
- The existing width-fitting behavior preserved: narrow terminals drop metric
  columns rather than wrapping, and the strip always renders.

**Non-Goals:**

- Rearranging the gauge order or changing what each gauge measures.
- Changing the stream charts, banner or footer layout.

## Decisions

**One `rowLayout` value per render.** Compute the grid once from `m.width` and
pass it to every row and to the gauges. The layout records the identity width,
which metric slots fit, and the resulting strip width.

**Reserve p50 and p95 as fixed slots.** The latency block always claims two
`colLat` columns when they fit; with no events, `avg` fills the first and the
second renders blank. This is what makes the grid width-only, and it is the
smallest fix for the existing strip shift.

**Bands are the row's own zones.** At 120 columns: identity 32, metric 62, strip
23, with the row's existing one-column separator after the identity and its
two-column separator before the strip. The gauge row reuses those exact numbers,
so no new magic constants are introduced and the sum is still the terminal width.

**Gauges keep their order.** `cache`, `cpu`, `mem` map positionally onto
identity, metric and strip. The gauges are node-wide aggregates, so no zone is a
semantic home for any one of them; the alignment is about the shared grid.

## Risks / Trade-offs

- The CPU bar becomes wide (62 columns) and the memory bar narrow (23), so the
  three bars are no longer equal width. That is the point of the change: the bars
  now match the zones above them.
- The `avg` fallback leaves a blank latency slot for the first polls before
  events arrive. It is one poll wide and buys a stable grid.
