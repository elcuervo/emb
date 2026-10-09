# emb-top-cluster-view Specification

## Purpose
Defines how `emb-top` turns a set of node specifications — fixed addresses and DNS names —
into the set of nodes it monitors, and how it presents and verdicts that fleet.

## Requirements

### Requirement: Node specifications accept addresses and DNS names

`emb-top` SHALL accept node specifications through a repeatable `-node` flag and a
comma-separated `-nodes` flag, each entry being `host:port`, `host` (taking the default port),
or a DNS name. A DNS name SHALL be expanded to one node per address record it resolves to.
Addresses that resolve to the same `host:port` SHALL be monitored once.

#### Scenario: A name expands to every address record

- **WHEN** `emb-top -nodes emb.internal` is run and `emb.internal` resolves to three addresses
- **THEN** all three are monitored as separate nodes

#### Scenario: Addresses and names mix in one fleet

- **WHEN** `-node db-a:16379 -nodes emb.internal` is run
- **THEN** the named node and every expanded address are monitored in one fleet

#### Scenario: Duplicate resolution is monitored once

- **WHEN** two specifications resolve to the same `host:port`
- **THEN** that node appears once in the fleet and is polled on one connection

#### Scenario: A specification that resolves to nothing is an error

- **WHEN** a specification names no address at all
- **THEN** `emb-top` reports a clear error naming the specification, and does not start presenting an empty fleet as healthy

### Requirement: Expanded names are refreshed while watching

`emb-top` SHALL re-resolve every DNS specification on a fixed interval, and immediately after a
node becomes unreachable, and SHALL add a newly resolved address to the fleet without a restart.
A resolution failure SHALL NOT stop the fleet from being monitored or rendered.

#### Scenario: A scaled-out node joins

- **WHEN** a new address appears for a monitored name
- **THEN** the node is polled and rendered from the next poll onwards, without restarting `emb-top`

#### Scenario: A moved node is followed

- **WHEN** a node's address changes while it is unreachable
- **THEN** the next resolution finds the new address and the node is polled there

#### Scenario: A failing resolver does not stop the view

- **WHEN** name resolution fails during a watch
- **THEN** the already-known nodes keep being polled and rendered, and the failure is reported rather than fatal

### Requirement: A node that still answers is never silently dropped

A node that no longer resolves SHALL keep being polled while it answers and SHALL be marked as
orphaned; it SHALL leave the fleet only once it is both unresolved and unreachable for a grace
period. Node rows SHALL keep a stable first-seen order and SHALL NOT reorder as traffic changes.

#### Scenario: A node outlives its DNS record

- **WHEN** a node stops resolving but still answers its poll
- **THEN** its row remains, marked orphaned, so the fleet total still accounts for its traffic

#### Scenario: A decommissioned node leaves

- **WHEN** a node is both unresolved and unreachable beyond the grace period
- **THEN** its row is removed from the fleet

#### Scenario: Rows do not follow traffic

- **GIVEN** several nodes whose request rates rise and fall between polls
- **WHEN** the fleet view repaints
- **THEN** each node keeps the row position it had on the previous poll

### Requirement: Idle and unreachable nodes are distinguished

`emb-top` SHALL report a node that answers but receives no traffic as idle, distinct from
unreachable, and SHALL NOT present an idle node as underutilized or as a load-balancing fault,
because which nodes the clients are configured with is not observable from the nodes.

#### Scenario: An idle node is not a fault

- **WHEN** a monitored node answers every poll with zero request traffic while other nodes are busy
- **THEN** its row reads idle, and the fleet verdict is not degraded for that node alone

#### Scenario: Membership and traffic are reported separately

- **WHEN** the fleet view renders
- **THEN** it reports how many nodes were discovered, how many received traffic, how many are idle, and how many are orphaned

### Requirement: Each node is polled independently

Every node SHALL be polled on its own connection with its own pipelined round trip, deadline and
event cursor. A node that stalls MUST NOT delay another node's poll or the rendered frame; its
row SHALL show its last known values together with the age of that sample.

#### Scenario: A hung node does not stall the fleet

- **WHEN** one node stops responding mid-watch
- **THEN** the other nodes continue to update on schedule and the frame keeps rendering

#### Scenario: A stale row is labelled

- **WHEN** a node's poll fails
- **THEN** its row shows the last known values and how long ago they were sampled

#### Scenario: A restarted node rebases

- **WHEN** a node restarts and its counters reset
- **THEN** its rates are rebased from the new connection rather than derived against the old counters

### Requirement: The fleet verdict is driven by actionable signals only

`emb-top` SHALL render one fleet verdict — `healthy`, `degraded`, `critical` or `no data` — from
cross-node signals that an operator can act on: nodes unreachable, refusing authentication or not
ready; a node's tail latency far above its peers; a node whose cache is cold after a restart; and
load spread beyond what the number of nodes explains. Per-node cache hit rate SHALL NOT drive the
verdict, and thresholds SHALL be built-in defaults requiring no new flags.

#### Scenario: Uneven load degrades the fleet

- **WHEN** one node carries several times its share of fleet requests while its peers are healthy
- **THEN** the verdict is degraded and the reason names skew and the offending node

#### Scenario: A cold cache after a restart is named

- **WHEN** a node has recently restarted and its cache hit rate and entry count are far below its peers'
- **THEN** the verdict degrades with a cold-cache reason, without blaming the clients

#### Scenario: A slow peer is named

- **WHEN** one node's tail latency stays far above its peers' while its load is comparable
- **THEN** the verdict degrades and the reason names that node

#### Scenario: An unreachable node is critical

- **WHEN** no poll to a node succeeds beyond the unreachable threshold
- **THEN** the verdict is critical and the node is named

#### Scenario: Diverse caches are not a fault

- **WHEN** per-node cache hit rates differ while errors, latency, connection and load are healthy
- **THEN** the fleet verdict stays healthy and the cache values render as information

#### Scenario: No samples yet

- **WHEN** fewer than two polls have completed for every node
- **THEN** the verdict is `no data` rather than a health claim

### Requirement: Expected share is one per monitored node

`emb-top` SHALL compute each node's expected share of fleet requests as one divided by the number
of monitored nodes, and SHALL NOT use DNS weights or priorities, because the clients balance
without consulting DNS.

#### Scenario: Share is reported against expectation

- **WHEN** the fleet view renders a node's share of fleet requests
- **THEN** the expectation shown beside it is one divided by the number of monitored nodes

#### Scenario: Weighted records do not change the expectation

- **WHEN** a monitored name resolves through records carrying differing weights
- **THEN** every node's expected share remains equal

### Requirement: The fleet view renders per-node rows above the aggregate

`emb-top` SHALL render one row per node — identity, share of fleet requests against its expected
share, its own verdict and signal chips, and a request-activity strip — and SHALL render the
aggregate above those rows. Selecting a node SHALL open the per-model dashboard for that node
without losing the fleet view.

#### Scenario: Node rows carry their own verdict

- **WHEN** the fleet view renders
- **THEN** each row shows that node's identity, share versus expectation, verdict and activity strip

#### Scenario: Drill-down and return

- **WHEN** the user selects a node
- **THEN** its per-model dashboard is shown, and returning restores the fleet view with rows in the same order

#### Scenario: A wide fleet stays readable

- **GIVEN** more nodes than fit the available height
- **WHEN** the fleet view renders
- **THEN** the node rows scroll as one unit, every node is reachable, and the aggregate stays visible
