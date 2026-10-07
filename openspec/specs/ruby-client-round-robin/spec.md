# ruby-client-round-robin

## Purpose

Defines how the emb Ruby client distributes commands across its pool of connections so
that traffic fans out to all emb instances behind connection-level load balancers (such
as AWS Service Connect), instead of pinning to a single connection.

## Requirements

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

### Requirement: Concurrent use is thread-safe

Commands issued concurrently from multiple threads SHALL never be sent over the same
connection at the same time, and SHALL each return the reply for their own command.

#### Scenario: Interleaved concurrent commands return correct replies

- **WHEN** multiple threads concurrently send commands with distinct payloads through a
  client with `pool: 4`
- **THEN** every thread SHALL receive the reply corresponding to its own command, with
  no cross-talk or corruption

### Requirement: Failure and reconnect behavior is unchanged

Connection failure and reconnect behavior SHALL behave as before this change:
redis-client reconnect attempts SHALL still apply, and error replies from the server
SHALL be raised as `RedisClient::CommandError` exactly as today.

#### Scenario: Server restart recovers

- **WHEN** the server drops the connection while a client with `reconnect_attempts: 3`
  is idle, then comes back
- **THEN** the next command SHALL reconnect and succeed

#### Scenario: Server error replies still raise

- **WHEN** the server replies with an error (for example an `ERR busy` rejection from
  `max_concurrent_requests`)
- **THEN** the client SHALL raise `RedisClient::CommandError` with that message

### Requirement: Work-conserving connection selection

When more commands are in flight than the pool has connections, a waiting command SHALL
run on whichever connection becomes free first, and SHALL NOT remain blocked on a specific
busy connection while another pool connection is free. Idle/sequential rotation order and
multi-instance selection SHALL be unchanged.

#### Scenario: Waiting command takes a freed connection

- **WHEN** a client with `pool: 2` has two commands in flight on both connections and a
  third command waiting, and one of the held connections is released
- **THEN** the waiting command SHALL proceed on the released connection without waiting
  for the other, still-held connection

#### Scenario: A freed connection is never left idle behind a waiter

- **WHEN** a connection is released while any command is waiting for a connection
- **THEN** exactly one waiting command SHALL be served from that freed connection before
  any command blocks again

#### Scenario: Pool of one still serializes

- **WHEN** a client is configured with `pool: 1`
- **THEN** concurrent commands SHALL queue on that single connection in arrival order
