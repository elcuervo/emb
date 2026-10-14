# Spec Delta

## ADDED Requirements

### Requirement: emb-top recommends the resource a node needs

Alongside a node's health status, `emb-top` SHALL render an advisory recommendation naming the resource to change, derived from that node's own reported CPU budget and admission pressure, its host's CPU throttling and stolen time, and its memory and cache-restore headroom. The recommendation SHALL state the counters it is derived from. It SHALL NOT drive the health status, and its thresholds SHALL be built-in defaults introducing no new flags.

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
