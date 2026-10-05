# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders per-model metrics

`emb-top` SHALL discover models via `EMB.MODELS`, poll `EMB.INFO <model>` for each, and render one per-model list carrying req/s, tok/s, err/s, latency, activity strip and identity metadata (dimension, pooling, quantization), updating as models are added or removed. Each model SHALL appear at exactly one position in the whole view: rows render in a stable first-seen client-side order — independent of the server's per-call enumeration order and of traffic — and one scroll offset SHALL page the whole list. A row SHALL keep a constant height whether or not its latest `EMB.INFO` reply arrived, changing values SHALL NOT shift a row's columns horizontally, and a model that leaves the node and returns SHALL keep its previous position.

#### Scenario: Per-model rates

- **WHEN** a model is loaded and receives request traffic
- **THEN** its panel shows req/s, tok/s, and err/s computed from `EMB.INFO` counter deltas plus its lifetime avg latency

#### Scenario: Model appears after start

- **WHEN** a model is loaded after `emb-top` has started
- **THEN** the dashboard discovers it on a subsequent `EMB.MODELS` poll and starts tracking it without restarting

#### Scenario: Model list scrolls

- **WHEN** more models are loaded than fit the panel height
- **THEN** the per-model list scrolls as one unit via keyboard, reaching every model, with an indicator naming the visible range (for example `rows 12–20 of 24`)

#### Scenario: Rows do not reorder under fluctuating traffic

- **GIVEN** two or more loaded models whose req/s rise and fall between polls
- **WHEN** the dashboard repaints on the next poll
- **THEN** each model keeps the same row position it had on the previous poll
- **AND** the order is the client's first-seen order, not the server's per-call enumeration order

#### Scenario: Changing values do not shift columns

- **WHEN** a per-model rate or latency value changes width (for example `9.9` to `10.0`)
- **THEN** the row's trailing columns keep their horizontal positions

#### Scenario: A missed poll does not move rows

- **GIVEN** a loaded model whose `EMB.INFO` reply is absent for one poll
- **WHEN** the dashboard repaints
- **THEN** that model's row keeps its position and height, showing its last-known values, and no row below it shifts

#### Scenario: A returning model keeps its position

- **GIVEN** a model drops out of `EMB.MODELS` for a poll and is announced again
- **WHEN** the dashboard repaints
- **THEN** the model returns to the position it held before it dropped, not the end of the list

### Requirement: emb-top renders a model-activity heatmap

`emb-top` SHALL render each model's request activity over a time window as a strip inside that model's row in the single per-model list, so the strip travels with the model, and SHALL show the model's current req/s beside its strip plus a legend naming the scale and time direction, alongside the throughput streams and gauges. The activity area SHALL NOT be a separate capped panel: there SHALL be no fixed limit on how many models' strips are reachable, and scrolling the list SHALL reveal every model's strip at that model's one row position.

#### Scenario: Heatmap reflects traffic

- **WHEN** one model receives traffic and another is idle
- **THEN** the busy model's row SHALL show a colored strip band and the idle row's strip SHALL be blank or near-blank
- **AND** each row SHALL show that model's current req/s

#### Scenario: Latency chart renders

- **WHEN** events are available
- **THEN** the latency chart advances per poll and auto-scales

#### Scenario: Heatmap rows stay stable

- **WHEN** the busiest model changes between polls
- **THEN** each model's strip stays in its row's stable first-seen position and no strip migrates

#### Scenario: Every model's activity is reachable

- **GIVEN** more models are loaded than fit the panel height
- **WHEN** the user scrolls the list
- **THEN** every loaded model's activity strip becomes visible, with no model hidden behind a fixed panel cap

### Requirement: emb-top has a headless sampling mode

`emb-top -once` SHALL print polled metrics as machine-readable lines (one per poll) to stdout and exit after `-samples` polls, without rendering the TUI, so scripts and tests can consume the same metrics. The per-model sections within a line SHALL appear in a stable first-seen order that does not follow the server's per-call `EMB.MODELS` enumeration order.

#### Scenario: Scripted sampling

- **WHEN** user runs `emb-top -once -samples 5 -interval 1s -addr host:port`
- **THEN** five machine-readable lines of aggregate and per-model metrics are printed and `emb-top` exits 0

#### Scenario: Sampling a dead node

- **WHEN** the node is unreachable during `-once` sampling
- **THEN** `emb-top` exits non-zero with the connection error on stdout/stderr

#### Scenario: Sampling does not reorder models

- **GIVEN** a node whose `EMB.MODELS` reply enumerates models in a different order on each call
- **WHEN** `-once` prints successive lines
- **THEN** the per-model sections keep a stable first-seen order across lines
