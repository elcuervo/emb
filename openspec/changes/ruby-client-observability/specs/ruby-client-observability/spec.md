# ruby-client-observability Specification

## Purpose
Specifies the Ruby client's observable data surface: structured debug logging, typed server stats, optional client-side latency metrics, and machine-readable benchmark output.

## ADDED Requirements

### Requirement: Structured debug logging
The Ruby client SHALL emit debug output through a configurable logger instead of writing to stdout directly, and SHALL sanitize text payloads by default.

#### Scenario: Logging routed through the configured logger
- **WHEN** `Emb.configure { |c| c.logger = Logger.new($stdout) }` is set
- **THEN** debug lines SHALL be written via `logger.debug`, not `$stdout.puts`

#### Scenario: Payloads sanitized by default
- **WHEN** a debug line is emitted for an `EMB`/`EMB.MULTI` call
- **THEN** it SHALL include command name, number of args, total text byte count, and round-trip ms
- **THEN** the full texts SHALL be omitted unless `debug_payload: true`

#### Scenario: debug! compat
- **WHEN** `Emb.debug!` is called
- **THEN** the client SHALL enable debug-level logging through the configured logger

### Requirement: Typed info and stats
The client SHALL return `info` and `stats` values with Ruby types inferred from the server field, while passing unknown fields through as strings.

#### Scenario: Known numeric fields are integers
- **WHEN** `Emb.info(:minilm)` is called against a server that reports `dim: 384, requests: 42, avg_latency_us: 2137, batching_max_tokens: 16384, model_bytes: 22972370`
- **THEN** each of those values SHALL be an `Integer`

#### Scenario: Known float field
- **WHEN** `Embb.info(:minilm)` reports `padding_efficiency: 0.8712`
- **THEN** the value SHALL be a `Float`

#### Scenario: Unknown fields pass through
- **WHEN** the server adds a field the client does not know
- **THEN** the value SHALL be returned as a `String`

### Requirement: Optional client-side metrics
When `metrics: true` is configured, the client SHALL accumulate per-model request count, mean latency, p50/p99 latency, and response bytes, and expose them via `client.metrics` without altering embed return values.

#### Scenario: Metrics enabled
- **WHEN** `Emb.configure { |c| c.metrics = true }` is set and the client serves embeddings
- **THEN** `client.metrics` SHALL return a typed hash with per-model count, mean latency, p50/p99, and bytes

#### Scenario: Default off
- **WHEN** `metrics` is not configured
- **THEN** the hot path SHALL add no per-call bookkeeping

### Requirement: Machine-readable benchmark output
`bench/bench.rb` SHALL support a machine-readable output format selected by the `EMB_BENCH_OUTPUT` environment variable, while keeping the human table as the default.

#### Scenario: JSON output
- **WHEN** `EMB_BENCH_OUTPUT=json` is set and the bench runs
- **THEN** the output SHALL be valid JSON including per-scenario embeds, req/s, p50, p99, overhead, and the stability gate verdict

#### Scenario: CSV output
- **WHEN** `EMB_BENCH_OUTPUT=csv` is set
- **THEN** the output SHALL be one CSV header row plus one row per scenario with the same columns as the table