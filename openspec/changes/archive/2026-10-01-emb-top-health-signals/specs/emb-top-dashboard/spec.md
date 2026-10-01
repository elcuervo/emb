# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders per-model metrics

`emb-top` SHALL discover models via `EMB.MODELS`, poll `EMB.INFO <model>` for each, and render per-model req/s, tok/s, err/s, avg latency, and identity metadata (dimension, pooling, quantization), updating as models are added or removed. Rows SHALL render in a stable order that does not change as traffic rates fluctuate, and changing values SHALL NOT shift a row's columns horizontally.

#### Scenario: Per-model rates

- **WHEN** a model is loaded and receives request traffic
- **THEN** its panel shows req/s, tok/s, and err/s computed from `EMB.INFO` counter deltas plus its lifetime avg latency

#### Scenario: Model appears after start

- **WHEN** a model is loaded after `emb-top` has started
- **THEN** the dashboard discovers it on a subsequent `EMB.MODELS` poll and starts tracking it without restarting

#### Scenario: Model list scrolls

- **WHEN** more models are loaded than fit the panel height
- **THEN** the per-model panel scrolls via keyboard with an indicator of hidden/visible rows

#### Scenario: Rows do not reorder under fluctuating traffic

- **GIVEN** two or more loaded models whose req/s rise and fall between polls
- **WHEN** the dashboard repaints on the next poll
- **THEN** each model keeps the same row position it had on the previous poll
- **AND** the row order follows the server's `EMB.MODELS` order

#### Scenario: Changing values do not shift columns

- **WHEN** a per-model rate or latency value changes width (for example `9.9` to `10.0`)
- **THEN** the row's trailing columns keep their horizontal positions

### Requirement: emb-top renders a model-activity heatmap

`emb-top` SHALL render a heatmap of per-model request activity over a time window (rows = models in stable server `EMB.MODELS` order, columns = recent polls, cell color = req/s) and a latency-percentile chart, alongside the existing throughput streams and gauges.

#### Scenario: Heatmap reflects traffic

- **WHEN** one model receives traffic and another is idle
- **THEN** the heatmap SHALL show a colored column band on the busy model's row and blank/near-blank cells on the idle row

#### Scenario: Latency chart renders

- **WHEN** events are available
- **THEN** the latency chart advances per poll and auto-scales

#### Scenario: Heatmap rows stay stable

- **WHEN** the busiest model changes between polls
- **THEN** the heatmap rows keep their stable server order and no row migrates

## ADDED Requirements

### Requirement: emb-top renders a health status banner

`emb-top` SHALL derive one overall node health status from its polled metrics — `healthy`, `degraded`, `critical`, or `no data` — and SHALL render it prominently with reason chips for the signals that drive it: error ratio, p95 latency, CPU usage, cache hit rate, active requests, and connection state. Thresholds SHALL be fixed built-in defaults; the change introduces no new flags.

#### Scenario: Healthy idle node

- **GIVEN** a reachable node with no recent errors, low CPU, and healthy cache behavior
- **WHEN** the dashboard renders
- **THEN** the banner shows `healthy`

#### Scenario: Errors degrade health

- **WHEN** the recent error ratio rises above the degraded threshold
- **THEN** the banner shows `degraded` or `critical` and includes an error reason chip

#### Scenario: Connection loss is not healthy

- **WHEN** the dashboard cannot reach the node
- **THEN** the banner does not show `healthy`, and the connection state is called out

#### Scenario: No samples yet

- **GIVEN** the dashboard has polled fewer than two times
- **WHEN** it renders
- **THEN** the banner shows `no data` rather than a health verdict

### Requirement: emb-top renders latency as a percentile band

`emb-top` SHALL render recent request latency as a shaded band spanning p50 to p99 with a p95 line, so the spread of the distribution is visible rather than a single percentile. Percentiles are derived from the bounded `MONITOR` event window.

#### Scenario: Band renders from events

- **WHEN** the event window contains successful requests
- **THEN** the latency visualization shows a p50–p99 band and a p95 line that advance per poll

#### Scenario: Tail latency widens the band

- **WHEN** the spread between p50 and p99 grows
- **THEN** the rendered band visibly widens

#### Scenario: No events yet

- **WHEN** no successful events are available
- **THEN** the band renders empty or flat without erroring

### Requirement: emb-top renders per-model health indicators

`emb-top` SHALL render a per-model health indicator derived from the model's recent error rate and latency: a status dot and a direction arrow showing whether latency or errors are rising. A model with no recent traffic SHALL render an explicit idle state rather than reading as unhealthy.

#### Scenario: Busy healthy model

- **WHEN** a model has recent traffic and no errors
- **THEN** its row shows a healthy indicator and no error emphasis

#### Scenario: Model errors are highlighted

- **WHEN** a model produces errors in the recent window
- **THEN** its row's indicator and error segment are visually emphasized

#### Scenario: Idle model

- **WHEN** a loaded model has no recent traffic
- **THEN** its row shows an idle state instead of a healthy or error verdict
