# emb-top-dashboard Specification

## Purpose
Live terminal dashboard for a running `emb` node: real-time throughput, per-model metrics, and resource/cache gauges rendered with terminal charts, plus a headless sampling mode for scripts.

## Requirements

### Requirement: emb-top connects to a running node and polls existing commands

`emb-top` SHALL connect to every monitored `emb` node and poll `EMB.MODELS`, `EMB.INFO <model>`, `EMB.STATS` and `MONITOR` on each tick, pipelined in a single round trip per node. `MONITOR` support is required: it is the only source of per-request events, so a node that does not implement it (an older server) surfaces as a poll error and the dashboard reports that node as disconnected rather than silently showing stale data.

#### Scenario: Default local connection

- **WHEN** user runs `emb-top` with no address flags and an `emb` server is running on localhost:6379
- **THEN** the dashboard connects and begins rendering

#### Scenario: Custom address and poll interval

- **WHEN** user runs `emb-top -addr localhost:16379 -interval 2s`
- **THEN** the dashboard polls the given node every 2 seconds

#### Scenario: Poll cycle pipeline

- **WHEN** a poll tick occurs
- **THEN** `EMB.MODELS`, one `EMB.INFO` per loaded model, `EMB.STATS`, and `MONITOR <afterSeq> <limit>` are sent in a single pipelined round trip

#### Scenario: Every node is polled on its own connection

- **WHEN** two or more nodes are monitored
- **THEN** each node receives its own pipelined round trip on its own connection, and no node's reply is parsed as another's

#### Scenario: A node without MONITOR is reported as disconnected

- **WHEN** a monitored node does not implement `MONITOR`
- **THEN** that node surfaces as a poll error and is reported as disconnected rather than rendering stale values

### Requirement: emb-top renders aggregate throughput

`emb-top` SHALL compute request-per-second and token-per-second from deltas of the cumulative `total_requests` and `total_tokens` counters between polls and SHALL render them as scrolling stream charts that auto-scale. With more than one node monitored, the rendered aggregate SHALL be the sum over the monitored nodes, and a node's own rates SHALL be attributed to that node's row.

#### Scenario: Throughput streams render

- **GIVEN** a server under load
- **WHEN** the dashboard has polled at least twice
- **THEN** the `req/s` and `tok/s` streams show live rate values that track the counter deltas divided by elapsed poll time

#### Scenario: Idle node shows zero rates

- **GIVEN** an idle server
- **WHEN** the dashboard polls
- **THEN** `req/s` and `tok/s` read approximately zero

#### Scenario: The aggregate is the fleet sum

- **WHEN** two nodes each report ten requests per second
- **THEN** the aggregate stream reads twenty requests per second, and each node's row reads ten

#### Scenario: A rejoining node does not spike the aggregate

- **WHEN** a node that was unreachable becomes reachable again
- **THEN** its rates are rebased from its new counters instead of being derived against the counters from before the gap

### Requirement: emb-top renders per-model metrics

`emb-top` SHALL discover models via `EMB.MODELS`, poll `EMB.INFO <model>` for each, and render one per-model list — req/s, tok/s, err/s, latency, activity strip and identity metadata (dimension, pooling, quantization) — updating as models are added or removed. Rows SHALL render in a stable first-seen order with constant height and one position per model, one scroll offset SHALL page the whole list, changing values SHALL NOT shift columns, and a returning model SHALL keep its position.

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

### Requirement: emb-top renders resource and cache gauges

`emb-top` SHALL render cache hit ratio and hit/miss/eviction rates from `EMB.STATS` cache counters, and RSS memory, CPU usage (derived from `cpu_user_usec`/`cpu_sys_usec` deltas), connections, active requests, goroutines, and uptime.

#### Scenario: Cache gauges

- **WHEN** the server has a cache configured
- **THEN** the dashboard shows cache hit ratio and recent hit/miss/eviction rates

#### Scenario: CPU gauge

- **WHEN** the node is under inference load
- **THEN** the CPU gauge rises proportionally to the `cpu_user_usec`/`cpu_sys_usec` deltas over the poll interval

### Requirement: emb-top supports authentication and TLS

`emb-top` SHALL support `AUTH` password authentication and TLS transport with flags mirroring the server's supported deployment configurations, applied per monitored node. When a node was discovered by resolving a name to an address, the TLS handshake SHALL verify the certificate against that name rather than the address, and an explicit override SHALL be available for deployments whose certificates carry different names.

#### Scenario: Password-protected node

- **WHEN** user runs `emb-top -password secret` against a node with `requirepass` configured
- **THEN** `emb-top` authenticates on connect and renders normally

#### Scenario: TLS node

- **WHEN** user runs `emb-top -tls` against a node with TLS enabled
- **THEN** `emb-top` connects over TLS and renders normally

#### Scenario: An expanded name still verifies its certificate

- **WHEN** a monitored name resolves to an address and that node serves a certificate for the name
- **THEN** the TLS handshake succeeds by verifying the name, not the address

#### Scenario: Auth failure is a node state

- **WHEN** a monitored node rejects the configured password
- **THEN** that node reports an authentication failure rather than a generic unreachable state

### Requirement: emb-top is resilient to connection loss

`emb-top` SHALL detect a dropped connection per node, show the affected node's connection-lost state with the last successful sample time, and automatically reconnect it on a later poll. An unreachable node SHALL be a fleet state rather than a reason to exit; only a run in which no node could be monitored at all SHALL fail.

#### Scenario: Node restart

- **WHEN** a node stops or restarts while `emb-top` is running
- **THEN** the dashboard shows a connection-lost state for that node without crashing
- **THEN** when the node returns, `emb-top` resumes rendering with counters rebased to the new connection

#### Scenario: Unreachable at startup

- **WHEN** `emb-top` cannot reach any monitored node at startup
- **THEN** `emb-top` exits with a non-zero status and a clear error message

#### Scenario: One unreachable node does not end the run

- **WHEN** exactly one of several monitored nodes is unreachable from startup
- **THEN** the remaining nodes are monitored and rendered, and the unreachable node is shown as such

#### Scenario: Sampling a dead fleet

- **WHEN** no monitored node produced a sample during `-once` sampling
- **THEN** `emb-top` exits non-zero with the connection errors reported

### Requirement: emb-top has a headless sampling mode

`emb-top -once` SHALL print polled metrics as machine-readable lines (one per poll) to stdout and exit after `-samples` polls, without rendering the TUI, so scripts and tests can consume the same metrics. Each line SHALL carry the fleet aggregate followed by one section per monitored node, and the per-model sections within a node SHALL appear in a stable first-seen order that does not follow the server's per-call `EMB.MODELS` enumeration order.

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

#### Scenario: Every monitored node is sampled

- **WHEN** several nodes are monitored
- **THEN** each printed line carries one section per node, and each node's section carries its own metrics

### Requirement: emb-top keybindings

`emb-top` SHALL support keyboard controls: `q` to quit, `p`/`space` to pause and resume polling, `r` to reset the visible window, and arrow/`j`/`k` to move within the visible list. In the fleet view the movement SHALL select a node row, entering SHALL open that node's per-model dashboard, and escaping SHALL return to the fleet view.

#### Scenario: Pause freezes charts

- **WHEN** user presses `p` while polls are running
- **THEN** polling stops and charts stop advancing until resumed

#### Scenario: Selecting a node opens its dashboard

- **WHEN** the user selects a node row and enters
- **THEN** the per-model dashboard for that node is shown

#### Scenario: Returning to the fleet

- **WHEN** the user escapes from a node's dashboard
- **THEN** the fleet view is shown again with its rows in their previous order

### Requirement: emb-top errors surface visually

Recent per-model and aggregate error rates SHALL be visible in the dashboard, with a visual emphasis (color/flash) when error rates rise above zero.

#### Scenario: Errors appear during a spike

- **WHEN** a model starts producing errors
- **THEN** its error counter/rate is highlighted in the dashboard instead of remaining inert text

### Requirement: emb-top consumes MONITOR events

`emb-top` SHALL poll `MONITOR` on its poll cycle (pipelined with `EMB.MODELS`/`EMB.INFO`/`EMB.STATS`), fetching only events newer than the last seen sequence, and SHALL derive per-model and aggregate latency percentiles (p50/p95/p99) from recent events.

#### Scenario: Percentiles from events

- **WHEN** the dashboard has polled `MONITOR` after a window of requests
- **THEN** it renders p50/p95/p99 latency values derived from the event window (falling back to the server's cumulative `avg_latency_us` when no events are available)

#### Scenario: Gap in the ring buffer

- **GIVEN** more requests completed than fit the ring buffer between two polls
- **THEN** the dashboard continues rendering (rates still come from `EMB.STATS` counter deltas) and does not error

### Requirement: emb-top renders a model-activity heatmap

`emb-top` SHALL render each model's request activity over a time window as a strip inside that model's row in the single per-model list, with the model's current req/s beside it and a legend naming the scale and time direction, alongside the throughput streams and gauges. There SHALL be no separate capped activity panel: scrolling SHALL reach every model's strip at that model's one row position.

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

### Requirement: emb-top streams complete dashboard frames headlessly

`emb-top` SHALL support a headless streaming mode that renders the dashboard's
own frames to stdout on the poll interval, without a TTY, without the alternate
screen, and without reading input. Each emitted frame MUST be the dashboard's
complete current frame — the same content the TUI paints — so a consumer needs
no terminal emulation and can display only the most recent frame and still be
correct. The mode MUST preserve the dashboard's colors when stdout is not a
terminal rather than letting the output profile strip them, and it SHALL keep
polling and resume after a connection loss instead of exiting. When several nodes
are monitored, each frame SHALL be the fleet frame, so a consumer sees every node
without any terminal or input handling.

#### Scenario: Frames stream on the poll interval

- **WHEN** the mode is run against a running node
- **THEN** it prints one complete frame per poll interval until it is stopped

#### Scenario: No terminal is required

- **WHEN** the mode's output is a pipe rather than a terminal
- **THEN** it still emits the dashboard's frames, with color, and opens no pty or alternate screen

#### Scenario: A frame is complete

- **WHEN** a consumer discards every frame but the latest
- **THEN** the latest frame alone renders the dashboard's current state, because each frame is self-contained

#### Scenario: The node restarts under the stream

- **WHEN** the node stops and returns while the mode is running
- **THEN** the mode keeps running, reports the connection loss in its frames, and resumes rendering with counters rebased

#### Scenario: A fleet frame shows every node

- **WHEN** several nodes are monitored and one becomes unreachable
- **THEN** each frame contains every node's row, with the unreachable node shown as such, and the stream keeps running

### Requirement: emb-top renders a health status banner

`emb-top` SHALL derive one overall health status from its polled metrics — `healthy`, `degraded`, `critical`, or `no data`. The verdict SHALL be driven only by actionable signals: connection state, error ratio, p95 latency relative to the session baseline, and CPU usage. Cache hit rate SHALL render as an informational chip that cannot raise the status. Thresholds SHALL be fixed built-in defaults; the change introduces no new flags. With several nodes monitored, the banner SHALL carry the fleet verdict, and each node's row SHALL carry that node's verdict derived by the same rules.

#### Scenario: Healthy idle node

- **GIVEN** a reachable node with no recent errors, low CPU, and no connection loss
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

#### Scenario: Low cache hit rate does not degrade health

- **WHEN** the cache hit rate is below any operational threshold while connection, error ratio, p95 latency and CPU are healthy
- **THEN** the banner still shows `healthy` and the cache chip is rendered without severity

#### Scenario: Fleet and node verdicts coexist

- **WHEN** several nodes are monitored and one is degraded while the others are healthy
- **THEN** the banner verdict reflects the fleet and that node's row shows its own degraded verdict

#### Scenario: CPU is measured against the node's own parallelism

- **WHEN** the fleet spans machines with different CPU counts
- **THEN** each node's CPU usage is derived from that node's own reported processor count rather than the machine running `emb-top`

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

### Requirement: emb-top aligns its gauges to the per-model row grid

`emb-top` SHALL lay the cache, CPU and memory gauges on the same column grid as
the per-model rows: the cache gauge under the identity block, the CPU gauge under
the metric block, and the memory gauge under the activity strip, each bar
spanning the columns of the zone above it. The grid SHALL be a function of the
terminal width alone, so it does not shift between rows or between polls.

#### Scenario: Gauges share the row's columns

- **WHEN** the dashboard renders per-model rows and the gauges below them
- **THEN** each gauge bar starts at the same column as the row zone it sits under and spans that zone's width

#### Scenario: The grid does not move when latency events lapse

- **GIVEN** a model rendering its p50/p95 columns and a model falling back to its average latency
- **WHEN** both rows repaint on the same poll
- **THEN** their activity strips start at the same column and the gauges stay on the same grid

#### Scenario: Narrow terminal drops columns without breaking the grid

- **WHEN** the terminal is too narrow for every metric column
- **THEN** the grid drops trailing metric columns, and the rows and gauges still share one grid without overflowing the terminal width

### Requirement: emb-top keeps its readouts on fixed columns

`emb-top` SHALL render every dynamic numeric readout — the health banner chips,
the header's uptime and model count, the gauge captions, the connection line and
the event ticker — in a fixed-width column, so a value growing under load moves
no glyph and no text after it. The health status label SHALL occupy a fixed width
so a verdict change does not shift the chips that follow it.

#### Scenario: Banner chips do not shuffle under load

- **WHEN** the error ratio, p95 latency, CPU and cache values grow between polls
- **THEN** every banner chip keeps its column and no chip after a growing value moves

#### Scenario: Gauge values keep their column

- **WHEN** a gauge's value grows (for example `0.0%` to `63.9%`)
- **THEN** the number and its unit stay in the same columns

#### Scenario: A status change does not shift the chips

- **WHEN** the banner's verdict changes between statuses of different text width
- **THEN** the chips after the verdict keep their columns

#### Scenario: A narrow band shrinks the value column

- **WHEN** a gauge's band is too narrow for the full-width value column
- **THEN** the value column shrinks with the band and the line does not overflow the terminal

### Requirement: emb-top pins each labelled metric's label column

`emb-top` SHALL render every labelled per-model metric (`p50`, `p95`, the `avg`
fallback and `err`) as a fixed label followed by its value, so the label keeps
its column whatever the value's width and whichever metric the row is showing. A
model with no recent requests SHALL therefore align its `avg` label with a busy
model's `p50` label.

#### Scenario: An idle row lines up with a busy row

- **WHEN** one model renders `avg` from its lifetime average and another renders `p50` from recent events
- **THEN** both labels start at the same column, and their `err` labels do too

#### Scenario: A growing value moves no label

- **WHEN** a metric's value grows between polls (for example `3µs` to `16.9ms`)
- **THEN** its label and the labels after it keep their columns
