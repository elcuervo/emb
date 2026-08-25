# async-tokenization Specification

## Purpose
TBD - created by archiving change async-tokenization. Update Purpose after archive.
## Requirements
### Requirement: Tokenization off the critical path
The server SHALL tokenize texts on dedicated workers concurrent with inference, so tokenization of a later request can overlap the run of an earlier batch.

#### Scenario: Overlap with inference
- **WHEN** multiple requests are queued
- **THEN** the server SHALL tokenize subsequent requests while the current batch is being inferred
- **THEN** each request SHALL receive its embeddings in arrival order

#### Scenario: Bounded queue
- **WHEN** tokenizer workers are slower than inference for a sustained burst
- **THEN** the queue SHALL be bounded and apply backpressure to the connection handler rather than growing without limit

### Requirement: Configurable tokenizer concurrency
The server SHALL expose a `tokenize_workers` config controlling the number of tokenizer goroutines.

#### Scenario: Default and disable
- **WHEN** `tokenize_workers` is unset
- **THEN** the server SHALL default to `min(4, cores)`
- **WHEN** `tokenize_workers: 0`
- **THEN** the server SHALL use the current serial tokenize-then-run behavior

#### Scenario: Seeds tokens and budget
- **WHEN** the P1 token budget is enabled
- **THEN** the async stage SHALL provide per-request real-token counts consumed by the batcher's budget flush

### Requirement: Error and lifecycle semantics preserved
Errors SHALL surface on the requesting channel, and `Close` SHALL drain in-flight work.

#### Scenario: Tokenization error
- **WHEN** tokenization fails for a request's text
- **THEN** that request's channel SHALL receive an error, and other requests in the batch SHALL remain unaffected

#### Scenario: Graceful shutdown
- **WHEN** the server closes a model's pool during shutdown
- **THEN** queued encodings SHALL be flushed or requester channels drained without deadlock

