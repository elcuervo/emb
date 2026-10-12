# Spec Delta

## MODIFIED Requirements

### Requirement: Exported values are consistent with INFO and EMB.STATS

Every exported cumulative counter and gauge SHALL be derived from the same aggregated server statistics that `INFO` and `EMB.STATS` render. The exporter SHALL NOT compute a metric through a separate path that could disagree with those commands, and SHALL NOT emit a value that was not measured. Per-model series SHALL carry a `model` attribute whose value is the model name reported by `EMB.MODELS`. The request-duration histogram is exempt from the command-aggregation clause: it is measured directly at the point the server serves each request, so its population spans every inference command and may exceed the text-pool counters behind `emb.requests`.

#### Scenario: Values agree with EMB.STATS

- **WHEN** an export occurs and `EMB.STATS` is read around the same time
- **THEN** each cumulative counter exported SHALL correspond to the same underlying measurement that `EMB.STATS` reports

#### Scenario: No invented values

- **WHEN** a measurement is unavailable on the host
- **THEN** the exporter SHALL omit that metric rather than emit a placeholder value

#### Scenario: Per-model attribution

- **WHEN** two models have served different numbers of requests
- **THEN** the per-model request series SHALL carry distinct `model` attribute values identifying each

#### Scenario: Histogram is a directly measured signal

- **WHEN** a request-duration histogram datapoint is received
- **THEN** its values SHALL originate from latency measured while serving those requests, not from a ratio of command fields

#### Scenario: Histogram population spans inference commands

- **WHEN** one model serves text and image requests within an interval
- **THEN** both SHALL contribute to that single model's histogram, so its count SHALL NOT be expected to equal the model's `emb.requests` delta

## ADDED Requirements

### Requirement: Per-model inference timing metrics are exported

The exporter SHALL export per-model inference timing as cumulative sums: `emb.dispatch.wait` (seconds a request spent waiting for an inference slot), `emb.inference.duration` (seconds of inference work), and `emb.inference.runs` (number of inference runs). Each SHALL carry the `model` attribute and SHALL NOT be pre-averaged; consumers derive mean queue and inference time from the deltas of these series.

#### Scenario: Timing sums are exported per model

- **WHEN** an export interval follows activity on two models
- **THEN** each SHALL carry `emb.dispatch.wait`, `emb.inference.duration`, and `emb.inference.runs` datapoints with its own `model` attribute

#### Scenario: An idle interval exports a zero delta

- **WHEN** a model served no requests during an interval
- **THEN** its timing sums SHALL report a zero delta rather than a fabricated value

#### Scenario: A model without the measured path is omitted

- **WHEN** a loaded model has no embedding inference pool
- **THEN** no inference timing series SHALL be created for it

#### Scenario: Means are not exported

- **WHEN** a consumer needs mean dispatch wait or mean inference duration
- **THEN** it SHALL derive them from the exported sums and run counts, and the exporter SHALL NOT emit a pre-averaged latency series

### Requirement: Per-model session and efficiency gauges are exported

The exporter SHALL export the per-model inference-session gauges `emb.sessions.busy` (sessions currently running work) and `emb.sessions.total` (sessions available), plus the `emb.padding.efficiency` gauge (embedding tokens consumed divided by token slots processed). Each SHALL carry the `model` attribute and SHALL report its current value on every export.

#### Scenario: Session gauges follow the model's state

- **WHEN** every session of a model is running work and an export occurs
- **THEN** `emb.sessions.busy` SHALL equal `emb.sessions.total` for that model's series

#### Scenario: Padding efficiency reflects batch padding waste

- **WHEN** a model's runs pad its shortest sequences to a batch's longest sequence
- **THEN** `emb.padding.efficiency` SHALL be greater than zero and at most one, and SHALL decrease as padding waste grows

### Requirement: Per-model scripted-evaluation metrics are exported

The exporter SHALL export the scripted-evaluation path as cumulative sums `emb.script.dispatch.wait` (seconds), `emb.script.inference.duration` (seconds), and `emb.script.inference.runs`, separate from the embedding-path series. Each SHALL carry the `model` attribute.

#### Scenario: Scripted runs are separate from embedding runs

- **WHEN** a model serves both an `EMB` embedding and an `EMB.EVAL` evaluation
- **THEN** the evaluation's time SHALL contribute only to the `emb.script.*` series and not to `emb.inference.*`

#### Scenario: A model that never ran a script exports a zero delta

- **WHEN** a model's scripted counters remain unchanged across an interval
- **THEN** its script series SHALL report a zero delta

### Requirement: Request latency is exported as a histogram

The exporter SHALL export `emb.request.duration`, an explicit-bucket histogram of completed-request latency in seconds, carrying the `model` attribute. Each datapoint SHALL report a count, a sum, and one bucket count per declared boundary plus an overflow bucket. Temporality SHALL be delta, matching the exporter's sums: no datapoint SHALL be emitted on the first export after startup, and the count and sum SHALL describe the interval alone.

#### Scenario: Latency distribution is exported

- **WHEN** requests complete during an export interval
- **THEN** a datapoint SHALL carry the interval's observation count, the interval's total duration, and per-bucket counts, for each model that served a request

#### Scenario: Bucket boundaries are explicit and ordered

- **WHEN** a histogram datapoint is received
- **THEN** its bounds SHALL be a strictly ascending list and its bucket counts SHALL contain exactly one more entry than there are bounds

#### Scenario: Every inference command contributes

- **WHEN** a request from any of `EMB`, `EMB.MULTI`, `EMB.IMG`, `EMB.IMGMULTI`, `EMB.EVAL`, or `EMB.EVSHA` completes for a model
- **THEN** it SHALL contribute one observation to that model's histogram for each request record the server produces for `MONITOR` on a configured model

#### Scenario: Failed requests are included

- **WHEN** a recorded inference request for a configured model fails
- **THEN** it SHALL still contribute one observation, so latency distributions do not silently drop failures

#### Scenario: Durations above the last boundary overflow

- **WHEN** a request takes longer than the largest declared boundary
- **THEN** it SHALL be counted in the final bucket count

#### Scenario: The first export emits no histogram

- **WHEN** the first export after startup runs
- **THEN** no `emb.request.duration` datapoint SHALL be emitted, so no bucket count is attributed to a period that began before the first measurement

#### Scenario: Only configured models create series

- **WHEN** a request names a model the server has no configured entry for
- **THEN** no `emb.request.duration` series SHALL be created for that name
