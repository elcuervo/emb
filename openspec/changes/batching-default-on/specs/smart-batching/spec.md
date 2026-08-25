# smart-batching Specification

## Purpose
Specifies the smart-batching window that coalesces concurrent EMB requests for the same model into shared ONNX runs for higher throughput.

## MODIFIED Requirements

### Requirement: Batch concurrent requests

The server SHALL collect concurrent embedding requests for the same model and execute them as a single batched ONNX inference, and batching SHALL be enabled by default so every model gets the performance path without configuration.

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