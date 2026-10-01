# smart-batching Specification

## Purpose
Specifies how the server batches concurrent embedding requests within a timeout window to improve throughput while bounding latency.

## MODIFIED Requirements

### Requirement: Batch concurrent requests

The server SHALL batch concurrent embedding requests for a model into multi-row inference runs within a configurable timeout window, and batching SHALL be enabled by default so every model gets the performance path without configuration.

#### Scenario: Configurable batching timeout

- **WHEN** a model config has `timeout: 5`
- **THEN** the server SHALL wait at most 5ms before executing the batched inference
- **THEN** batching SHALL be enabled by default with a 1ms window when `timeout` is unset

#### Scenario: Batching disabled

- **WHEN** a model config sets `timeout: 0`
- **THEN** the server SHALL NOT batch and SHALL use the worker pool instead

#### Scenario: Defaults engage automatically

- **WHEN** batching is enabled (default or explicit) and `max_batch_tokens` / `tokenize_workers` are unset
- **THEN** the token budget SHALL default to 16384 and tokenizer workers SHALL default to `min(4, cores)`

#### Scenario: Determinism-gated batching

- **WHEN** a model loads with batching enabled and its batch-determinism probe fails
- **THEN** the model SHALL NOT batch; requests SHALL run single-row on the worker pool
- **THEN** the effective batching timeout SHALL be `0` for that model

#### Scenario: Deterministic models batch as before

- **WHEN** a model's batch-determinism probe passes
- **THEN** concurrent requests SHALL batch within the configured timeout window, as specified by the `batch-determinism` capability