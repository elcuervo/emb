# pipeline-resource-lifecycle Specification

## Purpose
Specifies ownership and lifetime of pipeline resources: embedding pools, batchers, inference workers, tokenizer producers and registry-held tokenizers are constructed transactionally, published safely, and closed exactly once.

## Requirements
### Requirement: Transactional pool construction
Pool construction SHALL either return a usable pool or release all sessions and terminate all workers created by that construction attempt.

#### Scenario: Later session creation fails

- **WHEN** session creation succeeds for worker one and fails for worker two
- **THEN** construction SHALL return an error and close the first session exactly once
- **AND** no worker from the failed attempt SHALL remain running

### Requirement: Joined and idempotent closure
Pool and batcher closure SHALL stop admission, settle every accepted request exactly once, join inference workers and tokenizer producers, and release owned sessions exactly once. The registry SHALL close a shared tokenizer only after its users have stopped. Concurrent and repeated close calls SHALL observe the same completion and error outcome.

#### Scenario: Close during tokenization and inference

- **WHEN** closure begins with accepted work in the request queue, tokenizer handoff, or native inference
- **THEN** ordinary closure SHALL complete that accepted work and wait for all producers and workers before releasing their resources
- **AND** new submissions SHALL return a closed error without blocking indefinitely

#### Scenario: Repeated close

- **WHEN** two callers close the same pool concurrently and another closes it afterward
- **THEN** each owned session SHALL be closed once and all callers SHALL observe the stored closure outcome

### Requirement: Synchronized lazy resource publication
Concurrent first uses of a registered model SHALL synchronize initialization and publication before reading the embedding pool or its initialization error.

#### Scenario: Simultaneous cold requests

- **WHEN** multiple callers request the same previously unused model
- **THEN** one embedding pool SHALL be initialized and safely published to all successful callers
- **AND** the Go race detector SHALL report no conflicting pool publication access
