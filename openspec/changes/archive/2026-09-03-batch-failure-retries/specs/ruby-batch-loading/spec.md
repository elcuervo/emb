# ruby-batch-loading Specification

## Purpose
Specifies lazy client-side batching for the `emb` gem: embedding calls made within the same
execution scope (thread) coalesce into a single `EMB.MULTI` command using the `batch-loader`
gem, with a `batch` configuration option that makes the default proxy API lazy.

## MODIFIED Requirements

### Requirement: Batch failures retry then fail closed

When a batch command fails with a transient error (timeout, connection error, protocol error) and
`reconnect_attempts` is configured greater than zero, the gem SHALL re-send the command up
to `reconnect_attempts` additional times (the first attempt plus the configured retries in
total) before failing closed. Under the default configuration (`reconnect_attempts: 0`) a
transient failure SHALL fail closed after a single attempt. Operation errors — replies the
server parsed and rejected (`RedisClient::CommandError`, e.g. unknown model) — SHALL NOT be
retried under any configuration. When a batch fails after all retries (or immediately for an
operation error or the default configuration), the gem SHALL raise `Emb::ServerError` to the
code that forced the batch, SHALL remove every deferred item of that batch from the scope's
pending set, and SHALL NOT re-send the failed batch on any later resolution attempt.
`Emb::ServerError` SHALL carry the underlying redis error as `cause`, plus the model(s),
text count, and attempt count. Re-resolving an item from a failed batch SHALL return an
empty array (`[]`) and SHALL NOT perform I/O. New batches created in the same scope
afterwards SHALL contain only items deferred after the failure.

#### Scenario: Error surfaces once and pending is cleared

- **WHEN** a scope defers 6 items and the `EMB.MULTI` for them fails under the default configuration (`reconnect_attempts: 0`)
- **THEN** the forced value SHALL raise `Emb::ServerError` after a single attempt
- **AND** the scope's pending set SHALL be empty afterwards

#### Scenario: Transient failure recovers within retries

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers 6 items and the first two `EMB.MULTI` sends for them time out
- **AND** the third send succeeds
- **THEN** the forced loader SHALL materialize the real embeddings for all 6 items
- **AND** the server SHALL have received exactly 3 `EMB.MULTI` commands for the batch

#### Scenario: Exhausted retries raise a typed error and clear pending

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers 6 items and every `EMB.MULTI` send for them fails transiently
- **THEN** the forced value SHALL raise `Emb::ServerError`
- **AND** 3 attempts SHALL have been sent to the server (`reconnect_attempts + 1`)
- **AND** the scope's pending set SHALL be empty afterwards

#### Scenario: Operation errors are not retried

- **GIVEN** a client configured with `reconnect_attempts: 2`
- **WHEN** a scope defers items and the server returns an error reply (not a timeout/connection error) on the first send
- **THEN** the forced value SHALL raise `Emb::ServerError`
- **AND** exactly one command SHALL have been sent to the server

#### Scenario: ServerError carries failure context

- **WHEN** a batch fails (after exhausting retries, or on the first attempt)
- **THEN** the raised `Emb::ServerError`'s `cause` SHALL be the underlying redis error
- **AND** its message SHALL include the model(s), the text count, and the number of attempts

#### Scenario: Retry resolves to [] without I/O

- **WHEN** an item of a failed batch is resolved again after the failure
- **THEN** it SHALL return an empty array (`[]`)
- **AND** no command SHALL be sent to the server

#### Scenario: Subsequent batches exclude failed items

- **WHEN** a batch has failed in a scope and the scope then defers 2 new items which are forced
- **THEN** the server SHALL receive a single command containing exactly the 2 new pairs
- **AND** none of the previously failed items SHALL be re-sent

#### Scenario: Pending set stays bounded across failures

- **WHEN** several batches fail sequentially in the same scope (with or without retries in between)
- **THEN** the pending set SHALL NOT grow beyond the items currently deferred in the scope