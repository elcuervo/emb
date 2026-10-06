# Proposal

## Why

Each per-model metric segment was right-aligned as one string, so the label rode
along with the value: a model with no requests renders `avg 0µs` and puts its
`avg` label where a busy model's `p50 16.9ms` puts its `p50` four columns to the
left. The recorded take shows the latency labels stepping sideways between the
idle and loaded models, so the rows do not read as one table.

## What Changes

- Render the labelled metrics (`p50`, `p95`, `avg`, `err`) as a fixed label
  followed by the value, so the label keeps its column whatever the value's width
  or which metric it is.
- Keep the value in the rest of the slot so a growing value still moves nothing
  after it.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `emb-top-dashboard`: a labelled metric's label must keep its column across
  models and across the idle/loaded states.

## Impact

- `cmd/emb-top/main.go` — a `fixedLabeledCol` helper used by `modelRow`.
- `cmd/emb-top/width_test.go` — a label-column test.
- The re-recorded plate and docs capture.
