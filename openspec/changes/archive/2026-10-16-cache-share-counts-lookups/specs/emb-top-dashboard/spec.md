# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders resource and cache gauges

`emb-top` SHALL render cache hit ratio and hit/miss/eviction rates from `EMB.STATS` cache counters, and RSS memory, CPU usage (derived from `cpu_user_usec`/`cpu_sys_usec` deltas), connections, active requests, goroutines, and uptime.

CPU usage SHALL be read as a share of the node's own capacity — the same unit as the health banner's CPU chip — and the dashboard SHALL additionally state the CPU the window spent per uncached request, so a node whose traffic is mostly cache-served is not read as idle: the denominator of the CPU readout SHALL be the work that actually ran, not the requests that arrived.

The cache readout SHALL state the share of the window's cache lookups the cache answered (the answered over the answered plus the missed, 0–100) beside the hit ratio: the ratio is the node's own cumulative one and the share is the window's. Both SHALL be counted over the cache's lookups, never over the requests that ran (`total_requests` counts only the work that reached a session, and a cache hit reaches none): dividing by it reports more cache-served work than the window carried, which reads as a share above 100%.

#### Scenario: Cache gauges

- **WHEN** the server has a cache configured
- **THEN** the dashboard shows cache hit ratio, the share of the window's cache lookups the cache answered, and recent hit/miss/eviction rates

#### Scenario: The stated share cannot exceed the window's lookups

- **WHEN** a warm window answers far more cache lookups than the requests that ran through a session (one text repeated reads as hundreds of hits against one run)
- **THEN** the stated share SHALL stay within 0-100%, because its denominator is the cache's own lookups and not `total_requests`

#### Scenario: CPU gauge

- **WHEN** the node is under inference load
- **THEN** the CPU gauge rises proportionally to the `cpu_user_usec`/`cpu_sys_usec` deltas over the poll interval

#### Scenario: The CPU readout is the share of the node's capacity

- **WHEN** a node with more than one core reports CPU usage
- **THEN** the gauge and its value SHALL show the share of the node's capacity, matching the banner's chip for the same poll

#### Scenario: Cache-served traffic is not read as idle

- **WHEN** a window's requests are mostly answered from the cache and the CPU is low
- **THEN** the dashboard SHALL state the cached share and the CPU spent per uncached request, so the low CPU is attributable to the cache rather than to an idle node
