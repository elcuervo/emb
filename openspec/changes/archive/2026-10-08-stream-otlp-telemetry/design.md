# Design

## Context

See `proposal.md` for motivation and `specs/telemetry-export/spec.md` for the
behavior contract.

The relevant current state:

- `Server.infoSnapshot()` already aggregates one consistent view of the
  server — per-model pool stats, cache stats, resources, network bytes — and is
  the documented single source rendered by `INFO`. `EMB.STATS` reports the same
  underlying measurements through its own writer.
- Resource samplers (`registry.CurrentMemoryUsage`, `ProcessCPUTimes`,
  `HeapInUseBytes`, `NumGoroutines`) are already gathered in `resourceStats()`.
- Background work follows one pattern: `snapshotCoordinator` runs its own
  goroutine with a stop channel and a `Shutdown(ctx)` that respects the
  deadline (`internal/server/snapshot_coordinator.go`).
- `net/http` is already linked in the server binary (`internal/hfhub`), so
  transport adds no new dependency.
- The project hand-rolls small, well-specified wire formats (`internal/resp`)
  and buys hard standards (ONNX, tokenizers). `go.mod` has neither
  `prometheus` nor `opentelemetry`.

## Goals / Non-Goals

**Goals:**

- Emit server metrics over OTLP/HTTP to any OTLP-compatible backend, with no
  backend-specific code in `emb`.
- Keep the exporter's values identical to `INFO`/`EMB.STATS` by construction.
- Zero new third-party dependencies; the package is CGO-free and unit-testable
  without ONNX.
- Never affect request handling or process stability.

**Non-Goals:**

- Traces and logs (separate changes).
- Per-request latency histograms in this change (see Decisions).
- Prometheus text exposition and DogStatsD (a receiver can accept OTLP and
  re-expose it; adding a second protocol is not needed now).
- Configuring the exporter at runtime via `CONFIG SET`.

## Decisions

### D1: In-process exporter, not a sidecar

The exporter is a package inside the server process that reads the server's own
statistics directly.

*Alternative considered:* a Datadog Agent custom check or a RESP-polling sidecar
(both can be built against the existing commands with no server change).
Rejected because the request is that `emb` output the signals, a sidecar is an
extra thing to deploy, and reading the in-process snapshot is both cheaper and
guaranteed consistent. The package still takes a plain snapshot value (not the
`Server`) so a future sidecar can reuse it, and it stays importable with
`CGO_ENABLED=0`.

### D2: Snapshot-driven periodic reader, not hot-path instrumentation

A goroutine wakes once per export interval, calls the existing snapshot
aggregation, converts it to OTLP, and POSTs. No request handler is modified and
no per-request work is added.

*Alternative considered:* OTel SDK meters incremented in the request path.
Rejected for dependency weight, hot-path cost, and the risk of the exported
value drifting from `INFO`/`EMB.STATS`.

*Consequence:* the only latency value the snapshot holds is a per-model average,
which is a poor metric. This change therefore exports no latency metric; a
histogram built from the server's existing bounded `MONITOR` ring is a
follow-up. The spec allows additional `emb.*` metrics, so this is additive.

### D3: OTLP/HTTP with a JSON body, hand-rolled, stdlib transport only

The exporter builds the OTLP `ExportMetricsServiceRequest` JSON by hand and
POSTs it to `<endpoint>/v1/metrics` with `net/http`. This is the same trade-off
the project made for `internal/resp`: a small, stable, well-specified format of
which only a subset (resource → scope → Sum/Gauge data points) is needed.

*Alternatives considered:*

- **OpenTelemetry Go SDK** — the standard, but pulls the metrics SDK,
  semantic-conventions, and protobuf; the binary ships in the Docker image and
  the `emb-server` gem.
- **OTLP protobuf over gRPC** — the generated service stubs pull grpc; the full
  transport stack is unwarranted for a dozen metrics.
- **Prometheus text exposition** — simplest, but pull-only and not OTLP; a
  receiver that wants Prometheus can convert OTLP instead.
- **DogStatsD** — Datadog-native, but backend-specific in a general server.

OTLP/HTTP with JSON is accepted by both the OpenTelemetry Collector and
Datadog's OTLP metrics intake.

### D4: Delta temporality for sums

Each export reports the increase since the previous export for counters; gauges
report their current value. The reader keeps the previous snapshot and
subtracts.

*Alternative considered:* cumulative sums. Datadog "works best with delta
aggregation temporality" and its direct metrics intake rejects cumulative;
cumulative also makes the deployment stateful and can drop the first point after
a restart. Delta is also trivial here because the reader already holds the
previous snapshot. A missed interval simply produces a larger next delta.

The first export after startup emits no counter delta (there is no previous
measurement to subtract from), matching the "no invented values" rule.

### D5: Metric names and attributes

- Process metrics use OTel semantic-convention names: `process.memory.usage`
  (RSS, with the heap fallback already implemented),
  `process.memory.virtual` where available, `process.cpu.time` (delta, user and
  system), `process.uptime`, `system.memory.limit`.
- Server metrics use the `emb.` namespace: `emb.requests`, `emb.tokens`,
  `emb.errors`, `emb.active_requests`, `emb.connections`, `emb.cache.hits`,
  `emb.cache.misses`, `emb.cache.evictions`, `emb.models.loaded`,
  `emb.truncated.texts`, `emb.truncated.pairs`, `emb.truncated.images`,
  `emb.net.input.bytes`, `emb.net.output.bytes`.
- Resource attributes: `service.name` (default `emb`, overridable via
  `OTEL_SERVICE_NAME`), `service.version`, `host.name`, `process.pid`,
  `process.executable.name`.
- Per-model series carry a `model` datapoint attribute; the value is the model
  name from `EMB.MODELS`.

Derived values (`cache_hit_rate`, request rates) are **not** exported: a backend
computes them from the primitives, and emitting them would duplicate the
"measure, never invent" rule.

### D6: Configuration and precedence

Sources, highest precedence first: command-line flags (`-otel-endpoint`,
`-otel-interval`, plus task-level equivalents), the YAML `telemetry:` block, then
the standard `OTEL_*` environment variables. `OTEL_SDK_DISABLED=true` disables
export regardless of source. Arbitrary per-request headers are configurable
(needed for the `dd-api-key` header of Datadog's direct intake). With no endpoint
resolved from any source, the exporter is not constructed.

The resolved configuration is registered read-only in the existing `CONFIG`
registry so `CONFIG GET otel*` reports it (`otel_enabled`, `otel_endpoint`,
`otel_interval`) and `CONFIG SET` rejects it.

### D7: Lifecycle mirrors the snapshot coordinator

The exporter exposes `Start(ctx)`, `Shutdown(ctx)`, and `Close()`. It is wired
into `Server.New` (behind an `Option`) and into `Server.Shutdown` **after**
draining in-flight requests and the cache snapshot, and before closing
connections; `Shutdown` performs one final best-effort export bounded by the
shutdown context. A failed final export is logged and ignored, so it can never
change the exit status.

*Alternative considered:* a fire-and-forget ticker goroutine. Rejected: it would
leak on shutdown and lose the last interval's deltas.

### D8: Failure isolation and bounded cost

One export runs at a time; if the previous export is still in flight when the
next tick fires, the tick is skipped rather than queueing. Sends use an
`http.Client` with a bounded timeout, so a slow endpoint cannot accumulate
goroutines. Failures are dropped and logged on state transitions (`OK → failing`
and `failing → OK`) rather than once per interval. Nothing in the export path
touches a request handler or a lock held by one.

## Risks / Trade-offs

- **Hand-rolled OTLP JSON could drift from the spec** → Restrict the encoder to
  Sum and Gauge data points, and pin it with golden-payload tests asserting the
  exact JSON for a fixed snapshot. If the surface ever grows past Sum/Gauge, the
  SDK becomes the better trade.
- **No latency distribution in v1** → Follow-up consumes the existing `MONITOR`
  ring to build a histogram; additive, no spec change.
- **Cardinality** → Series are bounded by (fixed metric set) × (number of loaded
  models), which the server already bounds.
- **Datadog direct intake specifics** → It requires delta temporality (D4) and
  the `dd-api-key` header, and treats resource attributes as tags only with the
  `dd-otel-metric-config: {"resource_attributes_as_tags": true}` header. All are
  configuration, not code; documented as a Datadog recipe in `docs/operations.md`.
- **Payload size** → Datadog caps direct-intake payloads at 512 KiB compressed;
  the payload here is a few kilobytes, and one export runs at a time.
- **Clock assumptions** → Timestamps use the wall clock at export; deltas come
  from monotonic counters, not from timestamps, so clock adjustments are safe.

## Migration Plan

Additive and inert by default. Deployments that configure nothing behave exactly
as before. To enable: set an endpoint via any source (for Datadog, either a local
Agent's `:4318` receiver or the direct intake endpoint with a `dd-api-key`
header). Rollback is unsetting the endpoint and restarting; no state is
persisted. No data migration.

## Open Questions

- Which additional `emb.*` metrics are worth adding beyond the initial set
  (for example, snapshot success/failure counters) is deferred; the initial set
  is the spec's floor.
- Whether to build the latency histogram from `MONITOR` in a follow-up change.
