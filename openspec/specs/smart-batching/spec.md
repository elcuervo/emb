# smart-batching Specification

## Purpose
Specifies the optional smart-batching window that coalesces concurrent EMB requests for the same model into shared ONNX runs for higher throughput.
## Requirements
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

### Requirement: Throughput improvement

The smart batcher SHALL improve throughput under concurrent load without degrading single-request latency by more than the configured timeout.

#### Scenario: Concurrent throughput

- **WHEN** 8 concurrent clients send requests
- **THEN** throughput SHALL exceed the current baseline (509 req/s) by at least 50%

#### Scenario: Single-request latency

- **WHEN** a single client sends requests
- **THEN** p50 latency SHALL NOT exceed the baseline (3ms) + timeout

### Requirement: Idle-flush for single-request latency

When the batching run loop is idle (no batch currently in flight), a newly arrived request SHALL be executed immediately rather than waiting out the batching window, so a lone request does not incur an artificial delay beyond configuration when the server is idle; under concurrent load the window SHALL still coalesce requests as configured.

#### Scenario: Lone request not delayed when run loop idle

- **WHEN** the server is idle and a single embedding request arrives for a batched model
- **THEN** the request SHALL be served without waiting the configured `timeout`
- **AND** the returned embedding SHALL be correct and byte-identical to the same text via a non-batched path

#### Scenario: Bursts still batch under load

- **WHEN** many concurrent requests arrive for the same batched model
- **THEN** consecutive requests SHALL still be coalesced into shared ONNX runs within the configured window and token budget

