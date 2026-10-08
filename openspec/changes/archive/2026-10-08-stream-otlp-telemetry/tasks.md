# Tasks

## 1. Snapshot seam and metric model

- [x] 1.1 Define the telemetry package's snapshot input (plain values: uptime, active requests, connections, goroutines, RSS/heap/cpu, net in/out, models loaded, cache hits/misses/evictions, truncation counters, and per-model requests/tokens/errors) in `internal/telemetry`, importable with `CGO_ENABLED=0`. Verify with `CGO_ENABLED=0 go build ./internal/telemetry/`.
- [x] 1.2 Map that input to the OTLP metric set in one place (names, kinds Sum/Gauge, units, descriptions) so encoding and delta logic share one definition. Verify with a table test that every required `emb.*` and `process.*` metric in `specs/telemetry-export/spec.md` is present exactly once.
- [x] 1.3 Add a server method that populates the telemetry snapshot from the existing aggregation (`infoSnapshot()` plus the resource sample), so `INFO`, `EMB.STATS`, and telemetry read one source. Verify with a test asserting the snapshot's `total_requests`/`total_tokens`/`models_loaded` equal what `EMB.STATS` and `INFO` report for the same server state.

## 2. OTLP/HTTP encoding

- [x] 2.1 Implement OTLP JSON encoding of resource → scope → Sum/Gauge data points, including resource attributes (`service.name`, `service.version`, `host.name`, `process.pid`, `process.executable.name`) and the per-model `model` datapoint attribute. Verify with a golden-payload test that unmarshals the produced JSON and asserts the exact structure for a fixed snapshot.
- [x] 2.2 Implement delta conversion for monotonic sums, keeping the previous snapshot and emitting the increase per interval; emit no counter delta on the first tick after start and never emit a negative delta. Verify with table tests covering interval increase, first tick, and counter reset.

## 3. Exporter runtime

- [x] 3.1 Implement configuration resolution with precedence flags > YAML `telemetry:` block > `OTEL_*` environment variables, `OTEL_SDK_DISABLED=true` override, and configurable request headers. Verify with a table test over combinations of the three sources and the disabled flag.
- [x] 3.2 Implement the periodic reader: one export in flight at a time (skip a tick when the previous send has not returned), a bounded HTTP timeout, drop-on-failure, and failure logging on state transitions only. Verify with a fake clock and a gated fake sender that a hung send causes ticks to be skipped and the server-side snapshot call is never blocked, and that N consecutive failures log fewer than N times.
- [x] 3.3 Implement `Start(ctx)`, `Shutdown(ctx)`, and `Close()` with a final best-effort export bounded by the shutdown context, and a failed final flush that is logged and ignored. Verify with tests that shutdown performs one final send within the deadline and that an unreachable endpoint does not delay or error shutdown.

## 4. Server and configuration wiring

- [x] 4.1 Add the `telemetry:` fields to `internal/config` and the `-otel-endpoint` / `-otel-interval` flags. Verify with config-parsing tests covering the YAML block and the flags.
- [x] 4.2 Add a server `Option` that constructs and starts the exporter when an endpoint resolves, and wire `Shutdown`/`Close` to stop it in the documented order (after request drain and cache snapshot, before closing connections). Verify with tests that a server built with no endpoint starts no exporter and attempts no send, and that a server built with an endpoint using a fake exporter calls Stop on shutdown.
- [x] 4.3 Wire the resolved configuration through `cmd/emb/main.go`. Verify by building with `just build` and running the binary against a local `httptest` endpoint to confirm one OTLP POST per interval with a valid body.

## 5. Observability surface and documentation

- [x] 5.1 Register `otel_enabled`, `otel_endpoint`, and `otel_interval` as read-only parameters in the `CONFIG` registry. Verify with tests that `CONFIG GET otel*` returns them and `CONFIG SET` on each returns an error without changing the running configuration.
- [x] 5.2 Document the `telemetry:` YAML block, the `OTEL_*` variables and their precedence, and a Datadog recipe (local Agent `:4318`, or the direct intake endpoint with the `dd-api-key` header and delta temporality) in `docs/configuration.md` and `docs/operations.md`. Verify every documented command and config snippet runs as written.
- [x] 5.3 Confirm the exporter adds no dependency: verify `git diff go.mod go.sum` is empty and record the `CGO_ENABLED=0 go build ./internal/telemetry/` result.

## 6. Integration verification

- [x] 6.1 End-to-end test: start a server with an `httptest` telemetry endpoint, drive `EMB` requests across two export intervals, and assert that the received payloads contain delta request/token sums matching the requests made, gauges for active requests/connections, the resource identity attributes, and per-model series with the `model` attribute.
- [x] 6.2 Run `just test`, `just lint`, and `just deadcode`; add a `github.com/elcuervo/emb/internal/telemetry` floor to `coverage-floors.txt` from a measured `just cover` run, rounding down one point.

## Workflow follow-up

- Archive the change once `openspec validate stream-otlp-telemetry --strict` passes and the project's review requirements are met.
