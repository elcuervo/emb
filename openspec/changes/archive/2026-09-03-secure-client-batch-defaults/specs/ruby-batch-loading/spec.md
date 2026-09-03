## ADDED Requirements

### Requirement: Batch failures fail closed

When a batch command fails (timeout, connection error, or protocol error), the gem SHALL surface the error to the code that forced the batch, SHALL remove every deferred item of that batch from the scope's pending set, and SHALL NOT re-send the failed batch on any later resolution attempt. Re-resolving an item from a failed batch SHALL return an empty array (`[]`) and SHALL NOT perform I/O. New batches created in the same scope afterwards SHALL contain only items deferred after the failure.

#### Scenario: Error surfaces once and pending is cleared

- **WHEN** a scope defers 6 items and the `EMB.MULTI` for them fails
- **THEN** the forced value SHALL raise the original error
- **AND** the scope's pending set SHALL be empty afterwards

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