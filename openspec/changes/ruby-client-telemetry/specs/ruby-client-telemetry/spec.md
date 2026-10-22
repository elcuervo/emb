# Spec Delta

## Purpose

Lets an application see, in its own OpenTelemetry backend, what the `emb` Ruby
gem costs: per-call latency including the deferred (`lazy: :multi` / `:batch`)
coalescing queue, the shape of each resolved batch, and whether its shares
actually ran in parallel.

## ADDED Requirements

### Requirement: Telemetry is optional and needs no configuration

The gem SHALL NOT depend on the OpenTelemetry gems at runtime, and SHALL load them
only if the host application has installed them. Without them every signal path
SHALL return immediately and SHALL NOT change results, raise, log, or open
connections. With them installed the gem SHALL emit through the configured
provider, and SHALL NOT introduce a gem-specific setting to enable, disable,
filter, or sample signals. `OTEL_SDK_DISABLED` remains the standard off switch.

#### Scenario: OpenTelemetry not installed

- **WHEN** an application calls `Emb[:minilm]["hello"]` with no OpenTelemetry gem installed
- **THEN** the call returns the same value as before this capability and emits nothing

#### Scenario: Installed but no SDK configured

- **WHEN** the OpenTelemetry gems are installed and the application has configured no provider
- **THEN** the API's no-op providers absorb the signals and the call still returns the same value

#### Scenario: SDK configured

- **WHEN** an application has installed the gems and an SDK with an exporter
- **THEN** every embedding call emits the spans and metrics described by this capability without further gem configuration

### Requirement: One span per logical embedding call

The gem SHALL emit an `emb.embed` span for each logical call (`Proxy#[]`,
`Proxy#image`, and `eval`/`evalsha`), carrying attributes for the model, the
operation kind, the requested reply format, the text count, and the client's
batching mode. Under the
eager mode the span SHALL cover the command's round trip. Under a deferred mode
the span SHALL begin at that call's first deferral and end when the value
resolves, so its duration includes the coalescing queue.

#### Scenario: Eager call

- **WHEN** a client with `lazy: false` embeds one text
- **THEN** exactly one `emb.embed` span is emitted, with `emb.mode` = `eager`, and its duration covers the `EMB` command

#### Scenario: Deferred call

- **WHEN** a client with `lazy: :batch` defers an embedding and consumes the value later
- **THEN** one `emb.embed` span is emitted whose duration starts at the deferral and ends at resolution, not at the command alone

#### Scenario: Duplicate deferred calls

- **WHEN** the same model, text, and format are deferred more than once in one scope
- **THEN** one `emb.embed` span is emitted for the resulting single server item, and no additional command is sent

### Requirement: Batch and share spans expose parallelism

The gem SHALL emit one `emb.batch` span per forced batch per client, carrying the
share count, text count, and worker count used, and one `emb.share` span per
command actually sent, carrying the model and the instance that served it. Share
spans SHALL be children of their batch span. Under `lazy: :batch` concurrently
dispatched shares SHALL be recorded with overlapping time ranges so a
flame-graph consumer can see the overlap; under `lazy: :multi` they SHALL be
sequential.

#### Scenario: Concurrent shares under batch mode

- **WHEN** a `lazy: :batch` scope resolves into two shares that both take measurable time
- **THEN** the two `emb.share` spans overlap in time and the batch span's duration is less than the sum of the share durations

#### Scenario: Sequential shares under multi mode

- **WHEN** a `lazy: :multi` scope resolves into more than one command
- **THEN** the `emb.share` spans do not overlap

### Requirement: Deferred items that were never sent are not reported as inferences

When a deferred item resolves without being part of any sent command — for example
after a serial-path batch failure — the gem SHALL record that outcome on its
`emb.embed` span and SHALL NOT emit a share duration or count it as a dispatched
text for that call.

#### Scenario: Batch fails before a later share is sent

- **WHEN** the first command of a `lazy: :multi` batch fails and the remaining deferred items resolve to the default value
- **THEN** those items' `emb.embed` spans report that they were not dispatched and contribute no inference duration

### Requirement: Client metric set

The gem SHALL emit these metrics, each carrying a `model` attribute where the
value is model-specific and a `mode` attribute otherwise:

| Metric | Type | Unit | Attributes |
|---|---|---|---|
| `emb.client.embed.duration` | histogram | s | model, mode, format |
| `emb.client.queue.duration` | histogram | s | mode |
| `emb.client.share.duration` | histogram | s | model, instance |

#### Scenario: Deferred mode records queue and share duration

- **WHEN** a deferred call waits 3 ms before its share is dispatched and that share takes 12 ms
- **THEN** one `emb.client.queue.duration` observation near 3 ms and one `emb.client.share.duration` observation near 12 ms are recorded

#### Scenario: Eager mode records neither queue nor share duration

- **WHEN** a client with `lazy: false` embeds a text
- **THEN** `emb.client.embed.duration` is recorded, and neither `emb.client.queue.duration` nor `emb.client.share.duration` is

### Requirement: Batch shape and concurrency metrics

The gem SHALL record one set of batch observations per forced batch, and SHALL
maintain a live count of shares currently dispatched:

| Metric | Type | Unit | Attributes |
|---|---|---|---|
| `emb.client.batch.duration` | histogram | s | mode |
| `emb.client.batch.shares` | histogram | {share} | mode |
| `emb.client.batch.parallelism` | histogram | 1 | mode |
| `emb.client.shares.in_flight` | up-down counter | {share} | — |

#### Scenario: Parallelism value for a concurrent batch

- **WHEN** a batch dispatches two shares of 10 ms each and completes in 11 ms
- **THEN** one `emb.client.batch.parallelism` observation is recorded near 2, and one `emb.client.batch.duration` observation near 11 ms

#### Scenario: Live concurrency

- **WHEN** shares are dispatched and later complete
- **THEN** `emb.client.shares.in_flight` rises for the duration of dispatch and returns to its previous value, including when a share raises

### Requirement: Parallelism is a duration ratio

`emb.client.batch.parallelism` SHALL be the sum of a batch's share durations
divided by that batch's wall duration, computed once per force from the same
timings that produce the share durations. A fully serial batch SHALL report near 1
and N concurrently running shares SHALL report near N.

#### Scenario: Serial batch

- **WHEN** a `lazy: :multi` scope sends three shares one after another
- **THEN** its parallelism observation is near 1

### Requirement: Histogram boundaries fit the instrument's unit

Every client histogram SHALL declare recommended bucket boundaries matched to its
unit — 1 ms to 30 s for durations, count-scale boundaries for `{share}`, and
ratio-scale boundaries for the dimensionless parallelism metric — instead of
leaving consumers to the SDK's millisecond-scale defaults. Because the metrics SDK
does not apply instrument advisories yet, the documentation SHALL show the SDK
views that apply them per unit.

#### Scenario: Documented per-unit views

- **WHEN** an application adds the documented views before its first embedding call
- **THEN** duration histograms carry boundaries between 0.001 s and 30 s, the share-count histogram carries count-scale boundaries, and the parallelism histogram carries ratio-scale boundaries

### Requirement: Attributes are bounded and carry no content

Emitted attributes SHALL be limited to model name, operation kind, batching mode,
reply format, text counts, share and worker counts, queue time, dispatch outcome,
instance identifier, and error status. They SHALL NOT include input texts, image
bytes, embedding values, user or request identifiers, or the list of models in a
multi-model batch.

#### Scenario: Text content never leaves the process

- **WHEN** any embedding call is instrumented
- **THEN** no emitted span or metric attribute contains the input text, image bytes, or vector values

### Requirement: Telemetry cannot alter command behavior

Instrumentation SHALL NOT change command arguments, results, ordering, retry
behavior, or failure semantics, and SHALL NOT hold a batch's dispatch open. A
failing span processor, exporter, or attribute conversion SHALL be contained so
the caller still receives the same value or the same error it would have received
without telemetry.

#### Scenario: Span processor raises

- **WHEN** the application's span processor raises during span completion
- **THEN** the embedding call returns its normal value and no additional server command is sent
