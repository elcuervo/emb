## Purpose

Defines how the emb Ruby client distributes commands across its pool of connections so
that traffic fans out to all emb instances behind connection-level load balancers (such
as AWS Service Connect), instead of pinning to a single connection.

## ADDED Requirements

### Requirement: Round-robin connection distribution

When the client has more than one connection in its pool, the client SHALL route each
command through the next connection in rotation order: consecutive commands issued
without other activity SHALL use different connections, and selection SHALL wrap around
after the last connection.

#### Scenario: Sequential commands rotate across connections

- **WHEN** a client with `pool: 3` sends four commands in sequence (e.g. `PING`
  four times) with no concurrency
- **THEN** the four commands SHALL be sent over three connections in rotation order
  (connection 1, connection 2, connection 3, then connection 1 again)

#### Scenario: Single-connection pool keeps today's behavior

- **WHEN** a client is configured with `pool: 1`
- **THEN** all commands SHALL be sent over that single connection

### Requirement: Pool size controls connection count

The `pool` option SHALL control the number of Redis connections the client maintains:
`pool: N` SHALL yield N connections, and up to N commands SHALL be able to execute in
parallel without serializing on a shared connection.

#### Scenario: Parallel commands do not serialize

- **WHEN** a client with `pool: 8` issues embedding commands from 8 concurrent threads
- **THEN** the commands SHALL complete concurrently and all SHALL return correct
  results

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