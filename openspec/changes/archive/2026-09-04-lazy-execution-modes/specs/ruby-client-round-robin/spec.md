## MODIFIED Requirements

### Requirement: Round-robin connection distribution

When the client addresses a single instance (one url), the client SHALL route each command through the next connection in rotation order: consecutive commands issued without other activity SHALL use different connections, and selection SHALL wrap around after the last connection. When the client addresses multiple instances (`url` as an array), selection SHALL first rotate across instances (per the `client-multi-instance-distribution` capability), and within the selected instance across its connections in rotation order.

#### Scenario: Sequential commands rotate across connections

- **WHEN** a client with `pool: 3` sends four commands in sequence (e.g. `PING` four times) with no concurrency
- **THEN** the four commands SHALL be sent over three connections in rotation order (connection 1, connection 2, connection 3, then connection 1 again)

#### Scenario: Single-connection pool keeps today's behavior

- **WHEN** a client is configured with `pool: 1`
- **THEN** all commands SHALL be sent over that single connection

#### Scenario: Multi-instance selection rotates instances first

- **WHEN** a client has two urls and each instance pool has two connections, and four commands are sent in sequence
- **THEN** commands SHALL alternate instances, rotating connections within each instance

### Requirement: Pool size controls connection count

The `pool` option SHALL control the number of Redis connections the client maintains per instance: `pool: N` with one url SHALL yield N connections, and up to N commands SHALL be able to execute in parallel without serializing on a shared connection. With multiple urls, each instance SHALL maintain its own pool of N connections.

#### Scenario: Parallel commands do not serialize

- **WHEN** a client with `pool: 8` issues embedding commands from 8 concurrent threads
- **THEN** the commands SHALL complete concurrently and all SHALL return correct results

#### Scenario: Per-instance pools share the size option

- **WHEN** a client with `pool: 2` and three urls issues commands
- **THEN** each of the three per-instance pools SHALL have 2 connections