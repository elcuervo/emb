# Proposal

## Why

The per-model rows keep fixed columns, but the rest of the dashboard does not:
the health banner chips, the header, the gauge captions, the connection line and
the event ticker all render their numbers at whatever width the value happens to
be. Under load those values grow (`p95 12.9ms` → `p95 37.0ms`, `cpu 5%` →
`cpu 29%`, `cache 1%` → `cache 36%`), so every chip and field after them slides
sideways on each poll. The recorded take shows the whole banner shuffling.

## What Changes

- Pad every dynamic readout to a fixed width: the banner chips, the header's
  uptime and model count, the gauge caption values, the connection line and the
  ticker's text count and latency.
- Pad the health status label to a fixed width so a verdict change
  (`CRITICAL` → `HEALTHY`) does not shift the chips that follow it.
- Shrink the gauge value column with a narrow band rather than overflowing it.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `emb-top-dashboard`: dynamic readouts must occupy fixed columns so values
  growing under load move no text.

## Impact

- `cmd/emb-top/main.go` — `headerView`, `caption`, `gaugesView`, `tickerView`.
- `cmd/emb-top/health.go` — the signal chips and the banner label.
- `cmd/emb-top/health_test.go`, `cmd/emb-top/width_test.go` — stability tests.
- The re-recorded plate and docs capture.
