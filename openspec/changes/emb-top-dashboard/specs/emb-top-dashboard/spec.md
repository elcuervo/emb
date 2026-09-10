## Purpose

Live terminal dashboard for a running `emb` node: real-time throughput, per-model metrics, and resource/cache gauges rendered with terminal charts, plus a headless sampling mode for scripts.

## ADDED Requirements

### Requirement: emb-top connects to a running node and polls existing commands

`emb-top` SHALL connect to a running `emb` server and poll `EMB.MODELS`, `EMB.INFO <model>`, and `EMB.STATS` on each tick, pipelined in a single round trip, with no new server commands required.

#### Scenario: Default local connection

- **WHEN** user runs `emb-top` with no address flags and an `emb` server is running on localhost:6379
- **THEN** the dashboard connects and begins rendering

#### Scenario: Custom address and poll interval

- **WHEN** user runs `emb-top -addr localhost:16379 -interval 2s`
- **THEN** the dashboard polls the given node every 2 seconds

#### Scenario: Poll cycle pipeline

- **WHEN** a poll tick occurs
- **THEN** `EMB.MODELS`, one `EMB.INFO` per loaded model, and `EMB.STATS` are sent in a single pipelined round trip

### Requirement: emb-top renders aggregate throughput

`emb-top` SHALL compute aggregate request-per-second and token-per-second from deltas of the cumulative `total_requests` and `total_tokens` counters between polls and SHALL render them as scrolling stream charts that auto-scale.

#### Scenario: Throughput streams render

- **GIVEN** a server under load
- **WHEN** the dashboard has polled at least twice
- **THEN** the `req/s` and `tok/s` streams show live rate values that track the counter deltas divided by elapsed poll time

#### Scenario: Idle node shows zero rates

- **GIVEN** an idle server
- **WHEN** the dashboard polls
- **THEN** `req/s` and `tok/s` read approximately zero

### Requirement: emb-top renders per-model metrics

`emb-top` SHALL discover models via `EMB.MODELS`, poll `EMB.INFO <model>` for each, and render per-model req/s, tok/s, err/s, avg latency, and identity metadata (dimension, pooling, quantization), updating as models are added or removed.

#### Scenario: Per-model rates

- **WHEN** a model is loaded and receives request traffic
- **THEN** its panel shows req/s, tok/s, and err/s computed from `EMB.INFO` counter deltas plus its lifetime avg latency

#### Scenario: Model appears after start

- **WHEN** a model is loaded after `emb-top` has started
- **THEN** the dashboard discovers it on a subsequent `EMB.MODELS` poll and starts tracking it without restarting

#### Scenario: Model list scrolls

- **WHEN** more models are loaded than fit the panel height
- **THEN** the per-model panel scrolls via keyboard with an indicator of hidden/visible rows

### Requirement: emb-top renders resource and cache gauges

`emb-top` SHALL render cache hit ratio and hit/miss/eviction rates from `EMB.STATS` cache counters, and RSS memory, CPU usage (derived from `cpu_user_usec`/`cpu_sys_usec` deltas), connections, active requests, goroutines, and uptime.

#### Scenario: Cache gauges

- **WHEN** the server has a cache configured
- **THEN** the dashboard shows cache hit ratio and recent hit/miss/eviction rates

#### Scenario: CPU gauge

- **WHEN** the node is under inference load
- **THEN** the CPU gauge rises proportionally to the `cpu_user_usec`/`cpu_sys_usec` deltas over the poll interval

### Requirement: emb-top supports authentication and TLS

`emb-top` SHALL support `AUTH` password authentication and TLS transport with flags mirroring the server's supported deployment configurations.

#### Scenario: Password-protected node

- **WHEN** user runs `emb-top -password secret` against a node with `requirepass` configured
- **THEN** `emb-top` authenticates on connect and renders normally

#### Scenario: TLS node

- **WHEN** user runs `emb-top -tls` against a node with TLS enabled
- **THEN** `emb-top` connects over TLS and renders normally

### Requirement: emb-top is resilient to connection loss

`emb-top` SHALL detect a dropped connection, show a connection-lost banner with the last successful sample time, and automatically reconnect on the next poll.

#### Scenario: Node restart

- **WHEN** the node stops or restarts while `emb-top` is running
- **THEN** the dashboard shows a connection-lost state without crashing
- **THEN** when the node returns, `emb-top` resumes rendering with counters rebased to the new connection

#### Scenario: Unreachable at startup

- **WHEN** `emb-top` cannot reach the node at startup
- **THEN** `emb-top` exits with a non-zero status and a clear error message

### Requirement: emb-top has a headless sampling mode

`emb-top -once` SHALL print polled metrics as machine-readable lines (one per poll) to stdout and exit after `-samples` polls, without rendering the TUI, so scripts and tests can consume the same metrics.

#### Scenario: Scripted sampling

- **WHEN** user runs `emb-top -once -samples 5 -interval 1s -addr host:port`
- **THEN** five machine-readable lines of aggregate and per-model metrics are printed and `emb-top` exits 0

#### Scenario: Sampling a dead node

- **WHEN** the node is unreachable during `-once` sampling
- **THEN** `emb-top` exits non-zero with the connection error on stdout/stderr

### Requirement: emb-top keybindings

`emb-top` SHALL support keyboard controls: `q` to quit, `p`/`space` to pause and resume polling, `r` to reset the visible window, and arrow/`j`/`k` to scroll the per-model panel.

#### Scenario: Pause freezes charts

- **WHEN** user presses `p` while polls are running
- **THEN** polling stops and charts stop advancing until resumed

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

`emb-top` SHALL render a heatmap of per-model request activity over a time window (rows = models, columns = recent polls, cell color = req/s) and a latency-percentile stream chart, alongside the existing throughput streams and gauges.

#### Scenario: Heatmap reflects traffic

- **WHEN** one model receives traffic and another is idle
- **THEN** the heatmap SHALL show a colored column band on the busy model's row and blank/near-blank cells on the idle row

#### Scenario: Latency chart renders

- **WHEN** events are available
- **THEN** the p95 latency stream advances per poll and auto-scales