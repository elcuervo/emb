# telemetry-export Specification

## Purpose
Lets an `emb` node emit its metrics as standard OpenTelemetry signals over OTLP/HTTP, so any observability backend (Datadog, Grafana, an OpenTelemetry Collector) can collect them without `emb` knowing about that backend.

## Requirements

### Requirement: Telemetry export is opt-in and disabled by default

The server SHALL run no metrics exporter and initiate no outbound telemetry connection unless a telemetry endpoint is explicitly configured. Enabling telemetry SHALL NOT change any other server behavior.

#### Scenario: No endpoint configured

- **WHEN** the server starts with no telemetry endpoint from any configuration source
- **THEN** no exporter SHALL run and the process SHALL make no outbound telemetry connection

#### Scenario: Endpoint configured

- **WHEN** the server starts with a telemetry endpoint configured
- **THEN** the server SHALL export metrics to that endpoint once per export interval

### Requirement: Telemetry configuration sources and precedence

Telemetry SHALL be configurable through standard OTLP environment variables, a `telemetry:` YAML block, and command-line flags. When the same setting is provided by more than one source, a command-line flag SHALL take precedence over the YAML block, which SHALL take precedence over the environment. `OTEL_SDK_DISABLED=true` SHALL disable export regardless of other sources.

#### Scenario: Environment only

- **WHEN** `OTEL_EXPORTER_OTLP_ENDPOINT` is set and no flag or YAML telemetry block is present
- **THEN** the exporter SHALL use that endpoint

#### Scenario: Standard variables are honored

- **WHEN** `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_METRIC_EXPORT_INTERVAL`, or `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` are set
- **THEN** each SHALL influence the corresponding exporter setting

#### Scenario: Flag overrides environment

- **WHEN** both an `-otel-endpoint` flag and `OTEL_EXPORTER_OTLP_ENDPOINT` are set to different values
- **THEN** the exporter SHALL use the flag value

#### Scenario: Disabled by standard variable

- **WHEN** `OTEL_SDK_DISABLED=true` is set together with an otherwise valid endpoint
- **THEN** no exporter SHALL run

### Requirement: Exported values are consistent with INFO and EMB.STATS

Every exported metric SHALL be derived from the same aggregated server statistics that `INFO` and `EMB.STATS` render. The exporter SHALL NOT compute a metric through a separate path that could disagree with those commands, and SHALL NOT emit a value that was not measured. Per-model series SHALL carry a `model` attribute whose value is the model name reported by `EMB.MODELS`.

#### Scenario: Values agree with EMB.STATS

- **WHEN** an export occurs and `EMB.STATS` is read around the same time
- **THEN** each cumulative counter exported SHALL correspond to the same underlying measurement that `EMB.STATS` reports

#### Scenario: No invented values

- **WHEN** a measurement is unavailable on the host
- **THEN** the exporter SHALL omit that metric rather than emit a placeholder value

#### Scenario: Per-model attribution

- **WHEN** two models have served different numbers of requests
- **THEN** the per-model request series SHALL carry distinct `model` attribute values identifying each

### Requirement: Monotonic sums use delta temporality

Counters SHALL be exported as monotonic sums with delta temporality: each export SHALL report the increase since the previous export, not the cumulative total. Gauges SHALL be exported with their current value. No delta SHALL be negative, and the first export after startup SHALL NOT report a counter delta derived from a total accumulated before the first measurement.

#### Scenario: Delta reflects the interval

- **WHEN** an export interval sees a counter increase by N since the previous export
- **THEN** the exported sum SHALL be N

#### Scenario: Counter reset

- **WHEN** the process restarts and the underlying counter resets to a lower value
- **THEN** the exported delta SHALL NOT be negative

#### Scenario: First export

- **WHEN** the first export runs after a member of the exporter starts
- **THEN** counters with no previous measurement SHALL NOT be exported with an invented delta

### Requirement: Metric names and resource attributes follow conventions

Process-level metrics SHALL use OpenTelemetry semantic-convention names (`process.memory.usage`, `process.cpu.time`, `process.uptime`). Server-specific metrics SHALL use an `emb.` namespace, including at least requests, tokens, errors, active requests, connections, cache hits, cache misses, cache evictions, models loaded, and truncation counters. Each export SHALL carry resource attributes for `service.name`, `service.version`, and `host.name`.

#### Scenario: Process metrics use semantic conventions

- **WHEN** a telemetry export is received
- **THEN** the process memory, CPU, and uptime metrics SHALL be named with the OpenTelemetry semantic-convention names

#### Scenario: Resource identity

- **WHEN** a telemetry export is received
- **THEN** the included resource SHALL identify the service name, the server version, and the host

### Requirement: Export is failure-isolated and bounded

The exporter SHALL NOT block or slow request handling, and SHALL NOT terminate or destabilize the server when the endpoint is unreachable, slow, or returns an error. A send SHALL be bounded by a timeout; an unsuccessful export SHALL be dropped and retried on the next interval, and repeated failures SHALL be logged at a throttled rate rather than once per interval.

#### Scenario: Endpoint unreachable

- **WHEN** the telemetry endpoint is unreachable
- **THEN** the server SHALL continue serving requests normally
- **AND** later export attempts SHALL still occur

#### Scenario: Repeated failures are throttled

- **WHEN** the endpoint fails across many consecutive intervals
- **THEN** the failure SHALL NOT be logged once per interval

#### Scenario: Slow endpoint

- **WHEN** an export send takes longer than the request path tolerates
- **THEN** an in-flight export SHALL time out and SHALL NOT delay request processing

### Requirement: Effective telemetry configuration is observable

`CONFIG GET` SHALL report the effective telemetry configuration, including whether export is enabled, the resolved endpoint, and the export interval, alongside the other server parameters. These parameters SHALL be read-only: `CONFIG SET` SHALL reject them with an error and preserve the running configuration.

#### Scenario: Telemetry parameters are listed

- **WHEN** a client sends `CONFIG GET otel*`
- **THEN** the reply SHALL include the effective telemetry parameters

#### Scenario: Telemetry parameters are read-only

- **WHEN** a client attempts to change a telemetry parameter with `CONFIG SET`
- **THEN** the server SHALL return an error and make no change
