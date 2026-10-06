# Spec Delta

## ADDED Requirements

### Requirement: The zone recovers a lost upstream connection

A lost connection to the upstream MUST NOT end the zone's service. When a call
fails because the connection itself is gone, the zone SHALL dial again and retry
that call once, and it MUST load the composition preset on the new connection,
because the server that answers it may be a restarted one. A failure the server
replied with MUST NOT be treated as a lost connection.

#### Scenario: An idle close is recovered

- **WHEN** the upstream closes a connection that was idle for its own timeout
- **THEN** the next query is answered, after the zone dials again

#### Scenario: The preset is reloaded on a redial

- **WHEN** the zone redials and the server has no composition preset loaded
- **THEN** the preset is loaded before the query is retried, and the query is answered

#### Scenario: A reply is not retried

- **WHEN** the server answers a call with an error reply
- **THEN** that error is returned and no redial is attempted

### Requirement: One upstream connection serves one caller at a time

A call on the upstream is a write, a flush, and a read on one socket, so the zone
SHALL serialize its upstream calls: two queries in flight at once MUST NOT
interleave their exchanges on the connection.

#### Scenario: Concurrent queries share the connection safely

- **WHEN** several queries are served concurrently
- **THEN** each is answered, and they use the one connection the zone holds

#### Scenario: A redial is not interleaved

- **WHEN** a call is redialling after a lost connection
- **THEN** no other call uses the connection until the redial and its retry are done
