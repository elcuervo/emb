# Proposal

## Why

`emb` already measures everything an operator needs — requests, tokens, errors,
latency, cache activity, memory, CPU, connections — but only exposes it through
its own RESP commands (`INFO`, `EMB.STATS`, `EMB.INFO`, `MONITOR`). Getting those
numbers into a standard observability backend means running `emb-top` or writing
a bespoke RESP poller; there is no way to point a collector at an `emb` node. An
opt-in OTLP/HTTP exporter gives the metrics a standard wire format so any backend
— Datadog, Grafana, an OpenTelemetry Collector — can pick them up without `emb`
knowing about that backend.

## What Changes

- New `internal/telemetry` package: a periodic reader that builds OTLP/HTTP
  metrics from the server's existing statistics and POSTs them to a configured
  endpoint. **No new third-party dependency** — the transport is
  `net/http` (already linked) and the OTLP JSON encoding is hand-rolled, the same
  decision the project made for `internal/resp`.
- **Snapshot-driven.** The exporter reads the same aggregated statistics that
  `INFO` and `EMB.STATS` render, through a shared collector, so the three cannot
  drift. No request-handler hot-path changes and no per-request cost.
- **Delta temporality** for sums: each tick subtracts the previous snapshot's
  counters. This matches Datadog's OTLP requirement and avoids the
  stateful-collector / dropped-first-point caveats of cumulative export.
- **Standard metric names.** OTel semantic conventions for process metrics
  (`process.memory.usage`, `process.cpu.time`, `process.uptime`); `emb.*` for
  server-specific metrics; `model` as a datapoint attribute and `service.name`,
  `service.version`, `host.name` as resource attributes.
- **Opt-in, env-first configuration.** Standard `OTEL_*` environment variables,
  plus an `-otel-endpoint` flag and a `telemetry:` YAML block. With no endpoint
  configured the exporter is disabled and the server makes no outbound
  connection.
- **Lifecycle mirrors the snapshot coordinator.** Own goroutine, stop channel,
  and a best-effort flush during graceful shutdown inside the existing deadline.
- **Failure isolation.** Bounded send timeout, drop-on-failure, throttled
  logging; the exporter can never stall request handling or crash the server.
- The effective telemetry configuration (endpoint present/absent, interval) is
  visible read-only via `CONFIG GET`.

## Capabilities

### New Capabilities
- `telemetry-export`: opt-in emission of server metrics over OTLP/HTTP, kept
  snapshot-consistent with `INFO`/`EMB.STATS`, with configuration, temporality,
  failure-isolation, and shutdown-flush guarantees.

### Modified Capabilities
- `server-lifecycle`: graceful shutdown additionally flushes pending telemetry
  within the existing shutdown deadline.

## Impact

- New: `internal/telemetry` (exporter, OTLP JSON encoding, config), with tests.
- Modified: `internal/server/server.go` and `internal/server/info.go` (extract a
  shared statistics collector; run the exporter in the server lifecycle),
  `internal/config/config.go` (telemetry config + `-otel-*` flags),
  `cmd/emb/main.go` (wire configuration into the exporter),
  `docs/configuration.md` and `docs/operations.md`.
- Dependencies: none added; `go.mod` and `go.sum` are unchanged.
- Behavior: no change unless telemetry is explicitly configured.
