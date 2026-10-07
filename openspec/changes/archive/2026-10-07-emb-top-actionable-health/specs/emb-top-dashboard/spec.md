# Spec Delta

## MODIFIED Requirements

### Requirement: emb-top renders a health status banner

`emb-top` SHALL derive one overall node health status from its polled metrics — `healthy`, `degraded`, `critical`, or `no data`. The verdict SHALL be driven only by actionable node signals: connection state, error ratio, p95 latency relative to the session baseline, and CPU usage. Cache hit rate SHALL render as an informational chip that cannot raise the status. Thresholds SHALL be fixed built-in defaults; the change introduces no new flags.

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
