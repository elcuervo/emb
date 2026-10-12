# Tasks

## 1. Histogram accumulator

- [x] 1.1 Add the bucket boundaries and a lock-free per-model accumulator to `internal/telemetry` (fixed bucket bounds in seconds, `Observe(model, duration)` plus a per-model read used when building a snapshot), gated so only names the accumulator tracks can create series; verify with `go test ./internal/telemetry/ -race -count=1` covering bucket placement, a duration exactly equal to a boundary (counted in that bucket, not the next), a duration above the last boundary landing in the overflow bucket, and concurrent `Observe` calls from many goroutines.
- [x] 1.2 Extend `Snapshot`/`ModelSnapshot` with the bucket counts, observation count and microsecond sum, and add a regression test that a snapshot's bucket slice is independent of later `Observe` calls (mutating the accumulator must not change an already-built snapshot); verify with `go test ./internal/telemetry/ -count=1` inside `nix develop`.

## 2. OTLP encoding

- [x] 2.1 Add the histogram wire shape to `internal/telemetry/otlp.go` (explicit-bucket datapoint with bounds, bucket counts, count, sum, and delta temporality) and diff it against the previous snapshot exactly as sums are diffed; verify with a test asserting the emitted field structure, that bounds are strictly ascending, that bucket counts contain one more entry than bounds, that the first tick emits no histogram datapoint, and that a model absent from the previous snapshot emits none.
- [x] 2.2 Extend the metric definitions with the per-model timing and session series (`emb.dispatch.wait`, `emb.inference.duration`, `emb.inference.runs`, `emb.sessions.busy`, `emb.sessions.total`, `emb.padding.efficiency`, `emb.script.*`), keeping the existing `TestMetricSetCoversSpec` invariant (each name defined once, exactly one of global/model set, description and unit present) and adding the new names to its required list; verify with `go test ./internal/telemetry/ -count=1`.
- [x] 2.3 Document the new metric names, the histogram, and the fact that its population spans every inference command while `emb.requests` covers the text pool, in the telemetry section of `docs/configuration.md`; verify the documented metric names match `metricDefs` (grep each documented name against the encoder) and that the section still reads correctly with the surrounding export-precedence text.

## 3. Server wiring

- [x] 3.1 Route the five `monitor.Add` sites (`handleEMB`, `processMultiPair`, `processIMGMultiPair`, `handleIMG`, `runScripted`) through one recording helper that appends the ring event and observes the histogram only when the registry is configured with that model name; verify with a server test that a successful `EMB`, an `EMB.IMG`, and an `EMB.EVAL` each add one histogram observation, that a failed request for a configured model is still observed, and that a request naming an unconfigured model creates no series while still producing its `MONITOR` event.
- [x] 3.2 Fill the new snapshot fields in `Server.telemetrySnapshot()` from the pool/script counters that `EMB.INFO` already renders; verify by extending `TestTelemetrySnapshotMatchesInfo` so the exported per-model timing and session values equal the values `EMB.INFO` reports for the same model in the same test.

## 4. Integration verification

- [x] 4.1 Export a real payload end to end: start `./bin/emb` inside `nix develop` with `-otel-endpoint` pointed at a local stub OTLP/HTTP receiver, drive `EMB`, `EMB.IMG`, and `EMB.EVAL` traffic across two export intervals, and confirm the received body carries `emb.request.duration` datapoints per model with a non-zero count, a sum consistent with the observed latencies, and bucket counts that add up to the count, alongside the new per-model sums; verify with the receiver's captured bodies.
- [x] 4.2 Run the CGO-free harness check (`just verify-harness`) to confirm `internal/telemetry` still builds and tests without ONNX or a model.
- [x] 4.3 Run `just lint` and `just test` inside `nix develop` and confirm both pass with the new metrics in place.

## Workflow follow-up

- Archive the change per the OpenSpec archive workflow once review is satisfied, syncing the `telemetry-export` delta into `openspec/specs/`.
- Verify the archived `telemetry-export` spec contains the modified consistency requirement and the four added requirements.
