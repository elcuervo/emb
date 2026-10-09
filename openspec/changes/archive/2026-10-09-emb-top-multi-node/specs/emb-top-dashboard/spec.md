# Spec Delta

## MODIFIED Requirements

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
