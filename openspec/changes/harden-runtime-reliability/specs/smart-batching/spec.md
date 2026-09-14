## ADDED Requirements

### Requirement: Strict batch request count
Every batch dispatch SHALL contain at most the configured positive max_batch request count, including idle draining with serial or asynchronous tokenization. A multi-text request SHALL count as one request. Existing indivisible-request token-budget behavior SHALL be preserved.

#### Scenario: Single-request batch with queued follower
- **WHEN** max_batch is one and a second request queues while the first is tokenized
- **THEN** the two requests SHALL execute in separate inference runs

#### Scenario: Serial and asynchronous bursts
- **WHEN** more requests than max_batch are ready in either tokenization mode
- **THEN** each dispatched batch SHALL contain no more than max_batch requests
- **AND** every request SHALL receive its corresponding result exactly once
