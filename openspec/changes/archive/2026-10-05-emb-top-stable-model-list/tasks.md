# Tasks

## 1. Sampler: last-known per-model samples

- [x] 1.1 In `internal/embtop/stats.go`, keep the previous `ModelPoint` and raw `ModelStats` for a model missing from `res.PerModel`, and flag that poll's sample stale instead of dropping it; add a `stats_test.go` case asserting the values persist and the flag is set, then clear on the next poll.
- [x] 1.2 Expose the stale flag on the value the dashboard reads (`LatestModels`) and assert it in `stats_test.go`.

## 2. One list with the activity strip in the row

- [x] 2.1 Remove `heatmapView`, `hotRows` and the standalone heatmap panel; render the strip inside `modelsView`'s row using `heatColors`/`heatCellStyle`, keeping the time-direction legend once. Verify `go test ./cmd/emb-top/` and that `hmcheck_test.go` sees strip cells within a model row.
- [x] 2.2 Collapse each row to one line, folding `dim · pooling · quant` (and batching when present) inline when it fits the width budget; verify a test that renders with and without metadata and asserts every row occupies the same number of lines.
- [x] 2.3 Recompute the model-area height from a fixed chrome row count and the strip width from the remaining budget; verify `TestViewFitsWidth` passes at 100/120/160 columns and that the strip shrinks before columns are dropped.
- [x] 2.4 Remove now-unused helpers (`heatW`, `heatContentW`) and update `helpView` to describe the per-row strip; verify `just deadcode` and `just lint` pass.

## 3. Stability guarantees in the renderer

- [x] 3.1 Make `modelOrder` an ever-seen list with a present-model set from `EMB.MODELS`; verify a test that drops a model for one poll and asserts it returns to its prior position, not the end.
- [x] 3.2 Render a stale model's last-known values with a dimmed indicator; verify a test that a missed `EMB.INFO` leaves the row position and height unchanged and every row below it in place.
- [x] 3.3 Single scroll offset clamped in `Update`, and a `rows a–b of n` range indicator; verify a test asserting the indicator text and that the offset cannot exceed the present-row count.

## 4. Headless `-once` ordering

- [x] 4.1 In `internal/embtop/once.go`, reconcile `known` with first-seen order (filter announced, append new) and emit per-model sections in that order; verify a `once_test.go` case with a scripted server whose `EMB.MODELS` order changes each poll, asserting stable section order.

## 5. Integration and recorded surfaces

- [x] 5.1 Add a dashboard render test with more than eight models asserting each model has exactly one row position and every model becomes visible when scrolled; verify with `go test ./cmd/emb-top/ -run Stable`.
- [x] 5.2 Re-record the landing page plate and documentation capture with `just website-topviz` and confirm the referenced files changed and the site's published-tree check passes.
- [x] 5.3 Confirm the website `/stats` view renders the new frames with no code change by running `just website-dev` and loading `/stats`.
- [x] 5.4 Run the full gate: `just test`, `just lint`, `just deadcode`, and `just verify-harness`.

## Workflow follow-up

- Archive the change after review; sync the `emb-top-dashboard` delta into `openspec/specs/`.
