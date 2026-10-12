# Design

## Context

Motivation is in `proposal.md`; observable requirements are in `specs/telemetry-export/spec.md`. Current state that shapes the approach:

- `internal/telemetry` is a hand-rolled OTLP/HTTP JSON encoder with exactly two wire shapes — `gauge` and `sum` (`internal/telemetry/otlp.go`). Counters are exported with delta temporality by diffing each tick against the previous `Snapshot`; both `startTimeUnixNano` and the delta source come from `Exporter.prev`/`prevAt`.
- The package builds with `CGO_ENABLED=0` and imports nothing from `internal/server` (`internal/telemetry/snapshot.go` package doc); the server pushes a plain `Snapshot` built by `Server.telemetrySnapshot()`, which reuses `infoSnapshot()` so command and export values cannot drift. `just verify-harness` covers these packages with no ONNX or model.
- Per-model timings already exist as cumulative microsecond counters: `pipeline.Stats` (`DispatchWaitUs`, `RunUs`, `Runs`, `SessionsBusy`, `SessionsTotal`, `PaddingEfficiency`) and `ModelEntry.ScriptDispatch()` for the script path. `EMB.INFO` renders all of them; the exporter ignores all of them.
- Per-request latency is recorded at five `monitor.Add` call sites: `handleEMB`, `processMultiPair`, `processIMGMultiPair`, `handleIMG`, `runScripted`. The ring is bounded (8192) and its `MonitorEvent` has no command label, so it cannot serve cumulative statistics or per-op attribution.
- Model names originate in the request. `maxMonitorModelLen` (128) exists because those names are client-controlled and retained in a bounded ring — the same trust boundary applies to series labels.

## Goals / Non-Goals

**Goals:**

- Add the distribution signal the server currently cannot produce, without a second latency measurement path that could disagree with the ring.
- Export the per-model timing already measured, at no instrumentation cost.
- Keep the exporter's existing contracts: opt-in, failure-isolated, delta temporality, values measured rather than invented.

**Non-Goals:**

- Per-command (`EMB` vs `EMB.IMG` vs `EMB.EVAL`) attribution. Considered and dropped during exploration; the histogram is per model only. A command attribute would need a label at the single `mux.ServeRESP` choke point and 6x the bucket memory.
- Phase decomposition inside a request (tokenization, pooling, reply encoding). The batcher's poll loop and the reply writers would each need timing.
- Changing `EMB.INFO`, `EMB.STATS`, or `MONITOR`, or replacing `emb-top`'s 4096-event percentile window with server-side buckets.
- Per-model request/latency counters for the image path (`image_requests` stays global).

## Decisions

### 1. Cumulative bucket counts in `Snapshot`, diffed by the exporter

The accumulator keeps *cumulative* per-model bucket counts, `Snapshot` carries them, and `encodeRequest` diffs against `prev` exactly as it diffs sums. Rejected alternatives:

- **Drain-on-export** (exporter takes and resets the buckets): loses an interval whenever a send fails or overruns, and interacts badly with the retry-on-next-interval and skip-if-in-flight behaviour the failure-isolation requirement already pins.
- **Per-tick percentile estimation server-side**: needs a retained window per model, a sort or quantile sketch on the export path, and its own memory bound — a second implementation of what `emb-top` already does.

Diffing inherits the existing temporal rules for free: the first tick (`prev == nil`) emits nothing, a model absent from `prev` has no measured delta yet, and a restart resetting counts cannot produce a negative delta.

### 2. Explicit buckets, not an exponential histogram

OTLP offers explicit-bucket and exponential histograms. Explicit wins here: the bounds are a fixed, reviewable ladder tuned to emb's range, the wire shape is a bounds array plus a bucket-count array, and every backend reads it. Exponential histograms would remove boundary tuning but add scale/bucket-index encoding (and backend support is less uniform); emb's latency span is known well enough that tuning is not a burden.

Bounds in seconds — 17 boundaries, 18 buckets, ~2–2.5x steps from 50µs to 10s:

```
0.00005 0.0001 0.00025 0.0005 0.001 0.0025 0.005 0.01 0.025
0.05 0.1 0.25 0.5 1 2.5 5 10
```

The final bucket is the implicit `+Inf` overflow, so requests slower than 10s are still counted. Values are exported in **seconds** (semconv; `process.cpu.time` already exports seconds although `EMB.INFO` reports µs) as doubles, while the accumulated sum is an `int64` of microseconds converted at encode time — no float atomics on the request path.

### 3. One recording helper feeding both the ring and the histogram

`monitor.Add` stays the single source of the request-record population; the five sites call one server helper (`recordRequest(model, latencyUs, failed)`) that appends the ring event and observes the histogram. Both populations are then identical by construction, so the "every inference command contributes" and "failed requests are included" scenarios are properties of the call graph rather than conventions to maintain. The helper is the only place that needs to change if a sixth inference command appears.

Recording at `mux.ServeRESP` instead was rejected: it times only the post-parse portion, and `EMB.MULTI`/`EMB.IMGMULTI` span several models per command, so attribution there is ill-defined.

### 4. Cardinality gated by the configured model set

Observation is skipped when the registry has no entry for the requested name (`Registry.HasModel`, a map lookup). Series therefore number at most (buckets + 2) words times the models the operator configured, no matter what a client sends, and a hostile name can neither allocate a series nor evict a legitimate one. This also keeps the metric population aligned with the per-model series the commands already render.

Consequences, accepted deliberately: a request for an unconfigured name is recorded in `MONITOR` but not histogrammed, and a request served entirely from a restored cache snapshot for a model this process never loaded is histogrammed under its configured name (the gate is configuration, not load state).

### 5. Errors are observed

Failed requests contribute an observation, matching the ring's population. Errors are a real tail in the latency signal, and excluding them would make histogram count disagree with the recorded request count for reasons invisible to the operator. Distinguishing errors in the distribution would need a label, which the no-command-attribute decision already rules out.

### 6. Snapshot shape and aliasing

`ModelSnapshot` gains the per-model timing fields plus `Count`, `SumUs`, and a bucket-count slice. Because the exporter retains `&cur` as `prev` for the next tick, bucket slices MUST be freshly allocated per snapshot — sharing a slice with the live accumulator would let the next tick's observations mutate the baseline it is diffed against.

### 7. Definitions table, units, and names

New per-model sums reuse the existing `metricDef` shape (`emb.dispatch.wait`, `emb.inference.duration`, `emb.inference.runs`, `emb.script.*`); session occupancy and padding efficiency are gauges (`emb.sessions.busy`, `emb.sessions.total`, `emb.padding.efficiency`, unit `1` for the ratio). Histograms get their own definition table so the bounds live with the definition and the number-encoding path stays untouched.

| Exported | Kind | Source |
|---|---|---|
| `emb.dispatch.wait{model}` | Sum (s) | `Stats.DispatchWaitUs` |
| `emb.inference.duration{model}` | Sum (s) | `Stats.RunUs` |
| `emb.inference.runs{model}` | Sum | `Stats.Runs` |
| `emb.sessions.busy{model}` / `.total` | Gauge | `Stats.SessionsBusy/SessionsTotal` |
| `emb.padding.efficiency{model}` | Gauge (1) | `Stats.PaddingEfficiency` |
| `emb.script.dispatch.wait` / `.inference.duration` / `.inference.runs` | Sum | `ScriptDispatch()` |
| `emb.request.duration{model}` | Histogram (s, delta) | new accumulator at the five record sites |

Mean queue time, mean inference time, and mean batch size remain consumer-side divisions of these sums (and of `emb.requests` by `emb.inference.runs`); no pre-averaged series is exported.

## Risks / Trade-offs

- [One atomic add per bucket plus count and sum on the request path, at five sites] → the accumulator is per-model and lock-free (atomic `int64` fields); the added work is a fixed handful of atomic adds appended to a request that just ran an ONNX inference, and `just bench-script` covers the request path.
- [Two latency populations: the histogram spans all six inference commands, `emb.requests` covers the text pool only] → recorded as an explicit scenario in the delta spec so a dashboard author cannot silently divide one by the other.
- [Bucket bounds may mis-fit an unusual workload, wasting resolution] → the overflow bucket keeps the tail countable, and the exact bounds are not fixed by the spec, so they can be retuned without a requirements change.
- [Snapshot aliasing] → bucket slices are allocated per snapshot; the design states this as a requirement of the change rather than a comment.
- [Two percentile implementations once server-side buckets exist (`emb-top` keeps its window)] → accepted; unifying them would require rendering buckets over RESP and is out of scope.
- [Histogram memory] → ~18 int64 buckets plus count and sum per configured model, on the order of 200 B per model; negligible against the model weights.
- [`emb.request.duration` vocabulary overlaps the semconv `http.server.request.duration` naming without being HTTP] → the `emb.` prefix already marks these as server-specific metrics in the existing conventions requirement.

## Migration Plan

Purely additive and confined to telemetry: no flag, no configuration key, no command reply change, and no state to migrate. Deployment is the normal release path; a server with export disabled allocates the accumulator but sends nothing. Rollback is reverting the commit — the only durable artifact is the exported series, which a backend simply stops receiving.

## Open Questions

- Whether `EMB.STATS`/`EMB.MODELS` should later render the buckets so `emb-top` can drop its own percentile window. Deferrable: additive for clients, no impact on this change's specs or tasks.
- Whether the image and scripted paths deserve per-model request counters of their own (today `image_requests` is global and scripted counts are per model without latency attribution beyond `script_run_us`). Deferrable until the histogram shows a distribution worth attributing.
