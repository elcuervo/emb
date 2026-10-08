# Spec Delta

## ADDED Requirements

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
