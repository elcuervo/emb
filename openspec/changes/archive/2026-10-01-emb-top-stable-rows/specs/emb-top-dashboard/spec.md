# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders per-model metrics

`emb-top` SHALL discover models via `EMB.MODELS`, poll `EMB.INFO <model>` for each, and render per-model req/s, tok/s, err/s, avg latency, and identity metadata (dimension, pooling, quantization), updating as models are added or removed. Rows SHALL render in a stable first-seen order — independent of the server's enumeration order — that does not change as traffic rates fluctuate, and changing values SHALL NOT shift a row's columns horizontally.

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
- **AND** the order is the client's first-seen order, not the server's per-call enumeration order

#### Scenario: Changing values do not shift columns

- **WHEN** a per-model rate or latency value changes width (for example `9.9` to `10.0`)
- **THEN** the row's trailing columns keep their horizontal positions

### Requirement: emb-top renders a model-activity heatmap

`emb-top` SHALL render a heatmap of per-model request activity over a time window (rows = models in the stable first-seen order, columns = recent polls, cell color = req/s), showing each model's current req/s beside its strip and a legend naming the scale and time direction, alongside the existing throughput streams and gauges.

#### Scenario: Heatmap reflects traffic

- **WHEN** one model receives traffic and another is idle
- **THEN** the heatmap SHALL show a colored column band on the busy model's row and blank/near-blank cells on the idle row
- **AND** each row SHALL show that model's current req/s

#### Scenario: Latency chart renders

- **WHEN** events are available
- **THEN** the latency chart advances per poll and auto-scales

#### Scenario: Heatmap rows stay stable

- **WHEN** the busiest model changes between polls
- **THEN** the heatmap rows keep their stable first-seen order and no row migrates
