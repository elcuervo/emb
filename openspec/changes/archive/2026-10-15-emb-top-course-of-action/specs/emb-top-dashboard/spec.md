# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders resource and cache gauges

`emb-top` SHALL render cache hit ratio and hit/miss/eviction rates from `EMB.STATS` cache counters, and RSS memory, CPU usage (derived from `cpu_user_usec`/`cpu_sys_usec` deltas), connections, active requests, goroutines, and uptime.

CPU usage SHALL be read as a share of the node's own capacity — the same unit as the health banner's CPU chip — and the dashboard SHALL additionally state the CPU the window spent per uncached request, so a node whose traffic is mostly cache-served is not read as idle: the denominator of the CPU readout SHALL be the work that actually ran, not the requests that arrived.

The cache readout SHALL state the cached share of the window's traffic (the portion of requests and texts answered without inference) beside the hit ratio, since the two answer different questions: the ratio describes the entries, the share describes how much of the service's work the cache removed.

#### Scenario: Cache gauges

- **WHEN** the server has a cache configured
- **THEN** the dashboard shows cache hit ratio, the cached share of the window's traffic, and recent hit/miss/eviction rates

#### Scenario: CPU gauge

- **WHEN** the node is under inference load
- **THEN** the CPU gauge rises proportionally to the `cpu_user_usec`/`cpu_sys_usec` deltas over the poll interval

#### Scenario: The CPU readout is the share of the node's capacity

- **WHEN** a node with more than one core reports CPU usage
- **THEN** the gauge and its value SHALL show the share of the node's capacity, matching the banner's chip for the same poll

#### Scenario: Cache-served traffic is not read as idle

- **WHEN** a window's requests are mostly answered from the cache and the CPU is low
- **THEN** the dashboard SHALL state the cached share and the CPU spent per uncached request, so the low CPU is attributable to the cache rather than to an idle node

### Requirement: emb-top renders latency as a percentile band

`emb-top` SHALL render recent request latency as a shaded band spanning p50 to p99 with a p95 line, so the spread of the distribution is visible rather than a single percentile. Percentiles are derived from the bounded `MONITOR` event window.

The band's primary series SHALL remain every completed request — the latency the service actually delivered, cache hits included — and the band SHALL additionally mark the p95 of the requests that were not answered from the cache, so a window that mixes a microsecond cache hit with a second-long inference does not hide which population the tail came from. Both series SHALL be derived from the same event window and the same nearest-rank percentile computation, and the band's grid SHALL remain a function of the terminal width alone.

#### Scenario: Band renders from events

- **WHEN** the dashboard has polled `MONITOR` after a window of requests
- **THEN** it renders a p50–p99 band with a p95 line over every completed request

#### Scenario: Tail latency widens the band

- **WHEN** a window's requests include a large latency spread
- **THEN** the band widens to cover it

#### Scenario: The uncached tail is separable from the service tail

- **WHEN** a window contains both cache-answered and uncached requests with different latencies
- **THEN** the dashboard SHALL render the uncached requests' p95 as its own marked level beside the band, and SHALL state the cached share of the same window beside the advice

#### Scenario: A window with nothing uncached

- **WHEN** every request in the window was answered from the cache
- **THEN** no uncached p95 SHALL be marked, and the band SHALL render without erroring

#### Scenario: No events yet

- **WHEN** the event window is empty
- **THEN** the band renders empty or flat without erroring

### Requirement: emb-top recommends the resource a node needs

Alongside a node's health status, `emb-top` SHALL render an advisory recommendation naming both the resource to change and the action that changes it, derived from that node's own reported CPU budget and admission pressure, its host's CPU throttling and stolen time, its memory and cache-restore headroom, its per-model dispatch, admission and run phases, and the cached share of its traffic. The recommendation SHALL state the counters it is derived from. It SHALL name at most one primary action, and where the evidence also supports a second action it SHALL name the one it rejected and the counter that ruled it out, so the operator sees why not. It SHALL NOT drive the health status, and its thresholds SHALL be built-in defaults introducing no new flags.

Every rule SHALL have a magnitude floor: a claim about the host SHALL require throttling or stolen time that is a material share of the window, measured against the window's own wall time, and SHALL NOT fire while the node carries no inference load, so an idle node with a few milliseconds of steal reports nothing. A node that is holding its whole budget with no caller refused SHALL NOT be told to add CPU.

#### Scenario: A CPU budget that binds is named

- **WHEN** a node holds its whole CPU budget and its admission waits are timing out
- **THEN** the recommendation names CPU and states the budget in use and the number of waits that timed out

#### Scenario: A starving host is told apart from a binding budget

- **WHEN** a node's host is throttling or stealing CPU while the node's budget is not fully held
- **THEN** the recommendation names the host as the constraint rather than the node's own budget

#### Scenario: Cache headroom names memory

- **WHEN** a node's cache-restore headroom is nearly spent and its skipped entries are growing
- **THEN** the recommendation names memory and states the remaining headroom

#### Scenario: Growth at flat load is not a purchase

- **WHEN** a node's memory grows while its offered load and its cache occupancy are flat
- **THEN** the recommendation reports the growth as unexplained rather than recommending more memory

#### Scenario: Quiet signals recommend nothing

- **WHEN** no signal is near its threshold
- **THEN** no recommendation is rendered

#### Scenario: A recommendation does not move the verdict

- **WHEN** a recommendation is rendered for a node whose connection, error ratio and latency are healthy
- **THEN** the node's health status is unchanged, and the recommendation renders without severity

#### Scenario: A trivial amount of stolen time recommends nothing

- **WHEN** a node is carrying no inference load and its host has stolen a few milliseconds over the window
- **THEN** no recommendation SHALL be rendered, rather than naming the host

#### Scenario: The runner-up action is named with its counter

- **WHEN** a node's dispatch wait and its gate wait both contribute to the tail
- **THEN** the recommendation SHALL name the dominant one as the action and SHALL state the rejected one with the counter that ruled it out

## ADDED Requirements

### Requirement: emb-top attributes its tail to the path and phase that caused it

`emb-top` SHALL attribute the window's uncached tail to the phase it was spent in — `dispatch_us`, `admit_us` or `run_us` — and SHALL state the dominant phase with its mean, per model and for the node, beside the counters behind it. The attribution SHALL come from the events' own phases, never from the total latency alone, and SHALL cover only the measured phases: whatever the latency includes beyond them stays unattributed.

#### Scenario: The dominant phase is named with its evidence

- **WHEN** a node's uncached requests wait at the CPU admission gate for most of their time
- **THEN** the dashboard SHALL name the gate wait as the dominant phase, with the mean wait and the counters it was derived from

#### Scenario: Time outside the phases is not assigned to one

- **WHEN** an event's phases sum to less than its latency
- **THEN** the dashboard SHALL attribute only the measured phases and SHALL NOT assign the remainder to any of them

#### Scenario: Attribution is per model as well as per node

- **WHEN** two models on one node carry different phase mixes
- **THEN** the headless output SHALL carry each model's own phases and run count, and the rendered advice SHALL name the model whose tail dominates when one does

#### Scenario: The rendered grid does not move

- **WHEN** the attribution is rendered
- **THEN** the per-model rows SHALL keep the columns the fixed-grid requirement pins, and the per-model phases SHALL be carried in the headless output rather than by moving a column

### Requirement: emb-top withholds an attribution the evidence cannot support

The dashboard SHALL refuse a tail it cannot support. When the window holds too few events, or too few runs, for the p90/p95 to mean anything, it SHALL state the sample count and SHALL NOT recommend a capacity change on that tail. When the server does not report the per-request attribution at all, it SHALL render its existing readouts without the attribution rather than reading the absent phases as zero.

#### Scenario: A thin tail is reported as thin

- **WHEN** the window's p95 rests on very few events or the window's models barely ran
- **THEN** the dashboard SHALL state the sample count and SHALL NOT recommend a capacity change on that tail

#### Scenario: An older server is read without attribution

- **WHEN** a server does not report the per-request attribution fields
- **THEN** the dashboard SHALL render its existing readouts without the attribution, rather than reading the absent phases as zero

### Requirement: emb-top carries its attribution into the headless modes

The `-once` and `-frames` outputs SHALL carry the same attribution per node and per model as the rendered dashboard: the action token, the reason counter, the cached share, the per-path percentiles and the per-phase means. The tokens the headless output already carries SHALL keep their names and their meaning.

#### Scenario: The headless modes carry the attribution

- **WHEN** `-once` or `-frames` output is consumed programmatically
- **THEN** each node's and each model's section SHALL carry its action token, cached share, per-path percentiles and per-phase means

#### Scenario: Existing tokens keep their names

- **WHEN** `-once` output is compared with the previous release's
- **THEN** every token it already carried SHALL still be present under the same name, with the new attribution added beside it
