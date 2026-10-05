# Proposal

## Why

`emb-top`'s model list and its activity heatmap are two independent windows over
the same models: the heatmap hard-caps at the first eight models and never
scrolls, while the per-model panel scrolls a different window. A model therefore
sits at one height in the activity block and another in the detail list, models
past the eighth are unreachable, and a single missed `EMB.INFO` reply drops a
row's metadata line and shifts every row below it. The dashboard is meant to be
the steady view of "what models exist and how they are performing", so the list
must be one surface with one order and no layout jumps.

## What Changes

- The dashboard SHALL render **one per-model list**: a single first-seen order
  and a single scroll offset, with each model appearing at exactly one position
  in the whole view.
- The **activity heat strip SHALL move into each model's row** instead of living
  in a separate capped block, so the time history travels with the model and the
  list scales past eight models.
- Model rows SHALL have a **constant height**: a model that misses one
  `EMB.INFO` reply keeps its last-known values and meta, so rows below never
  shift.
- A model that leaves the node and returns SHALL **keep its previous position**
  rather than reappearing at the end.
- The **scroll indicator SHALL name the visible range** (`rows 12–20 of 24`)
  instead of only the window size, and scrolling SHALL reach every model.
- The headless **`-once`** sampler SHALL emit per-model sections in the same
  stable first-seen order, so no surface reorders models.
- The documentation's recorded `emb-top` plate and the website's live dashboard
  SHALL be re-recorded/re-rendered from the new layout (they consume the same
  `View()` through `-frames`).

## Capabilities

### New Capabilities

_(none)_

### Modified Capabilities

- `emb-top-dashboard`: per-model rows become a single scrollable list with
  constant row height, last-known-value carry-over, position retention and a
  range indicator; the activity heatmap becomes a per-row strip rather than a
  separate capped panel; the headless sampling mode gains stable model ordering.

## Impact

- `cmd/emb-top/main.go` — merge `heatmapView`/`hotRows` into the per-model row
  renderer; single scroll window; constant row height; scroll-range indicator.
- `cmd/emb-top/health.go` — unchanged (per-model indicator reused in the row).
- `internal/embtop/once.go` — first-seen model order for `-once` output.
- `internal/embtop/stats.go` — retain last-known per-model sample/meta across a
  missed poll, and model position across removal/re-add (small state addition).
- Tests: `cmd/emb-top/main_test.go` (order, single window, constant row height,
  range indicator), `hmcheck_test.go` (in-row strip), `internal/embtop/once_test.go`.
- `website/tools/topviz/` — re-record the plate and documentation capture
  (`just website-topviz`); the website's `/stats` viewer needs no code change.
- No new dependencies and no new flags; `emb-top` stays a pure RESP2 client.
- Known limit: the website's `/stats` view is read-only (no input path), so it
  shows the list's top window; single-line rows maximize how many models fit.
