## Purpose

Specifies how the emb Ruby client addresses multiple emb instances: `url` accepts an array of endpoints treated as interchangeable replicas, work distributes across instances in rotation, batch mode dispatches chunk shares concurrently, and pre-send failures retry safely on another instance without risking duplicate inference.

## ADDED Requirements

### Requirement: Multi-instance configuration

The gem SHALL accept `url` as either a String (single instance) or an Array of Strings (multiple instances). Each entry SHALL be a redis URL identifying one emb instance, and the instances SHALL be interchangeable replicas serving the same model set. The client SHALL maintain one connection pool per instance; the `pool` option SHALL size each per-instance pool independently.

#### Scenario: Single url behaves as today

- **WHEN** `Emb.new(url: "redis://emb-a:6379")` is created
- **THEN** the client SHALL maintain a single connection pool to `emb-a`

#### Scenario: Array url creates per-instance pools

- **WHEN** `Emb.new(url: ["redis://emb-a:6379", "redis://emb-b:6379"], pool: 2)` is created
- **THEN** the client SHALL maintain two connection pools, each of size 2

#### Scenario: Empty array is rejected

- **WHEN** a client is configured with an empty url array
- **THEN** configuration SHALL raise a clear error

### Requirement: Instance round-robin distribution

Eager commands (immediate `EMB` sends) SHALL distribute across the configured instances in rotation order, wrapping around after the last instance. Within the selected instance, connection-level round-robin SHALL continue to apply (per the `ruby-client-round-robin` capability).

#### Scenario: Sequential eager commands rotate across instances

- **WHEN** a client with 3 urls sends four eager embeds in sequence with no concurrency
- **THEN** the four commands SHALL be sent to instances 1, 2, 3, then 1 again, in rotation order

#### Scenario: Single-instance rotation preserved

- **WHEN** a client has exactly one url
- **THEN** all commands SHALL be sent to that instance, rotating across its connections as today

### Requirement: Concurrent batch fan-out

In `lazy: :batch` mode, when a scope's deferred pairs resolve, the client SHALL dispatch the chunk shares to instances concurrently (one share per instance) rather than serially, SHALL dispatch every share before waiting on any single share's completion, and SHALL reassemble the results in deferral order after all shares complete.

#### Scenario: Shares dispatch in parallel across instances

- **WHEN** `lazy: :batch` is configured with 2 urls and a scope defers pairs spanning two chunks
- **THEN** both chunk shares SHALL be in flight simultaneously, one to each instance
- **AND** the resolving call SHALL wait for both and return results in deferral order

#### Scenario: Wall time bounded by the slowest share

- **WHEN** a scope resolves into a slow share and a fast share dispatched to different instances
- **THEN** the resolving call SHALL complete in approximately the slow share's latency, not the sum of both

### Requirement: Pre-send failure retry

When dispatch of a chunk share to an instance fails before the command was sent (connection-level error), the client SHALL retry the share on another instance. When a command times out after it was sent (read timeout), the client SHALL NOT re-dispatch it to any instance: the command may have executed, and re-sending would duplicate inference. After retries are exhausted, terminal share failures SHALL follow the fail-closed behavior of `ruby-batch-loading`.

#### Scenario: Dead instance does not lose the share

- **WHEN** a share's selected instance refuses the connection before the command is sent, with a healthy instance available
- **THEN** the share SHALL be dispatched to the healthy instance
- **AND** the forcing caller SHALL receive the correct embeddings

#### Scenario: Read timeout is not re-dispatched

- **WHEN** a share's command is sent and the read times out
- **THEN** the timeout error SHALL surface to the forcing caller
- **AND** no instance SHALL receive a re-sent copy of that share

### Requirement: Per-instance isolation on failure

A terminal failure of one share SHALL NOT cause healthy shares' already-executed work to be re-sent, and SHALL NOT leave the failed share's items in the scope's pending set.

#### Scenario: Successful shares stay consumed

- **WHEN** two shares dispatch in parallel and exactly one fails terminally after retries
- **THEN** the force SHALL raise the failing share's error
- **AND** the successful share's commands SHALL NOT be re-sent on any later resolution
- **AND** the failed share's items SHALL be cleared from the scope's pending set