# Proposal

## Why

The OTLP exporter reports per-model request, token, and error *counts* but no timing at all, so an operator watching a dashboard cannot tell whether a slow model is queueing, computing, or being called with oversized batches — and cannot see tail latency, because the server keeps no distribution. The server measures far more than it exports: every per-model dispatch-wait, run-time, run-count, session-occupancy, and padding-efficiency number is already rendered by `EMB.INFO` and silently dropped by the exporter, and per-request latency is written to the bounded `MONITOR` ring where nothing but `emb-top` can read it.

## What Changes

- Export the per-model timing and pipeline counters the server already measures as OTLP sums: dispatch wait, inference duration, inference runs, sessions busy/total, padding efficiency, and the scripted-evaluation counters.
- Add one OTLP **latency histogram**, `emb.request.duration{model}`, recorded at the same five points that already record a `MONITOR` event, so request latency distributions (p50/p95/p99) are available to any backend without reading the bounded ring.
- Extend the hand-rolled OTLP/JSON encoder with the histogram wire shape (`explicitBounds`, `bucketCounts`, `count`, `sum`) under delta temporality, matching the existing sum behaviour.
- Relax the "exported values are consistent with `INFO`/`EMB.STATS`" requirement so it binds cumulative counters and gauges to the command aggregation while allowing the histogram to be its own measured signal, and state explicitly that the histogram's population is all inference commands, not the text-pool population behind `emb.requests`.
- No **BREAKING** changes: export stays opt-in, no existing metric is renamed, removed, or changes meaning, and no command's reply changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `telemetry-export`: adds per-model latency and pipeline metrics, adds the request-duration histogram (data type, buckets, temporality, attribution, cardinality bound), and narrows the "consistent with INFO and EMB.STATS" requirement so it no longer forbids a directly measured distribution.

## Impact

- `internal/telemetry/`: new histogram accumulator (`CGO_ENABLED=0`, no `internal/server` import), `Snapshot`/`ModelSnapshot` gain per-model timing and bucket fields, `otlp.go` gains the histogram wire type and delta diffing, `metrics.go` gains the new definitions.
- `internal/server/`: the five `monitor.Add` call sites route through one recording helper (`handleEMB`, `processMultiPair`, `processIMGMultiPair`, `handleIMG`, `runScripted`); `telemetrySnapshot()` widens to carry the new counters and buckets.
- Specs: `telemetry-export` delta only. `EMB.INFO`, `EMB.STATS`, and `MONITOR` replies are unchanged, so `server-stats-observability` and `emb-top-*` are untouched.
- `emb-top` keeps its own 4096-event percentile window; unifying it onto server-side buckets is explicitly out of scope.
