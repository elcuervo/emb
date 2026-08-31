# token-budget-batching Specification

## Purpose
TBD - created by archiving change token-budget-batching. Update Purpose after archive.
## Requirements
### Requirement: Token-budget flush bound
The batcher SHALL flush a batch when accumulated real tokens across queued requests reach the configured `max_batch_tokens` budget.

#### Scenario: Mixed-length budget flush
- **WHEN** queued requests in the window accumulate to 16384 real tokens (one long + many short texts)
- **THEN** the batcher SHALL start the ONNX run immediately, without waiting for the timeout or max batch count
- **THEN** padding waste SHALL be bounded to the last partially-filled run

#### Scenario: Budget overrides count
- **WHEN** a window contains fewer requests than `max_batch` but their total tokens exceed `max_batch_tokens`
- **THEN** the batcher SHALL flush on the token budget

#### Scenario: Count remains a secondary bound
- **WHEN** accumulated requests reach `max_batch` before the token budget
- **THEN** the batcher SHALL flush on request count as before

#### Scenario: Zero budget disables accounting
- **WHEN** a model config sets `max_batch_tokens: 0` (or omits it and the default is considered off)
- **THEN** the batcher SHALL behave exactly as the pre-change count-based window

### Requirement: Oversized commands are truncated at admission

A single `EMB` command exceeding the configured `max_texts` cap SHALL be truncated to its first `max_texts` texts before any cache lookup or inference, with overflow reply slots as `null`; commands within the cap SHALL be processed as before.

#### Scenario: Within-cap oversized-token command runs as one inference

- **WHEN** a single `EMB` command carries more real tokens than the window budget but has fewer texts than `max_texts`
- **THEN** the server SHALL run it as a single inference and return all its embeddings (behavior unchanged from before this change)

#### Scenario: Over-cap command is truncated

- **WHEN** a single `EMB` command carries more texts than `max_texts`
- **THEN** the server SHALL infer only the first `max_texts` texts
- **AND** SHALL reply with one slot per requested text, overflow slots `null`
- **AND** SHALL NOT tokenize, cache-lookup, or infer the overflow texts

### Requirement: Budget observability
The server SHALL expose the active token budget and padding efficiency through the stats commands.

#### Scenario: EMB.INFO exposes budget and efficiency
- **WHEN** a client calls `EMB.INFO <model>` on a model with token-budget batching
- **THEN** the response SHALL include `batching_max_tokens` and padding efficiency (real tokens / processed token-slots)

